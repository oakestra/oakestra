// Package config centralizes environment-variable configuration for the
// resource abstractor, mirroring the env vars read by the Python service
// (resource-abstractor/db/mongodb_client.py, resource_abstractor.py and
// services/hook_service.py).
package config

import (
	"os"
	"strconv"
	"time"
)

const (
	defaultLogLevel        = "DEBUG"
	defaultHookConnectSecs = 10
	defaultHookRequestSecs = 5
)

// Config holds all runtime configuration read from the environment.
type Config struct {
	// Port the HTTP server listens on. No default in the Python service either
	// (it must be supplied by the deployment environment).
	Port string

	// MongoURL and MongoPort make up the MongoDB connection target:
	// mongodb://{MongoURL}:{MongoPort}
	MongoURL  string
	MongoPort string

	// LogLevel controls slog verbosity (DEBUG, INFO, WARN, ERROR).
	LogLevel string

	// HookConnectTimeout / HookRequestTimeout bound outbound webhook calls
	// fired by services.Hooks. They mirror HOOK_CONNECT_TIMEOUT (connect)
	// and HOOK_REQUEST_TIMEOUT (read) from hook_service.py.
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
	if v := os.Getenv(key); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	return time.Duration(fallbackSeconds) * time.Second
}
