package canary

import (
	"strings"
	"testing"
	"time"
)

func fullEnv() map[string]string {
	return map[string]string{
		"SW_SUPERPIPELINE_URL":       "https://app.superpipeline.dev/",
		"SWC_SUPERWITNESS_URL":       "http://127.0.0.1:8790",
		"SW_HUB_URL":                 "https://hub.example/",
		"SWC_HUB_CLIENT_ID":          "svc_canary",
		"SWC_HUB_CLIENT_SECRET_FILE": "/etc/superwitness/canary-client-secret",
		"SW_TRACES_URL":              "http://127.0.0.1:10428",
		"SW_LOGS_URL":                "http://127.0.0.1:9428",
		"SWC_BOARD_ID":               "brd_canary",
	}
}

func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadConfigAppliesDefaults(t *testing.T) {
	c, err := LoadConfig(envFrom(fullEnv()))
	if err != nil {
		t.Fatal(err)
	}
	if c.SuperpipelineURL != "https://app.superpipeline.dev" || c.HubURL != "https://hub.example" {
		t.Errorf("urls = %q / %q", c.SuperpipelineURL, c.HubURL)
	}
	if c.OTLPLogsURL != "http://127.0.0.1:4318/v1/logs" {
		t.Errorf("otlp logs url = %q", c.OTLPLogsURL)
	}
	if c.WorkStage != "work" || c.GateStage != "review" || c.ExpectHarness != "hermes" {
		t.Errorf("stage/harness defaults = %q %q %q", c.WorkStage, c.GateStage, c.ExpectHarness)
	}
	if c.Standard != "rubric:superwitness-canary@1" {
		t.Errorf("standard = %q", c.Standard)
	}
	if c.ClaimTimeout != 10*time.Minute || c.RunTimeout != 30*time.Minute || c.SettleDelay != 90*time.Second ||
		c.DocTimeout != 5*time.Minute || c.PollInterval != 15*time.Second {
		t.Errorf("durations = %v %v %v %v %v", c.ClaimTimeout, c.RunTimeout, c.SettleDelay, c.DocTimeout, c.PollInterval)
	}
}

func TestLoadConfigNamesEveryMissingVariable(t *testing.T) {
	env := fullEnv()
	delete(env, "SWC_BOARD_ID")
	delete(env, "SW_LOGS_URL")
	_, err := LoadConfig(envFrom(env))
	if err == nil || !strings.Contains(err.Error(), "SWC_BOARD_ID, SW_LOGS_URL") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadConfigRejectsBadDurationsAndStandards(t *testing.T) {
	for name, val := range map[string]string{"SWC_RUN_TIMEOUT": "soon", "SWC_POLL_INTERVAL": "-1m", "SWC_STANDARD": "canary"} {
		env := fullEnv()
		env[name] = val
		if _, err := LoadConfig(envFrom(env)); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s=%q: err = %v", name, val, err)
		}
	}
}
