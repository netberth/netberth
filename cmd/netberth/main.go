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
	"github.com/netberth/netberth/internal/service"
	"github.com/netberth/netberth/pkg/logger"
)

func main() {
	cfg, err := config.Load(os.Getenv("NB_CONFIG_PATH"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}
	logger.Init(cfg.Log.Level, cfg.Log.Format)

	jwtSecret := cfg.Auth.JWTSecret
	if jwtSecret == "" {
		secretPath := filepath.Join(filepath.Dir(cfg.Database.Path), ".jwt_secret")
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

	database, err := db.Open(cfg.Database.Path)
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

	certDir := filepath.Join(filepath.Dir(cfg.Database.Path), "certs")
	wire := service.NewWire(database, certDir)
	if err := wire.StartAll(); err != nil {
		logger.Log.Warn().Err(err).Msg("some engines failed to start")
	}

	hub := ws.NewHub(wire.Forward, database)
	go hub.Broadcast()
	handler := router.New(database, authService, wire, hub)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		logger.Log.Info().Str("addr", addr).Msg("NetBerth starting")
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
