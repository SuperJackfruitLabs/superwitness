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
		if strings.Contains(string(b), `"index"`) {
			t.Errorf("%s: a single report's error carries an index: %s", c.name, b)
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
	var e2 struct {
		Error struct {
			Code  string `json:"code"`
			Index *int   `json:"index"`
		} `json:"error"`
	}
	_ = json.Unmarshal(b, &e2)
	if resp.StatusCode != 403 || e2.Error.Code != "source_not_allowed" || e2.Error.Index == nil || *e2.Error.Index != 1 {
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

type listed struct {
	Runs []struct {
		ExternalRef   string          `json:"external_ref"`
		Ref           string          `json:"ref"`
		Status        string          `json:"status"`
		LatestVerdict json.RawMessage `json:"latest_verdict"`
	} `json:"runs"`
	NextCursor *string        `json:"next_cursor"`
	Counts     map[string]int `json:"counts"`
}

func seedRuns(t *testing.T, r *registry) {
	t.Helper()
	var items []string
	for i, st := range []string{"running", "succeeded", "failed"} {
		rep := strings.Replace(runReport, "run_01", fmt.Sprintf("run_%02d", i+1), 1)
		rep = strings.Replace(rep, `"running"`, `"`+st+`"`, 1)
		rep = strings.Replace(rep, "09:00:00Z", fmt.Sprintf("09:0%d:00Z", i), 1)
		items = append(items, rep)
	}
	if resp, b := do(t, r.srv, "POST", "/v1/runs", reporterTok, `{"runs":[`+strings.Join(items, ",")+`]}`); resp.StatusCode != 200 {
		t.Fatalf("seed: %d %s", resp.StatusCode, b)
	}
}

func getList(t *testing.T, r *registry, query string) listed {
	t.Helper()
	resp, b := do(t, r.srv, "GET", "/v1/runs"+query, humanTok, "")
	if resp.StatusCode != 200 {
		t.Fatalf("GET /v1/runs%s: %d %s", query, resp.StatusCode, b)
	}
	var l listed
	if err := json.Unmarshal(b, &l); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestListRuns(t *testing.T) {
	r := newRegistryServer(t)
	seedRuns(t, r)
	l := getList(t, r, "")
	if len(l.Runs) != 3 || l.Runs[0].ExternalRef != "brd_01/run_03" || l.Runs[0].Ref != "superpipeline:brd_01/run_03" ||
		l.NextCursor != nil || l.Counts["running"] != 1 || l.Counts["queued"] != 0 || string(l.Runs[0].LatestVerdict) != "null" {
		t.Errorf("list = %+v", l)
	}
	l = getList(t, r, "?status=succeeded&status=failed")
	if len(l.Runs) != 2 || l.Counts["running"] != 1 {
		t.Errorf("status filter (counts ignore it) = %+v", l)
	}
	l = getList(t, r, "?needs_verdict=true")
	if len(l.Runs) != 2 {
		t.Errorf("needs_verdict = %+v", l)
	}
	l = getList(t, r, "?limit=2")
	if len(l.Runs) != 2 || l.NextCursor == nil {
		t.Fatalf("limit 2 = %+v", l)
	}
	l = getList(t, r, "?limit=2&cursor="+*l.NextCursor)
	if len(l.Runs) != 1 || l.Runs[0].ExternalRef != "brd_01/run_01" || l.NextCursor != nil {
		t.Errorf("page 2 = %+v", l)
	}
	l = getList(t, r, "?limit=5000")
	if len(l.Runs) != 3 {
		t.Errorf("an oversized limit is clamped, not refused: %+v", l)
	}
	l = getList(t, r, "?since=2026-10-06T09:01:00Z&until=2026-10-06T09:02:00Z")
	if len(l.Runs) != 1 || l.Runs[0].ExternalRef != "brd_01/run_02" {
		t.Errorf("since/until = %+v", l)
	}
	// An agent token reads too: the audience is the grant.
	if resp, b := do(t, r.srv, "GET", "/v1/runs", "dev:prn_agent01:agent", ""); resp.StatusCode != 200 {
		t.Errorf("agent read: %d %s", resp.StatusCode, b)
	}
}

func TestListRunsShowsTheLatestVerdict(t *testing.T) {
	r := newRegistryServer(t)
	seedRuns(t, r)
	body := `{"idempotency_key":"k-list","subject_kind":"run","subject_ref":"superpipeline:brd_01/run_01","standard":"stage:review","value":{"decision":"pass"}}`
	if resp, b := do(t, r.srv, "POST", "/v1/verdicts", humanTok, body); resp.StatusCode != 201 {
		t.Fatalf("verdict: %d %s", resp.StatusCode, b)
	}
	for _, run := range getList(t, r, "").Runs {
		if run.ExternalRef == "brd_01/run_01" && !strings.Contains(string(run.LatestVerdict), `"decision":"pass"`) {
			t.Errorf("latest_verdict = %s", run.LatestVerdict)
		}
	}
}

func TestListRunsRefusals(t *testing.T) {
	r := newRegistryServer(t)
	for q, code := range map[string]string{
		"?status=done":           "invalid_status",
		"?source=Superpipeline":  "invalid_source",
		"?since=yesterday":       "invalid_time",
		"?until=2026-10-06":      "invalid_time",
		"?cursor=garbage":        "invalid_cursor",
		"?limit=0":               "invalid_limit",
		"?limit=-1":              "invalid_limit",
		"?needs_verdict=perhaps": "invalid_needs_verdict",
	} {
		if resp, b := do(t, r.srv, "GET", "/v1/runs"+q, humanTok, ""); resp.StatusCode != 400 || errCode(t, b) != code {
			t.Errorf("%s: %d %s; want 400 %s", q, resp.StatusCode, b, code)
		}
	}
	if resp, _ := do(t, r.srv, "GET", "/v1/runs", "", ""); resp.StatusCode != 401 {
		t.Errorf("no token: %d", resp.StatusCode)
	}
	r.runs.Fail = runs.ErrUnavailable
	if resp, b := do(t, r.srv, "GET", "/v1/runs", humanTok, ""); resp.StatusCode != 503 || errCode(t, b) != "store_unavailable" {
		t.Errorf("store down: %d %s", resp.StatusCode, b)
	}
}
