package main

import (
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/oakestra/oakestra/addons/internal/addons/reconcile"
	"github.com/oakestra/oakestra/addons/internal/addons/runtime"
	"github.com/oakestra/oakestra/addons/internal/addons/statedir"
	"github.com/oakestra/oakestra/addons/internal/addons/updates"
)

// config is read from the environment. Variable names that existing deployments already set
// are kept as they are.
type config struct {
	Port              int
	MongoURI          string
	MarketplaceAddr   string
	Plane             string
	ReconcileInterval time.Duration
	UpdateInterval    time.Duration
	CORSOrigin        string
	LogLevel          slog.Level
	// StateDir holds the grant key and the instance id. It must be a volume of its own, not
	// shared with Mongo or any addon.
	StateDir string
	// Limits apply to every addon container.
	Limits runtime.Limits
}

// Default limits for addon containers. Generous for a sidecar-style addon, small next to the
// RAM an edge host has, and each can be raised or disabled per deployment.
const (
	defaultPidsLimit   = "1024"
	defaultMemoryLimit = "512m"
	defaultCPULimit    = "1"
	// minMemoryBytes is docker's lower bound for a memory limit.
	minMemoryBytes = 6 << 20
)

func loadConfig(getenv func(string) string) (config, error) {
	get := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}
	var c config
	var err error

	if c.Port, err = strconv.Atoi(get("ADDONS_MANAGER_PORT", "11101")); err != nil || c.Port < 1 || c.Port > 65535 {
		return c, fmt.Errorf("ADDONS_MANAGER_PORT must be a port number")
	}
	c.MongoURI = fmt.Sprintf("mongodb://%s:%s", get("ADDONS_ENGINE_MONGO_URL", "localhost"), get("ADDONS_ENGINE_MONGO_PORT", "10007"))
	c.MarketplaceAddr = strings.TrimRight(get("MARKETPLACE_ADDR", "http://localhost:11102"), "/")

	c.Plane = get("ORCHESTRATION_PLANE", "root")
	if c.Plane != "root" && c.Plane != "cluster" {
		return c, fmt.Errorf("ORCHESTRATION_PLANE must be root or cluster, got %q", c.Plane)
	}
	if c.ReconcileInterval, err = parseDuration(get("RECONCILE_INTERVAL", ""), reconcile.DefaultInterval); err != nil {
		return c, fmt.Errorf("RECONCILE_INTERVAL: %w", err)
	}
	if c.UpdateInterval, err = parseDuration(get("UPDATE_CHECK_INTERVAL", ""), updates.DefaultInterval); err != nil {
		return c, fmt.Errorf("UPDATE_CHECK_INTERVAL: %w", err)
	}
	c.CORSOrigin = get("ADDONS_CORS_ORIGIN", "*")
	c.StateDir = get("ADDONS_STATE_DIR", statedir.DefaultDir)

	if c.Limits.PidsLimit, err = strconv.ParseInt(get("ADDON_PIDS_LIMIT", defaultPidsLimit), 10, 64); err != nil || c.Limits.PidsLimit < 0 {
		return c, fmt.Errorf("ADDON_PIDS_LIMIT must be a non-negative integer (0 disables the limit)")
	}
	if c.Limits.MemoryBytes, err = parseMemory(get("ADDON_MEMORY_LIMIT", defaultMemoryLimit)); err != nil {
		return c, fmt.Errorf("ADDON_MEMORY_LIMIT: %w", err)
	}
	if c.Limits.NanoCPUs, err = parseCPUs(get("ADDON_CPU_LIMIT", defaultCPULimit)); err != nil {
		return c, fmt.Errorf("ADDON_CPU_LIMIT: %w", err)
	}

	c.LogLevel = slog.LevelInfo
	if v := getenv("LOG_LEVEL"); v != "" {
		if err := c.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return c, fmt.Errorf("LOG_LEVEL: %w", err)
		}
	}
	return c, nil
}

// parseDuration accepts Go durations ("15s") and plain seconds ("15"), since existing
// deployments set the intervals in seconds.
func parseDuration(s string, def time.Duration) (time.Duration, error) {
	if s == "" {
		return def, nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		s = strconv.Itoa(n) + "s"
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d < time.Second {
		return 0, fmt.Errorf("%s is shorter than 1s", d)
	}
	return d, nil
}

// parseMemory accepts a byte count with an optional k, m or g suffix (binary units, like
// docker's --memory), and "0" or "unlimited" for no limit.
func parseMemory(s string) (int64, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	if v == "0" || v == "unlimited" {
		return 0, nil
	}
	v = strings.TrimSuffix(v, "b")
	mult := int64(1)
	switch {
	case strings.HasSuffix(v, "k"):
		mult = 1 << 10
	case strings.HasSuffix(v, "m"):
		mult = 1 << 20
	case strings.HasSuffix(v, "g"):
		mult = 1 << 30
	}
	if mult != 1 {
		v = v[:len(v)-1]
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 || n > math.MaxInt64/mult {
		return 0, fmt.Errorf("%q is not a size like 512m or 1g (0 or unlimited disables the limit)", s)
	}
	if n*mult < minMemoryBytes {
		return 0, fmt.Errorf("%q is below docker's minimum of 6m", s)
	}
	return n * mult, nil
}

// parseCPUs accepts a number of CPUs, fractions allowed, and "0" for no limit.
func parseCPUs(s string) (int64, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) || f > 1e6 {
		return 0, fmt.Errorf("%q is not a number of CPUs like 1 or 0.5 (0 disables the limit)", s)
	}
	n := int64(math.Round(f * 1e9))
	if f > 0 && n == 0 {
		return 0, fmt.Errorf("%q is too small", s)
	}
	return n, nil
}
