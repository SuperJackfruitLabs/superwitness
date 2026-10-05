package api_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

const graderBody = `{"idempotency_key":"canary-2026-10-04","subject_kind":"run","subject_ref":"superpipeline:brd_01/run_01",
  "standard":"stage:review","value":{"score":1},"comment":"canary"}`

func TestPostVerdict(t *testing.T) {
	srv := newTestServer(t, nil)
	resp, b := do(t, srv, "POST", "/v1/verdicts", "dev:prn_canary:service", graderBody)
	if resp.StatusCode != 201 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	var v verdicts.Verdict
	_ = json.Unmarshal(b, &v)
	if v.ID == "" || v.Judge != "prn_canary" || v.JudgeKind != verdicts.JudgeGrader || v.Kind != "grader" {
		t.Errorf("verdict = %+v", v)
	}
	resp, b = do(t, srv, "POST", "/v1/verdicts", "dev:prn_canary:service", graderBody)
	var again verdicts.Verdict
	_ = json.Unmarshal(b, &again)
	if resp.StatusCode != 200 || again.ID != v.ID {
		t.Errorf("replay: %d %s", resp.StatusCode, b)
	}
	// it shows on the document
	_, b = do(t, srv, "GET", runPath, humanTok, "")
	var doc struct {
		Verdicts []struct {
			ID string `json:"id"`
		} `json:"verdicts"`
	}
	_ = json.Unmarshal(b, &doc)
	found := false
	for _, dv := range doc.Verdicts {
		found = found || dv.ID == v.ID
	}
	if !found {
		t.Errorf("verdict %s missing from document: %s", v.ID, b)
	}
}

func TestPostVerdictRefusals(t *testing.T) {
	srv := newTestServer(t, nil)
	if resp, b := do(t, srv, "POST", "/v1/verdicts", "dev:prn_agent01:agent", graderBody); resp.StatusCode != 403 || errCode(t, b) != "self_judgement" {
		t.Errorf("self-judgement: %d %s", resp.StatusCode, b)
	}
	if resp, b := do(t, srv, "POST", "/v1/verdicts", humanTok, `{"idempotency_key":`); resp.StatusCode != 400 || errCode(t, b) != "invalid_json" {
		t.Errorf("malformed: %d %s", resp.StatusCode, b)
	}
	if resp, b := do(t, srv, "POST", "/v1/verdicts", humanTok, `{"idempotency_key":"k","surprise":1}`); resp.StatusCode != 400 || errCode(t, b) != "invalid_json" {
		t.Errorf("unknown field: %d %s", resp.StatusCode, b)
	}
	if resp, _ := do(t, srv, "POST", "/v1/verdicts", "", graderBody); resp.StatusCode != 401 {
		t.Errorf("no token: %d", resp.StatusCode)
	}
}

func TestPostVerdictStoreDown(t *testing.T) {
	srv := newTestServer(t, func(_ *api.Ops, s *verdicts.MemStore) { s.Fail = verdicts.ErrUnavailable })
	resp, b := do(t, srv, "POST", "/v1/verdicts", humanTok, graderBody)
	if resp.StatusCode != 503 || errCode(t, b) != "store_unavailable" || resp.Header.Get("Retry-After") == "" {
		t.Errorf("%d %s %v", resp.StatusCode, b, resp.Header)
	}
	// the document still renders, with gates, and says the store is unavailable
	_, b = do(t, srv, "GET", runPath, humanTok, "")
	var doc struct {
		Sources  map[string]string `json:"sources"`
		Verdicts []json.RawMessage `json:"verdicts"`
	}
	_ = json.Unmarshal(b, &doc)
	if doc.Sources["verdicts"] != "unavailable" || len(doc.Verdicts) != 4 {
		t.Errorf("doc = %s", b)
	}
}

type pinger source.SourceStatus

func (p pinger) Ping(ctx context.Context) source.SourceStatus {
	if p == "slow" {
		<-ctx.Done()
		return source.StatusUnavailable
	}
	return source.SourceStatus(p)
}

func TestHealth(t *testing.T) {
	h := &api.Health{Version: "v1.2.3", Timeout: 50 * time.Millisecond, Pingers: map[string]source.Pinger{
		"traces": pinger(source.StatusOK), "logs": pinger(source.StatusUnauthorized), "verdicts": pinger("slow"),
	}}
	rec := httptest.NewRecorder()
	start := time.Now()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))
	if time.Since(start) > 500*time.Millisecond {
		t.Error("health waited on a slow dependency")
	}
	var body struct {
		Status, Version string
		Sources         map[string]string
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != 200 || body.Status != "ok" || body.Version != "v1.2.3" || body.Sources["traces"] != "ok" ||
		body.Sources["logs"] != "unauthorized" || body.Sources["verdicts"] != "timeout" {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

func TestPostVerdictBodyCap(t *testing.T) {
	srv := newTestServer(t, nil)
	big := `{"idempotency_key":"k","comment":"` + strings.Repeat("x", 70<<10) + `"}`
	if resp, b := do(t, srv, "POST", "/v1/verdicts", humanTok, big); resp.StatusCode != 400 || errCode(t, b) != "invalid_json" || !strings.Contains(string(b), "64 KiB") {
		t.Errorf("oversize: %d %s", resp.StatusCode, b)
	}
	if resp, b := do(t, srv, "POST", "/v1/verdicts", humanTok, graderBody+`{}`); resp.StatusCode != 400 || errCode(t, b) != "invalid_json" {
		t.Errorf("trailing data: %d %s", resp.StatusCode, b)
	}
}

func TestPostVerdictNonRetryableHasNoRetryAfter(t *testing.T) {
	srv := newTestServer(t, nil)
	resp, b := do(t, srv, "POST", "/v1/verdicts", "dev:prn_agent01:agent", graderBody)
	if resp.StatusCode != 403 || resp.Header.Get("Retry-After") != "" {
		t.Errorf("%d %s %v", resp.StatusCode, b, resp.Header)
	}
}
