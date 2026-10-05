package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func full() map[string]string {
	return map[string]string{
		"SW_DATABASE_URL":           "postgres://sw@127.0.0.1:5433/superwitness",
		"SW_PUBLIC_URL":             "https://superwitness.example/",
		"SW_SUPERPIPELINE_URL":      "https://app.superpipeline.dev/",
		"SW_HUB_URL":                "https://hub.agentpod.dev",
		"SW_HUB_CLIENT_ID":          "superwitness",
		"SW_HUB_CLIENT_SECRET_FILE": "/etc/superwitness/hub-client-secret",
		"SW_TRACES_URL":             "http://127.0.0.1:10428",
		"SW_LOGS_URL":               "http://127.0.0.1:9428",
	}
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(env(full()))
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":8790" {
		t.Errorf("Listen = %q", c.Listen)
	}
	if c.SourceTimeout != 2*time.Second {
		t.Errorf("SourceTimeout = %v", c.SourceTimeout)
	}
	if c.SuperpipelineURL != "https://app.superpipeline.dev" || c.PublicURL != "https://superwitness.example" {
		t.Errorf("trailing slash not trimmed: %q %q", c.SuperpipelineURL, c.PublicURL)
	}
	if c.FakeSources {
		t.Error("FakeSources defaulted to true")
	}
}

func TestLoadNamesEveryMissingSetting(t *testing.T) {
	_, err := Load(env(map[string]string{}))
	if err == nil {
		t.Fatal("want error")
	}
	for _, name := range []string{"SW_DATABASE_URL", "SW_PUBLIC_URL", "SW_SUPERPIPELINE_URL", "SW_HUB_URL",
		"SW_HUB_CLIENT_ID", "SW_HUB_CLIENT_SECRET_FILE", "SW_TRACES_URL", "SW_LOGS_URL"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
}

func TestLoadFakeModeNeedsOnlyDatabaseAndBindsLoopback(t *testing.T) {
	c, err := Load(env(map[string]string{"SW_FAKE_SOURCES": "1", "SW_DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:8790" || !c.FakeSources {
		t.Errorf("got %+v", c)
	}
	if c.PublicURL != "http://127.0.0.1:8790" {
		t.Errorf("PublicURL = %q", c.PublicURL)
	}
}

func TestLoadFakeModeRefusesPublicListen(t *testing.T) {
	_, err := Load(env(map[string]string{"SW_FAKE_SOURCES": "true", "SW_DATABASE_URL": "postgres://x", "SW_LISTEN": ":8790"}))
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("want loopback refusal, got %v", err)
	}
}

func TestLoadRejectsBadTimeoutAndURL(t *testing.T) {
	m := full()
	m["SW_SOURCE_TIMEOUT"] = "-1s"
	if _, err := Load(env(m)); err == nil || !strings.Contains(err.Error(), "SW_SOURCE_TIMEOUT") {
		t.Errorf("timeout: %v", err)
	}
	m = full()
	m["SW_TRACES_URL"] = "10428"
	if _, err := Load(env(m)); err == nil || !strings.Contains(err.Error(), "SW_TRACES_URL") {
		t.Errorf("url: %v", err)
	}
	m = full()
	m["SW_FAKE_SOURCES"] = "maybe"
	if _, err := Load(env(m)); err == nil || !strings.Contains(err.Error(), "SW_FAKE_SOURCES") {
		t.Errorf("bool: %v", err)
	}
}

func TestMigrateDSNPrefersMigrateURL(t *testing.T) {
	m := full()
	m["SW_MIGRATE_DATABASE_URL"] = "  postgres://owner@127.0.0.1:5433/superwitness  "
	c, err := Load(env(m))
	if err != nil {
		t.Fatal(err)
	}
	if c.MigrateDatabaseURL != "postgres://owner@127.0.0.1:5433/superwitness" {
		t.Errorf("MigrateDatabaseURL = %q; want it trimmed", c.MigrateDatabaseURL)
	}
	if c.MigrateDSN() != c.MigrateDatabaseURL || c.DatabaseURL != m["SW_DATABASE_URL"] {
		t.Errorf("MigrateDSN = %q, DatabaseURL = %q", c.MigrateDSN(), c.DatabaseURL)
	}
}

func TestMigrateDSNFallsBackToDatabaseURL(t *testing.T) {
	c, err := Load(env(full()))
	if err != nil {
		t.Fatal(err)
	}
	if c.MigrateDSN() != c.DatabaseURL {
		t.Errorf("MigrateDSN = %q; want SW_DATABASE_URL", c.MigrateDSN())
	}
}

func TestLoadMigrateNeedsOnlyADatabaseURL(t *testing.T) {
	dsn, err := LoadMigrate(env(map[string]string{"SW_MIGRATE_DATABASE_URL": " postgres://owner ", "SW_DATABASE_URL": "postgres://app"}))
	if err != nil || dsn != "postgres://owner" {
		t.Errorf("both set: %q %v", dsn, err)
	}
	dsn, err = LoadMigrate(env(map[string]string{"SW_DATABASE_URL": "postgres://app"}))
	if err != nil || dsn != "postgres://app" {
		t.Errorf("fallback: %q %v", dsn, err)
	}
	_, err = LoadMigrate(env(map[string]string{}))
	if err == nil || !strings.Contains(err.Error(), "SW_MIGRATE_DATABASE_URL") || !strings.Contains(err.Error(), "SW_DATABASE_URL") {
		t.Errorf("neither set: %v", err)
	}
}
