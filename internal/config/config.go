// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package config

import (
	"os"
	"path/filepath"
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
}

type DatabaseConfig struct {
	Path string `yaml:"path" env:"NB_DB_PATH"`
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
			Host:         "0.0.0.0",
			Port:         8443,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
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
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	cfg.applyEnv()
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
	if v := os.Getenv("NB_JWT_SECRET"); v != "" {
		c.Auth.JWTSecret = v
	}
	if v := os.Getenv("NB_LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
}
