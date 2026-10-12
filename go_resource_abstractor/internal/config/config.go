// Package config centralizes environment-variable configuration and slog
// setup for the resource-abstractor binary. It reads the same env vars as
// the Python service (resource-abstractor/db/mongodb_client.py,
// resource_abstractor.py and services/hook_service.py).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLogLevel        = "DEBUG"
	defaultHookConnectSecs = 10
	defaultHookRequestSecs = 5
)

// Config holds all runtime configuration read from the environment.
type Config struct {
	// Port the HTTP server listens on. Must be supplied; there's no default.
	Port string

	// MongoURL and MongoPort make up the MongoDB connection target:
	// mongodb://{MongoURL}:{MongoPort}
	MongoURL  string
	MongoPort string

	// LogLevel controls slog verbosity (DEBUG, INFO, WARN, ERROR).
	LogLevel string

	// HookConnectTimeout / HookRequestTimeout bound outbound webhook calls
	// fired by abstractor.Service. Same two knobs as HOOK_CONNECT_TIMEOUT
	// (connect) and HOOK_REQUEST_TIMEOUT (read) in hook_service.py.
	HookConnectTimeout time.Duration
	HookRequestTimeout time.Duration
}

// Load reads configuration from the process environment.
func Load() Config {
	return Config{
		Port:               os.Getenv("RESOURCE_ABSTRACTOR_PORT"),
		MongoURL:           os.Getenv("MONGO_URL"),
		MongoPort:          os.Getenv("MONGO_PORT"),
		LogLevel:           envOrDefault("LOG_LEVEL", defaultLogLevel),
		HookConnectTimeout: envSecondsOrDefault("HOOK_CONNECT_TIMEOUT", defaultHookConnectSecs),
		HookRequestTimeout: envSecondsOrDefault("HOOK_REQUEST_TIMEOUT", defaultHookRequestSecs),
	}
}

// Validate reports whether the loaded configuration is usable. Without it,
// an empty Port silently binds a random ephemeral port instead of erroring,
// so the service looks healthy while every consumer gets connection-refused.
func (c Config) Validate() error {
	if c.Port == "" {
		return errors.New("RESOURCE_ABSTRACTOR_PORT must be set")
	}
	if !isValidPort(c.Port) {
		return fmt.Errorf("RESOURCE_ABSTRACTOR_PORT must be a port number between 1 and 65535, got %q", c.Port)
	}
	if c.MongoURL == "" || c.MongoPort == "" {
		return errors.New("MONGO_URL and MONGO_PORT must both be set")
	}
	if !isValidPort(c.MongoPort) {
		return fmt.Errorf("MONGO_PORT must be a port number between 1 and 65535, got %q", c.MongoPort)
	}
	return nil
}

func isValidPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

// MongoURI builds the base connection string used to reach MongoDB.
func (c Config) MongoURI() string {
	return "mongodb://" + c.MongoURL + ":" + c.MongoPort
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envSecondsOrDefault(key string, fallbackSeconds int) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return time.Duration(fallbackSeconds) * time.Second
	}

	secs, err := strconv.Atoi(v)
	if err != nil {
		return time.Duration(fallbackSeconds) * time.Second
	}
	return time.Duration(secs) * time.Second
}

// NewLogger builds a text-formatted slog.Logger writing to stdout at the
// level named by levelName. Matched case-insensitively against DEBUG, INFO,
// WARN/WARNING and ERROR; anything else falls back to DEBUG, matching the
// Python service's default.
//
// It doesn't call slog.SetDefault. main wires the returned logger explicitly
// into abstractor.Options and rest.NewHandler instead, so neither the
// library nor the HTTP layer depends on process-global state.
func NewLogger(levelName string) *slog.Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(levelName),
	})
	return slog.New(handler)
}

func parseLevel(levelName string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(levelName)) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelDebug
	}
}
