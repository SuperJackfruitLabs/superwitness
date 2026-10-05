package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/join"
	"github.com/SuperJackfruitLabs/superwitness/internal/runs"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/fake"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

const (
	reporterTok = "dev:prn_reporter01:service:runs:write"
	runReport   = `{"source":"superpipeline","external_ref":"brd_01/run_01","scope":{"id":"brd_01","name":"Press"},
		"title":"Draft the release note","status":"running","source_status":"in_progress",
		"started_at":"2026-10-06T09:00:00Z","reported_at":"2026-10-06T09:00:01Z"}`
)

var registryNow = time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)

type registry struct {
	srv   *httptest.Server
	runs  *runs.MemStore
	store *verdicts.MemStore
	log   *bytes.Buffer
}

func newRegistryServer(t *testing.T) *registry {
	t.Helper()
	d, err := fake.NewDev()
	if err != nil {
		t.Fatal(err)
	}
	store := verdicts.NewMemStore()
	rs := runs.NewMemStore(store)
	ops := &api.Ops{
		Join: &join.Joiner{Superpipeline: d.SP, AgentPod: d.AP, Traces: d.Traces, Logs: d.Logs, Errors: d.Errors,
			Verdicts: store, Principals: d},
		Spans: d, Logs: d, Attempts: d,
		Verdicts:   &verdicts.Service{Store: store, Subjects: &join.Subjects{Superpipeline: d.SP, AgentPod: d.AP, Attempts: d}},
		Runs:       rs,
		RunSources: map[string]string{"prn_reporter01": "superpipeline"},
		Rubrics:    nil,
		Now:        func() time.Time { return registryNow },
	}
	var buf bytes.Buffer
	srv := httptest.NewServer((&api.Server{Ops: ops, Auth: auth.DevAuthenticator{},
		Logger: slog.New(slog.NewJSONHandler(&buf, nil))}).Handler())
	t.Cleanup(srv.Close)
	return &registry{srv: srv, runs: rs, store: store, log: &buf}
}

