//go:build integration

package app

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/config"
	"github.com/SuperJackfruitLabs/superwitness/internal/testutil"
)

func TestBuildFakeMode(t *testing.T) {
	env := map[string]string{"SW_FAKE_SOURCES": "1", "SW_DATABASE_URL": testutil.StartPostgres(t)} // one container, not one per getenv
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	var logs lockedBuffer
	a, err := Build(context.Background(), cfg, "test", slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()

	get := func(path, tok string) (int, string) {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	// Migrations run in the background; the store reports unavailable until they finish.
	deadline := time.Now().Add(30 * time.Second)
	for {
		code, body := get("/health", "")
		if code == 200 && strings.Contains(body, `"verdicts":"ok"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("health never reported verdicts ok: %d %s", code, body)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if code, body := get("/v1/runs/superpipeline/brd_01/run_01", "dev:prn_human01:human"); code != 200 || !strings.Contains(body, `"trace_ids"`) {
		t.Errorf("run: %d %s", code, body)
	}
	if code, body := get("/v1/runs/superpipeline/brd_01/run_01/transcript", "dev:prn_grader01:agent:transcripts:read"); code != 200 ||
		!strings.Contains(body, "[redacted:anthropic-key]") {
		t.Errorf("transcript: %d %s", code, body)
	}
	if code, body := get("/v1/runs/superpipeline/brd_01/run_01/transcript", "dev:prn_grader01:agent"); code != 403 ||
		!strings.Contains(body, "transcripts_forbidden") {
		t.Errorf("transcript without the scope: %d %s", code, body)
	}
	if n := strings.Count(logs.String(), `"msg":"transcript.read"`); n != 2 {
		t.Errorf("%d transcript.read lines on the binary's logger, want 2", n)
	}
	if code, body := get("/runs/superpipeline/brd_01/run_01", ""); code != 200 || !strings.Contains(body, `<div id="root">`) {
		t.Errorf("page: %d", code)
	}
	// /v1/* and /mcp never fall through to the index.html fallback.
	for _, p := range []string{"/v1/foo", "/v1/runs/nope", "/mcp/x"} {
		code, body := get(p, "dev:prn_human01:human")
		if code != 404 || strings.Contains(body, `<div id="root">`) || !strings.Contains(body, `"not_found"`) {
			t.Errorf("%s: %d %s; want the API's JSON 404", p, code, body)
		}
	}
	if code, body := get("/v1/foo", ""); code != 404 || strings.Contains(body, `<div id="root">`) {
		t.Errorf("unauthenticated /v1/foo: %d %s", code, body)
	}
	req, _ := http.NewRequest("POST", srv.URL+"/v1/verdicts", strings.NewReader(
		`{"idempotency_key":"k1","subject_kind":"run","subject_ref":"superpipeline:brd_01/run_01","standard":"stage:review","value":{"decision":"approved"}}`))
	req.Header.Set("Authorization", "Bearer dev:prn_human01:human")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Errorf("post verdict: %d", resp.StatusCode)
	}
}

// lockedBuffer collects log output from the server's goroutines.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuffer) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }
