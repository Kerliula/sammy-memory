// Package config loads Sammy's runtime configuration from the environment.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/caarlos0/env/v11"
)

// Config holds the settings every Sammy subcommand needs.
type Config struct {
	DatabaseURL string `env:"DATABASE_URL,required"`
	HTTPAddr    string `env:"HTTP_ADDR" envDefault:":8080"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
}

// Load parses the process environment into a Config.
func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if _, err := cfg.SlogLevel(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// SlogLevel converts the configured LOG_LEVEL into a slog.Level.
func (c Config) SlogLevel() (slog.Level, error) {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid LOG_LEVEL %q: want debug, info, warn or error", c.LogLevel)
	}
}

// Logger builds the process-wide JSON logger at the configured level.
func (c Config) Logger() *slog.Logger {
	level, _ := c.SlogLevel() // validated by Load
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}
