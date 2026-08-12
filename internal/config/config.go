// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Log      LogConfig      `yaml:"log"`
}

type ServerConfig struct {
	Host         string        `yaml:"host" env:"NB_SERVER_HOST"`
	Port         int           `yaml:"port" env:"NB_SERVER_PORT"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	// ReadHeaderTimeout bounds how long a peer may take to send headers
	// (slowloris defense); IdleTimeout bounds keep-alive idle sockets.
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	MaxHeaderBytes    int           `yaml:"max_header_bytes"`
	RateLimitRate     int           `yaml:"rate_limit_rate" env:"NB_RATE_LIMIT_RATE"`
	RateLimitBurst    int           `yaml:"rate_limit_burst" env:"NB_RATE_LIMIT_BURST"`
	TrustedProxies    []string      `yaml:"trusted_proxies" env:"NB_TRUSTED_PROXIES"`
	TLSEnabled        bool          `yaml:"tls_enabled" env:"NB_TLS_ENABLED"`
	TLSCert           string        `yaml:"tls_cert" env:"NB_TLS_CERT"`
	TLSKey            string        `yaml:"tls_key" env:"NB_TLS_KEY"`
}

type DatabaseConfig struct {
	Path   string `yaml:"path" env:"NB_DB_PATH"`
	Driver string `yaml:"driver" env:"NB_DB_DRIVER"` // sqlite (default) or postgres
	DSN    string `yaml:"dsn" env:"NB_DB_DSN"`       // postgres://... when driver=postgres
}

type AuthConfig struct {
	JWTSecret          string        `yaml:"jwt_secret" env:"NB_JWT_SECRET"`
	AccessTokenExpiry  time.Duration `yaml:"access_token_expiry"`
	RefreshTokenExpiry time.Duration `yaml:"refresh_token_expiry"`
	SessionMaxAge      time.Duration `yaml:"session_max_age"`
}

type LogConfig struct {
	Level  string `yaml:"level" env:"NB_LOG_LEVEL"`
	Format string `yaml:"format"` // json or console
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:              "0.0.0.0",
			Port:              8443,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    64 << 10,
			RateLimitRate:     100,
			RateLimitBurst:    200,
			TLSEnabled:        false,
		},
		Database: DatabaseConfig{
			Path: "./data/netberth.db",
		},
		Auth: AuthConfig{
			JWTSecret:          "",
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
			SessionMaxAge:      24 * time.Hour,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()
	if path == "" {
		path = filepath.Join("config", "netberth.yaml")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	// Env overrides must apply whether or not a config file exists. A missing
	// config file is the normal deployment shape (e.g. Docker), and skipping
	// applyEnv here silently disabled every NB_* variable.
	cfg.applyEnv()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func parsePort(v string) int {
	port := 0
	for _, c := range v {
		if c >= '0' && c <= '9' {
			port = port*10 + int(c-'0')
		}
	}
	if port == 0 {
		return 8443
	}
	return port
}

func (c *Config) applyEnv() {
	if v := os.Getenv("NB_SERVER_HOST"); v != "" {
		c.Server.Host = v
	}
	if v := os.Getenv("NB_SERVER_PORT"); v != "" {
		c.Server.Port = parsePort(v)
	}
	if v := os.Getenv("NB_DB_PATH"); v != "" {
		c.Database.Path = v
	}
	if v := os.Getenv("NB_DB_DRIVER"); v != "" {
		c.Database.Driver = strings.ToLower(strings.TrimSpace(v))
	}
	if v := os.Getenv("NB_DB_DSN"); v != "" {
		c.Database.DSN = v
	}
	if v := os.Getenv("NB_JWT_SECRET"); v != "" {
		c.Auth.JWTSecret = v
	}
	if v := os.Getenv("NB_LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
	if v := os.Getenv("NB_TLS_ENABLED"); v != "" {
		c.Server.TLSEnabled = parseBool(v)
	}
	if v := os.Getenv("NB_TLS_CERT"); v != "" {
		c.Server.TLSCert = v
	}
	if v := os.Getenv("NB_TLS_KEY"); v != "" {
		c.Server.TLSKey = v
	}
	if v := os.Getenv("NB_RATE_LIMIT_RATE"); v != "" {
		c.Server.RateLimitRate = parseInt(v)
	}
	if v := os.Getenv("NB_RATE_LIMIT_BURST"); v != "" {
		c.Server.RateLimitBurst = parseInt(v)
	}
	if v := os.Getenv("NB_TRUSTED_PROXIES"); v != "" {
		c.Server.TrustedProxies = splitList(v)
	}
}

// validate rejects configuration that would lock the panel out or silently
// ignore proxy headers. Called after YAML + env are merged.
func (c *Config) validate() error {
	if c.Server.RateLimitRate < 1 || c.Server.RateLimitBurst < 1 {
		return fmt.Errorf("rate_limit_rate and rate_limit_burst must be >= 1 (got %d/%d)",
			c.Server.RateLimitRate, c.Server.RateLimitBurst)
	}
	for _, s := range c.Server.TrustedProxies {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if strings.Contains(s, "/") {
			if _, _, err := net.ParseCIDR(s); err != nil {
				return fmt.Errorf("invalid trusted proxy CIDR %q: %w", s, err)
			}
			continue
		}
		if net.ParseIP(s) == nil {
			return fmt.Errorf("invalid trusted proxy IP %q", s)
		}
	}
	return nil
}

func parseInt(v string) int {
	n := 0
	for _, c := range v {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func splitList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
