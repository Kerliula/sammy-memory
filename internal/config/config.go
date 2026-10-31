// Package config reads the settings Sammy needs when it starts.
package config

import (
	"fmt"      // Build error messages.
	"log/slog" // Create structured logs.
	"os"       // Access standard error output (stderr).
	"strings"  // Convert text to lowercase.

	"github.com/caarlos0/env/v11" // Read environment variables into a struct.
)

// Config groups our settings into one value. A struct is a set of named fields.
// Each field below is a string, which means text.
//
// The labels in backticks tell the env library where to read each setting.
type Config struct {
	DatabaseURL string `env:"DATABASE_URL,required"`

	HTTPAddr string `env:"HTTP_ADDR" envDefault:":8080"`

	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

func Load() (Config, error) {
	// [Config] tells the env library which struct to fill in.
	settings, err := env.ParseAs[Config]()
	if err != nil {
		// Config{} is an empty Config. %w includes the original error.
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	// Check that LogLevel contains a supported value.
	// The underscore discards the level: we only need the error here.
	_, err = settings.SlogLevel()
	if err != nil {
		return Config{}, err
	}

	return settings, nil
}

// SlogLevel converts a text setting such as "info" into a logging constant.
// (settings Config) makes this a method: call it with settings.SlogLevel().
func (settings Config) SlogLevel() (slog.Level, error) {
	levelName := strings.ToLower(settings.LogLevel)
	switch levelName {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		// %q puts quotes around the invalid value in the error message.
		return 0, fmt.Errorf("invalid LOG_LEVEL %q: want debug, info, warn or error", settings.LogLevel)
	}
}

// Logger creates a logger that writes JSON to stderr.
// Call this on settings returned successfully by Load.
func (settings Config) Logger() *slog.Logger {
	// Load already checked this value, so we discard the error here.
	minimumLevel, _ := settings.SlogLevel()

	options := slog.HandlerOptions{
		Level: minimumLevel,
	}

	// The handler controls the output format, destination, and minimum level.
	// &options passes a pointer (a reference to the options value).
	handler := slog.NewJSONHandler(os.Stderr, &options)

	return slog.New(handler)
}
