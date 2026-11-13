package main

import (
	"strings"
	"testing"
	"time"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadConfigDefaults(t *testing.T) {
	c, err := loadConfig(envOf(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 11102 || c.MongoURI != "mongodb://localhost:10007" || c.OfficialURL != DefaultOfficialIndex ||
		c.SyncInterval != 6*time.Hour || c.CORSOrigin != "" || c.OfficialToken != "" {
		t.Errorf("defaults: %+v", c)
	}
}

func TestLoadConfig(t *testing.T) {
	c, err := loadConfig(envOf(map[string]string{
		"ADDON_MARKETPLACE_PORT":       "9000",
		"ADDON_MARKETPLACE_MONGO_URL":  "mongo",
		"ADDON_MARKETPLACE_MONGO_PORT": "27017",
		"MARKETPLACE_OFFICIAL_INDEX":   "OFF",
		"MARKETPLACE_OFFICIAL_TOKEN":   "ghp_secret",
		"MARKETPLACE_SYNC_INTERVAL":    "30m",
		"MARKETPLACE_CORS_ORIGIN":      "https://dash.test",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 9000 || c.MongoURI != "mongodb://mongo:27017" || c.OfficialURL != "" || c.SyncInterval != 30*time.Minute {
		t.Errorf("config: %+v", c)
	}
	if strings.Contains(c.String(), "ghp_secret") {
		t.Error("String() leaks the token")
	}

	if c, err := loadConfig(envOf(map[string]string{"MARKETPLACE_OFFICIAL_INDEX": "file:///data/index.json"})); err != nil || c.OfficialURL != "file:///data/index.json" {
		t.Errorf("file URL: %v", err)
	}
	for k, v := range map[string]string{
		"ADDON_MARKETPLACE_PORT":     "nope",
		"MARKETPLACE_SYNC_INTERVAL":  "5s",
		"MARKETPLACE_OFFICIAL_INDEX": "ftp://example.test/index.json",
	} {
		if _, err := loadConfig(envOf(map[string]string{k: v})); err == nil {
			t.Errorf("%s=%s accepted", k, v)
		}
	}
}

func TestRegistryAllowPrivate(t *testing.T) {
	c, err := loadConfig(envOf(nil))
	if err != nil || c.RegistryAllowPrivate {
		t.Fatalf("default must be off: %+v %v", c, err)
	}
	c, err = loadConfig(envOf(map[string]string{"MARKETPLACE_REGISTRY_ALLOW_PRIVATE": "true"}))
	if err != nil || !c.RegistryAllowPrivate {
		t.Fatalf("true: %+v %v", c, err)
	}
	if _, err := loadConfig(envOf(map[string]string{"MARKETPLACE_REGISTRY_ALLOW_PRIVATE": "maybe"})); err == nil {
		t.Error("expected an error for a non-boolean")
	}
}
