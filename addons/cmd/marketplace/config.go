package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oakestra/oakestra/addons/internal/marketplace/source"
)

// DefaultOfficialIndex is where community-marketplace CI publishes the official index.
const DefaultOfficialIndex = "https://oakestra.github.io/community-marketplace/index.json"

type config struct {
	Port          int
	MongoURI      string
	OfficialURL   string // empty when off
	OfficialToken string
	SyncInterval  time.Duration
	CORSOrigin    string
	// RegistryAllowPrivate permits resolving image digests from registries on private or
	// loopback addresses, over plain http.
	RegistryAllowPrivate bool
}

// String leaves the official token out, so the config can be logged.
func (c config) String() string {
	off := c.OfficialURL
	if off == "" {
		off = "off"
	}
	return fmt.Sprintf("port=%d mongo=%s official=%s official_token=%t sync_interval=%s cors_origin=%q registry_allow_private=%t",
		c.Port, c.MongoURI, off, c.OfficialToken != "", c.SyncInterval, c.CORSOrigin, c.RegistryAllowPrivate)
}

// loadConfig reads the environment. Where a variable already existed, the name is kept so
// existing compose files keep working.
func loadConfig(getenv func(string) string) (config, error) {
	env := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	var c config

	port, err := strconv.Atoi(env("ADDON_MARKETPLACE_PORT", "11102"))
	if err != nil || port < 1 || port > 65535 {
		return c, fmt.Errorf("ADDON_MARKETPLACE_PORT must be a port number")
	}
	c.Port = port

	mongoPort, err := strconv.Atoi(env("ADDON_MARKETPLACE_MONGO_PORT", "10007"))
	if err != nil || mongoPort < 1 || mongoPort > 65535 {
		return c, fmt.Errorf("ADDON_MARKETPLACE_MONGO_PORT must be a port number")
	}
	c.MongoURI = fmt.Sprintf("mongodb://%s:%d", env("ADDON_MARKETPLACE_MONGO_URL", "localhost"), mongoPort)

	official := env("MARKETPLACE_OFFICIAL_INDEX", DefaultOfficialIndex)
	if !strings.EqualFold(official, "off") {
		// http is allowed here because the operator sets it, e.g. for a dev index on the LAN.
		// Private addresses are allowed for the same reason (see main.go).
		if _, err := source.CheckURL(official, true, true); err != nil {
			return c, fmt.Errorf("MARKETPLACE_OFFICIAL_INDEX: %w", err)
		}
		c.OfficialURL = official
	}
	c.OfficialToken = strings.TrimSpace(getenv("MARKETPLACE_OFFICIAL_TOKEN"))

	interval, err := time.ParseDuration(env("MARKETPLACE_SYNC_INTERVAL", "6h"))
	if err != nil || interval < time.Minute {
		return c, fmt.Errorf("MARKETPLACE_SYNC_INTERVAL must be a duration of at least 1m (e.g. 6h)")
	}
	c.SyncInterval = interval
	c.CORSOrigin = env("MARKETPLACE_CORS_ORIGIN", "")
	allowPrivate, err := strconv.ParseBool(env("MARKETPLACE_REGISTRY_ALLOW_PRIVATE", "false"))
	if err != nil {
		return c, fmt.Errorf("MARKETPLACE_REGISTRY_ALLOW_PRIVATE must be true or false")
	}
	c.RegistryAllowPrivate = allowPrivate
	return c, nil
}