func TestReportRun(t *testing.T) {
	r := newRegistryServer(t)
	resp, b := do(t, r.srv, "POST", "/v1/runs", reporterTok, runReport)
	if resp.StatusCode != 200 || strings.TrimSpace(string(b)) != `{"results":[{"external_ref":"brd_01/run_01","applied":true}]}` {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	resp, b = do(t, r.srv, "POST", "/v1/runs", reporterTok, runReport)
	if resp.StatusCode != 200 || !strings.Contains(string(b), `"applied":false`) {
		t.Errorf("replay: %d %s", resp.StatusCode, b)
	}
	batch := `{"runs":[` + strings.Replace(runReport, "run_01", "run_02", 1) + `,` + strings.Replace(runReport, "run_01", "run_03", 1) + `]}`
	resp, b = do(t, r.srv, "POST", "/v1/runs", reporterTok, batch)
	if resp.StatusCode != 200 || strings.Count(string(b), `"applied":true`) != 2 ||
		strings.Index(string(b), "run_02") > strings.Index(string(b), "run_03") {
		t.Errorf("batch: %d %s; want both applied, in request order", resp.StatusCode, b)
	}
}

func TestReportRunRefusals(t *testing.T) {
	r := newRegistryServer(t)
	bad := strings.Replace(runReport, `"running"`, `"done"`, 1)
	for _, c := range []struct {
		name, tok, body string
		status          int
		code            string
	}{
		{"a human", humanTok, runReport, 403, "service_principal_required"},
		{"an agent", "dev:prn_agent01:agent:runs:write", runReport, 403, "service_principal_required"},
		{"a service without the scope", "dev:prn_reporter01:service:evidence:read", runReport, 403, "insufficient_scope"},
		{"an unbound service", "dev:prn_other01:service:runs:write", runReport, 403, "source_not_allowed"},
		{"another source", reporterTok, strings.Replace(runReport, `"superpipeline"`, `"canary"`, 1), 403, "source_not_allowed"},
		{"not JSON", reporterTok, `{"source":`, 400, "invalid_json"},
		{"a top-level array", reporterTok, `[` + runReport + `]`, 400, "invalid_json"},
		{"an invalid report", reporterTok, bad, 422, "invalid_report"},
		{"no token", "", runReport, 401, "unauthenticated"},
	} {
		resp, b := do(t, r.srv, "POST", "/v1/runs", c.tok, c.body)
		if resp.StatusCode != c.status || errCode(t, b) != c.code {
			t.Errorf("%s: %d %s; want %d %s", c.name, resp.StatusCode, b, c.status, c.code)
		}
	}
	if p, _ := r.runs.List(t.Context(), runs.Filter{Limit: 10}); len(p.Runs) != 0 {
		t.Errorf("a refused report was written: %+v", p.Runs)
	}
}

func TestReportBatchIsAllOrNothing(t *testing.T) {
	r := newRegistryServer(t)
	bad := strings.Replace(strings.Replace(runReport, "run_01", "run_02", 1), `"running"`, `"done"`, 1)
	resp, b := do(t, r.srv, "POST", "/v1/runs", reporterTok, `{"runs":[`+runReport+`,`+bad+`]}`)
	var e struct {
		Error struct {
			Code  string `json:"code"`
			Index *int   `json:"index"`
		} `json:"error"`
	}
	_ = json.Unmarshal(b, &e)
	if resp.StatusCode != 422 || e.Error.Code != "invalid_report" || e.Error.Index == nil || *e.Error.Index != 1 {
		t.Fatalf("%d %s; want 422 invalid_report at index 1", resp.StatusCode, b)
	}
	// A wrong source on the second item: 403 with its index, and the first is not written either.
	other := strings.Replace(strings.Replace(runReport, "run_01", "run_02", 1), `"superpipeline"`, `"canary"`, 1)
	resp, b = do(t, r.srv, "POST", "/v1/runs", reporterTok, `{"runs":[`+runReport+`,`+other+`]}`)
	_ = json.Unmarshal(b, &e)
	if resp.StatusCode != 403 || e.Error.Code != "source_not_allowed" || e.Error.Index == nil || *e.Error.Index != 1 {
		t.Errorf("%d %s", resp.StatusCode, b)
	}
	if p, _ := r.runs.List(t.Context(), runs.Filter{Limit: 10}); len(p.Runs) != 0 {
		t.Errorf("part of a refused batch was written: %+v", p.Runs)
	}
}

func TestReportLimits(t *testing.T) {
	r := newRegistryServer(t)
	items := make([]string, 101)
	for i := range items {
		items[i] = strings.Replace(runReport, "run_01", fmt.Sprintf("run_%03d", i), 1)
	}
	if resp, b := do(t, r.srv, "POST", "/v1/runs", reporterTok, `{"runs":[`+strings.Join(items, ",")+`]}`); resp.StatusCode != 400 || errCode(t, b) != "invalid_json" {
		t.Errorf("101 reports: %d %s", resp.StatusCode, b)
	}
	huge := strings.Replace(runReport, `"in_progress"`, `"`+strings.Repeat("x", runs.MaxBodyBytes)+`"`, 1)
	if resp, b := do(t, r.srv, "POST", "/v1/runs", reporterTok, huge); resp.StatusCode != 413 || errCode(t, b) != "body_too_large" {
		t.Errorf("over 256 KiB: %d %s", resp.StatusCode, b)
	}
}

func TestReportStoreDown(t *testing.T) {
	r := newRegistryServer(t)
	r.runs.Fail = runs.ErrUnavailable
	resp, b := do(t, r.srv, "POST", "/v1/runs", reporterTok, runReport)
	if resp.StatusCode != 503 || errCode(t, b) != "store_unavailable" || resp.Header.Get("Retry-After") == "" {
		t.Errorf("%d %s", resp.StatusCode, b)
	}
}

func TestReportRejectionIsLoggedWithoutSecrets(t *testing.T) {
	r := newRegistryServer(t)
	do(t, r.srv, "POST", "/v1/runs", "dev:prn_reporter01:service", runReport)
	line := r.log.String()
	if !strings.Contains(line, `"msg":"runs.report_rejected"`) || !strings.Contains(line, `"code":"insufficient_scope"`) ||
		!strings.Contains(line, `"principal":"prn_reporter01"`) {
		t.Errorf("audit line = %s", line)
	}
	if strings.Contains(line, "dev:") || strings.Contains(line, "Draft the release note") {
		t.Errorf("the audit line carries the token or the body: %s", line)
	}
}
