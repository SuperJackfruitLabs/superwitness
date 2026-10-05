//go:build integration

package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/config"
	"github.com/SuperJackfruitLabs/superwitness/internal/testutil"
)

// The built binary, as the runtime role: a reporter reports, a human lists, judges, and the run
// leaves needs_verdict.
func TestRegistryEndToEnd(t *testing.T) {
	roles := testutil.StartPostgresWithRoles(t)
	cfg, err := config.Load(envOf(map[string]string{"SW_FAKE_SOURCES": "1", "SW_DATABASE_URL": roles.AppDSN,
		"SW_MIGRATE_DATABASE_URL": roles.OwnerDSN, "SW_RUN_SOURCES": "prn_reporter01=superpipeline"}))
	if err != nil {
		t.Fatal(err)
	}
	a, err := Build(context.Background(), cfg, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()

	call := func(method, path, tok, body string) (int, string) {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		_, b := call("GET", "/health", "", "")
		if strings.Contains(b, `"verdicts":"ok"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never migrated: %s", b)
		}
		time.Sleep(100 * time.Millisecond)
	}
	roles.GrantRuntime(t)

	report := `{"source":"superpipeline","external_ref":"brd_01/run_01","status":"succeeded","source_status":"done",
		"started_at":"2026-10-06T09:00:00Z","reported_at":"` + time.Now().UTC().Format(time.RFC3339) + `"}`
	if code, b := call("POST", "/v1/runs", "dev:prn_reporter01:service:runs:write", report); code != 200 || !strings.Contains(b, `"applied":true`) {
		t.Fatalf("report: %d %s", code, b)
	}
	if code, b := call("GET", "/v1/runs?needs_verdict=true", "dev:prn_human01:human", ""); code != 200 || !strings.Contains(b, `"brd_01/run_01"`) {
		t.Fatalf("needs_verdict before: %d %s", code, b)
	}
	verdict := `{"idempotency_key":"k-registry","subject_kind":"run","subject_ref":"superpipeline:brd_01/run_01","standard":"stage:review","value":{"decision":"pass"}}`
	if code, b := call("POST", "/v1/verdicts", "dev:prn_human01:human", verdict); code != 201 {
		t.Fatalf("verdict: %d %s", code, b)
	}
	if code, b := call("GET", "/v1/runs?needs_verdict=true", "dev:prn_human01:human", ""); code != 200 || strings.Contains(b, `"brd_01/run_01"`) {
		t.Errorf("needs_verdict after: %d %s", code, b)
	}
	if code, b := call("GET", "/v1/runs", "dev:prn_human01:human", ""); !strings.Contains(b, `"latest_verdict":{"id":"vrd_`) {
		t.Errorf("latest_verdict: %d %s", code, b)
	}
	if code, b := call("GET", "/v1/rubrics", "dev:prn_human01:human", ""); code != 200 || !strings.Contains(b, `"rubrics":[]`) {
		t.Errorf("rubrics: %d %s", code, b)
	}
}
