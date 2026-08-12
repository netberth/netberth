// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/netberth/netberth/internal/api/router"
	ws "github.com/netberth/netberth/internal/api/websocket"
	"github.com/netberth/netberth/internal/auth"
	"github.com/netberth/netberth/internal/config"
	"github.com/netberth/netberth/internal/db"
	"github.com/netberth/netberth/internal/diagnose"
	"github.com/netberth/netberth/internal/service"
	"github.com/netberth/netberth/internal/tlsutil"
	"github.com/netberth/netberth/pkg/logger"
)

func main() {
	cfg, err := config.Load(os.Getenv("NB_CONFIG_PATH"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}
	logger.Init(cfg.Log.Level, cfg.Log.Format)

	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		res := diagnose.Run(cfg)
		diagnose.Print(os.Stdout, res)
		if !res.AllOK() {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// SQLite stores everything next to the DB file; Postgres keeps local
	// state (JWT secret, certs) under ./data.
	dataDir := filepath.Dir(cfg.Database.Path)
	if cfg.Database.Driver != "" && cfg.Database.Driver != "sqlite" {
		dataDir = "./data"
	}

	jwtSecret := cfg.Auth.JWTSecret
	if jwtSecret == "" {
		secretPath := filepath.Join(dataDir, ".jwt_secret")
		if data, err := os.ReadFile(secretPath); err == nil && len(data) > 0 {
			jwtSecret = string(data)
			logger.Log.Info().Msg("JWT secret loaded from persisted file")
		} else {
			jwtSecret = randomHex(32)
			os.MkdirAll(filepath.Dir(secretPath), 0700)
			if err := os.WriteFile(secretPath, []byte(jwtSecret), 0600); err != nil {
				logger.Log.Warn().Err(err).Msg("failed to persist JWT secret")
			} else {
				logger.Log.Info().Msg("JWT secret persisted to .jwt_secret")
			}
		}
	}

	dbDSN := cfg.Database.Path
	if cfg.Database.Driver != "" && cfg.Database.Driver != "sqlite" {
		dbDSN = cfg.Database.DSN
	}
	database, err := db.OpenDatabase(cfg.Database.Driver, dbDSN)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to open database")
	}
	defer database.Close()

	authService := auth.NewService(
		jwtSecret,
		cfg.Auth.AccessTokenExpiry,
		cfg.Auth.RefreshTokenExpiry,
	)

	// First-run admin user initialization
	adminPass := randomHex(8)
	hash, err := authService.HashPassword(adminPass)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to hash admin password")
	}
	seeded, err := db.SeedAdminUser(database, hash)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to seed admin user")
	}
	if seeded {
		logger.Log.Warn().Str("username", "admin").Str("password", adminPass).
			Msg("ADMIN CREDENTIALS — change immediately after login")
	}

	certDir := filepath.Join(dataDir, "certs")
	wire := service.NewWire(database, certDir)
	if err := wire.StartAll(); err != nil {
		logger.Log.Warn().Err(err).Msg("some engines failed to start")
	}

	hub := ws.NewHub(wire.Forward, database)
	go hub.Broadcast()
	webhookDispatcher := service.NewWebhookDispatcher(database, wire.Bus())
	defer webhookDispatcher.Stop()
	handler := router.New(database, authService, wire, hub, router.Options{
		TrustedProxies:    cfg.Server.TrustedProxies,
		RateLimitRate:     cfg.Server.RateLimitRate,
		RateLimitBurst:    cfg.Server.RateLimitBurst,
		WebhookDispatcher: webhookDispatcher,
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
		MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
	}

	scheme := "http"
	if cfg.Server.TLSEnabled {
		certPath, keyPath := cfg.Server.TLSCert, cfg.Server.TLSKey
		if certPath == "" && keyPath == "" {
			certPath = filepath.Join(dataDir, "tls", "server.crt")
			keyPath = filepath.Join(dataDir, "tls", "server.key")
			tlsCert, err := tlsutil.EnsureSelfSigned(certPath, keyPath, []string{"localhost", "127.0.0.1", "::1"})
			if err != nil {
				logger.Log.Fatal().Err(err).Msg("failed to create self-signed TLS certificate")
			}
			srv.TLSConfig = tlsutil.ServerTLSConfig(tlsCert)
			logger.Log.Warn().Str("cert", certPath).
				Msg("TLS enabled with auto-generated self-signed certificate — replace with a trusted certificate for production")
		} else {
			if certPath == "" || keyPath == "" {
				logger.Log.Fatal().Msg("NB_TLS_CERT and NB_TLS_KEY must be set together")
			}
			tlsCert, err := tlsutil.Load(certPath, keyPath)
			if err != nil {
				logger.Log.Fatal().Err(err).Msg("failed to load TLS certificate")
			}
			srv.TLSConfig = tlsutil.ServerTLSConfig(tlsCert)
		}
		scheme = "https"
	}

	go func() {
		logger.Log.Info().Str("addr", addr).Str("scheme", scheme).Msg("NetBerth starting")
		if cfg.Server.TLSEnabled {
			if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				logger.Log.Fatal().Err(err).Msg("server failed")
			}
			return
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Log.Fatal().Err(err).Msg("server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info().Msg("shutting down...")
	wire.StopAll()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Log.Fatal().Err(err).Msg("server forced to shutdown")
	}
	logger.Log.Info().Msg("server stopped")
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
