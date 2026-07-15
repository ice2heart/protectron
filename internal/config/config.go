// Package config loads bot configuration from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	APIToken string
	MongoURI string
	MongoDB  string
	// AdminID is the bot owner: greeted on join instead of being captcha'd. 0 = unset.
	AdminID  int64
	LogLevel slog.Level
}

func Load() (*Config, error) {
	cfg := &Config{
		MongoDB:  "protectron",
		LogLevel: slog.LevelInfo,
	}

	cfg.APIToken = os.Getenv("API_TOKEN")
	if cfg.APIToken == "" {
		return nil, fmt.Errorf("API_TOKEN is required")
	}

	cfg.MongoURI = os.Getenv("MONGO_URI")
	if cfg.MongoURI == "" {
		return nil, fmt.Errorf("MONGO_URI is required")
	}

	if v := os.Getenv("MONGO_DB"); v != "" {
		cfg.MongoDB = v
	}

	if v := os.Getenv("ADMIN_ID"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("ADMIN_ID must be an integer: %w", err)
		}
		cfg.AdminID = id
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(strings.ToUpper(v))); err != nil {
			return nil, fmt.Errorf("LOG_LEVEL: %w", err)
		}
	}

	return cfg, nil
}
