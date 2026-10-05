// Package canary is superwitness's live acceptance test: ten flows, each
// proven once on a real run. It is a
// black-box client of everything it checks, so it decodes what it reads with its own types.
package canary

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Config is everything the canary reads from its environment. Names shared with the
// service's own configuration keep the SW_ prefix; canary-only names use SWC_.
type Config struct {
	SuperpipelineURL string
	SuperwitnessURL  string
	HubURL           string
	ClientID         string
	ClientSecretFile string
	TracesURL        string
	TracesTokenFile  string
	LogsURL          string
	LogsTokenFile    string
	OTLPLogsURL      string
	BoardID          string
	WorkStage        string
	GateStage        string
	ExpectHarness    string
	Standard         string
	ClaimTimeout     time.Duration
	RunTimeout       time.Duration
	SettleDelay      time.Duration
	DocTimeout       time.Duration
	PollInterval     time.Duration
}

func LoadConfig(getenv func(string) string) (Config, error) {
	var missing []string
	req := func(name string) string {
		v := strings.TrimSpace(getenv(name))
		if v == "" {
			missing = append(missing, name)
		}
		return v
	}
	opt := func(name, def string) string {
		if v := strings.TrimSpace(getenv(name)); v != "" {
			return v
		}
		return def
	}
	c := Config{
		SuperpipelineURL: strings.TrimRight(req("SW_SUPERPIPELINE_URL"), "/"),
		SuperwitnessURL:  strings.TrimRight(req("SWC_SUPERWITNESS_URL"), "/"),
		HubURL:           strings.TrimRight(req("SW_HUB_URL"), "/"),
		ClientID:         req("SWC_HUB_CLIENT_ID"),
		ClientSecretFile: req("SWC_HUB_CLIENT_SECRET_FILE"),
		TracesURL:        strings.TrimRight(req("SW_TRACES_URL"), "/"),
		LogsURL:          strings.TrimRight(req("SW_LOGS_URL"), "/"),
		BoardID:          req("SWC_BOARD_ID"),
		TracesTokenFile:  opt("SW_TRACES_TOKEN_FILE", ""),
		LogsTokenFile:    opt("SW_LOGS_TOKEN_FILE", ""),
		OTLPLogsURL:      opt("SWC_OTLP_LOGS_URL", "http://127.0.0.1:4318/v1/logs"),
		WorkStage:        opt("SWC_WORK_STAGE", "work"),
		GateStage:        opt("SWC_GATE_STAGE", "review"),
		ExpectHarness:    opt("SWC_EXPECT_HARNESS", "hermes"),
		Standard:         opt("SWC_STANDARD", "rubric:superwitness-canary@1"),
	}
	durations := []struct {
		name string
		def  time.Duration
		dst  *time.Duration
	}{
		{"SWC_CLAIM_TIMEOUT", 10 * time.Minute, &c.ClaimTimeout},
		{"SWC_RUN_TIMEOUT", 30 * time.Minute, &c.RunTimeout},
		{"SWC_SETTLE_DELAY", 90 * time.Second, &c.SettleDelay},
		{"SWC_DOC_TIMEOUT", 5 * time.Minute, &c.DocTimeout},
		{"SWC_POLL_INTERVAL", 15 * time.Second, &c.PollInterval},
	}
	var bad []string
	for _, d := range durations {
		*d.dst = d.def
		if v := strings.TrimSpace(getenv(d.name)); v != "" {
			p, err := time.ParseDuration(v)
			if err != nil || p <= 0 {
				bad = append(bad, d.name)
				continue
			}
			*d.dst = p
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return c, fmt.Errorf("missing environment: %s", strings.Join(missing, ", "))
	}
	if len(bad) > 0 {
		return c, fmt.Errorf("not a positive duration: %s", strings.Join(bad, ", "))
	}
	if !strings.HasPrefix(c.Standard, "rubric:") || !strings.Contains(c.Standard, "@") {
		return c, fmt.Errorf("SWC_STANDARD must be rubric:<id>@<version>, got %q", c.Standard)
	}
	return c, nil
}
