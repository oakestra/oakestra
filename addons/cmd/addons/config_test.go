package main

import (
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoadConfigDefaults(t *testing.T) {
	c, err := loadConfig(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 11101 || c.Plane != "root" || c.ReconcileInterval != 15*time.Second || c.UpdateInterval != 10*time.Minute ||
		c.MongoURI != "mongodb://localhost:10007" || c.MarketplaceAddr != "http://localhost:11102" ||
		c.StateDir != "/var/lib/oakestra-addons" {
		t.Errorf("%+v", c)
	}
	if c.Limits.PidsLimit != 1024 || c.Limits.MemoryBytes != 512<<20 || c.Limits.NanoCPUs != 1e9 {
		t.Errorf("limits = %+v", c.Limits)
	}
}

func TestLoadConfigLimits(t *testing.T) {
	c, err := loadConfig(env(map[string]string{"ADDON_PIDS_LIMIT": "0", "ADDON_MEMORY_LIMIT": "unlimited", "ADDON_CPU_LIMIT": "0", "ADDONS_STATE_DIR": "/state"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Limits.PidsLimit != 0 || c.Limits.MemoryBytes != 0 || c.Limits.NanoCPUs != 0 || c.StateDir != "/state" {
		t.Errorf("%+v", c)
	}
	for in, want := range map[string]int64{"1g": 1 << 30, "256M": 256 << 20, "8192k": 8 << 20, "1gb": 1 << 30, "0": 0, "10485760": 10 << 20} {
		if got, err := parseMemory(in); err != nil || got != want {
			t.Errorf("parseMemory(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for in, want := range map[string]int64{"0.5": 5e8, "2": 2e9, "0": 0} {
		if got, err := parseCPUs(in); err != nil || got != want {
			t.Errorf("parseCPUs(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
}

func TestLoadConfigValues(t *testing.T) {
	c, err := loadConfig(env(map[string]string{
		"ADDONS_MANAGER_PORT": "11201", "ADDONS_ENGINE_MONGO_URL": "cluster_mongo", "ADDONS_ENGINE_MONGO_PORT": "10107",
		"MARKETPLACE_ADDR": "http://marketplace_manager:11102/", "ORCHESTRATION_PLANE": "cluster",
		"RECONCILE_INTERVAL": "30", "UPDATE_CHECK_INTERVAL": "1h",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 11201 || c.MongoURI != "mongodb://cluster_mongo:10107" || c.MarketplaceAddr != "http://marketplace_manager:11102" ||
		c.Plane != "cluster" || c.ReconcileInterval != 30*time.Second || c.UpdateInterval != time.Hour {
		t.Errorf("%+v", c)
	}
}

func TestLoadConfigRejects(t *testing.T) {
	for k, v := range map[string]string{
		"ORCHESTRATION_PLANE": "edge", "ADDONS_MANAGER_PORT": "x", "RECONCILE_INTERVAL": "0", "UPDATE_CHECK_INTERVAL": "soon", "LOG_LEVEL": "loud",
	} {
		if _, err := loadConfig(env(map[string]string{k: v})); err == nil {
			t.Errorf("%s=%s accepted", k, v)
		}
	}
	for k, vs := range map[string][]string{
		"ADDON_PIDS_LIMIT":   {"-1", "many"},
		"ADDON_MEMORY_LIMIT": {"-1m", "1t", "lots", "1k", "99999999999g"},
		"ADDON_CPU_LIMIT":    {"-1", "NaN", "one", "1e-12"},
	} {
		for _, v := range vs {
			if _, err := loadConfig(env(map[string]string{k: v})); err == nil {
				t.Errorf("%s=%s accepted", k, v)
			}
		}
	}
}
