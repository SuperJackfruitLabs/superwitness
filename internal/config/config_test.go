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

func TestRunSources(t *testing.T) {
	m := full()
	m["SW_RUN_SOURCES"] = " prn_reporter01=superpipeline , prn_reporter02=canary-runner "
	c, err := Load(env(m))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.RunSources) != 2 || c.RunSources["prn_reporter01"] != "superpipeline" || c.RunSources["prn_reporter02"] != "canary-runner" {
		t.Errorf("RunSources = %v", c.RunSources)
	}
	for _, bad := range []string{
		"prn_reporter01",               // no source
		"prn_reporter01=",              // empty source
		"=superpipeline",               // no principal
		"usr_01=superpipeline",         // not a principal id
		"prn_reporter01=Superpipeline", // not a source name
		"prn_reporter01=superpipeline,prn_reporter01=canary", // bound twice
	} {
		m["SW_RUN_SOURCES"] = bad
		if _, err := Load(env(m)); err == nil || !strings.Contains(err.Error(), "SW_RUN_SOURCES") {
			t.Errorf("%q: err = %v; want one naming SW_RUN_SOURCES", bad, err)
		}
	}
	delete(m, "SW_RUN_SOURCES")
	c, err = Load(env(m))
	if err != nil || c.RunSources == nil || len(c.RunSources) != 0 {
		t.Errorf("unset: %v %v; want an empty, non-nil map (nobody may report)", c.RunSources, err)
	}
}

func TestRunSourcesInFakeMode(t *testing.T) {
	c, err := Load(env(map[string]string{"SW_FAKE_SOURCES": "1", "SW_DATABASE_URL": "postgres://x",
		"SW_RUN_SOURCES": "prn_reporter01=superpipeline"}))
	if err != nil || c.RunSources["prn_reporter01"] != "superpipeline" {
		t.Errorf("fake mode: %v %v", c.RunSources, err)
	}
}

func TestAppSettings(t *testing.T) {
	m := full()
	m["SW_PUBLIC_URL"] = "https://app.superwitness.example"
	m["SW_ALLOWED_PRINCIPALS"] = " prn_human01 , prn_human02 "
	m["SW_APP_CLIENT_ID"] = "superwitness-console"
	m["SW_SESSION_SECRET_FILE"] = "/etc/superwitness/session-secret"
	m["SW_TRUSTED_PROXIES"] = "127.0.0.1, 10.0.0.0/8, ::1"
	c, err := Load(env(m))
	if err != nil {
		t.Fatal(err)
	}
	if !c.AppEnabled() || len(c.AllowedPrincipals) != 2 || c.AllowedPrincipals[1] != "prn_human02" ||
		c.AppClientID != "superwitness-console" || c.SessionSecretFile != "/etc/superwitness/session-secret" {
		t.Errorf("app settings = %+v", c)
	}
	if len(c.TrustedProxies) != 3 || c.TrustedProxies[0].String() != "127.0.0.1/32" || c.TrustedProxies[1].String() != "10.0.0.0/8" ||
		c.TrustedProxies[2].String() != "::1/128" {
		t.Errorf("trusted proxies = %v", c.TrustedProxies)
	}

	for name, edit := range map[string]func(map[string]string){
		"a listed id that is not a principal": func(m map[string]string) { m["SW_ALLOWED_PRINCIPALS"] = "prn_human01,usr_01" },
		"no client id":                        func(m map[string]string) { delete(m, "SW_APP_CLIENT_ID") },
		"no session secret":                   func(m map[string]string) { delete(m, "SW_SESSION_SECRET_FILE") },
		"a plain-http public URL":             func(m map[string]string) { m["SW_PUBLIC_URL"] = "http://superwitness.example" },
		"a bad proxy":                         func(m map[string]string) { m["SW_TRUSTED_PROXIES"] = "localhost" },
	} {
		mm := map[string]string{}
		for k, v := range m {
			mm[k] = v
		}
		edit(mm)
		if _, err := Load(env(mm)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	// Sign-in off: nobody listed. The other app settings are then not required.
	off := full()
	if c, err := Load(env(off)); err != nil || c.AppEnabled() || c.TrustedProxies != nil {
		t.Errorf("off: %+v %v", c, err)
	}
}

func TestAppSettingsInFakeMode(t *testing.T) {
	base := map[string]string{"SW_FAKE_SOURCES": "1", "SW_DATABASE_URL": "postgres://x", "SW_PUBLIC_URL": "http://127.0.0.1:8790",
		"SW_ALLOWED_PRINCIPALS": "prn_human01", "SW_APP_CLIENT_ID": "superwitness-console", "SW_SESSION_SECRET_FILE": "/tmp/s"}
	if _, err := Load(env(base)); err == nil || !strings.Contains(err.Error(), "SW_HUB_URL") {
		t.Errorf("fake mode without a hub: %v; sign-in needs one", err)
	}
	base["SW_HUB_URL"] = "http://127.0.0.1:8791"
	if c, err := Load(env(base)); err != nil || !c.AppEnabled() {
		t.Errorf("fake mode over loopback http: %v", err)
	}
}
