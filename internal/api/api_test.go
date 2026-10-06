package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/join"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/fake"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

const (
	humanTok = "dev:prn_human01:human"
	runPath  = "/v1/runs/superpipeline/brd_01/run_01"
)

func newTestServer(t *testing.T, mutate func(*api.Ops, *verdicts.MemStore)) *httptest.Server {
	t.Helper()
	d, err := fake.NewDev()
	if err != nil {
		t.Fatal(err)
	}
	store := verdicts.NewMemStore()
	ops := &api.Ops{
		Join: &join.Joiner{Superpipeline: d.SP, AgentPod: d.AP, Traces: d.Traces, Logs: d.Logs, Errors: d.Errors,
			Verdicts: store, Principals: d},
		Spans: d, Logs: d, Attempts: d,
		Verdicts: &verdicts.Service{Store: store, Subjects: &join.Subjects{Superpipeline: d.SP, AgentPod: d.AP, Attempts: d}},
	}
	if mutate != nil {
		mutate(ops, store)
	}
	srv := httptest.NewServer((&api.Server{Ops: ops, Auth: auth.DevAuthenticator{}}).Handler())
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, srv *httptest.Server, method, path, token, body string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func errCode(t *testing.T, b []byte) string {
	t.Helper()
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(b, &e)
	return e.Error.Code
}

func TestGetRun(t *testing.T) {
	srv := newTestServer(t, nil)
	resp, b := do(t, srv, "GET", runPath, humanTok, "")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	var doc join.RunDocument
	if err := json.Unmarshal(b, &doc); err != nil || doc.Run.Ref != "superpipeline:brd_01/run_01" {
		t.Errorf("doc = %+v, %v", doc.Run, err)
	}
}

func TestGetRunErrors(t *testing.T) {
	srv := newTestServer(t, nil)
	if resp, b := do(t, srv, "GET", "/v1/runs/superpipeline/brd_01/run_99", humanTok, ""); resp.StatusCode != 404 || errCode(t, b) != "run_not_found" {
		t.Errorf("not found: %d %s", resp.StatusCode, b)
	}
	if resp, b := do(t, srv, "GET", "/v1/runs/superpipeline/brd_01/run%2201", humanTok, ""); resp.StatusCode != 400 || errCode(t, b) != "invalid_run_ref" {
		t.Errorf("bad ref: %d %s", resp.StatusCode, b)
	}
	if resp, _ := do(t, srv, "GET", runPath, "", ""); resp.StatusCode != 401 {
		t.Errorf("no token: %d", resp.StatusCode)
	}
	if resp, b := do(t, srv, "GET", "/v1/nope", humanTok, ""); resp.StatusCode != 404 || errCode(t, b) != "not_found" {
		t.Errorf("unknown route: %d %s", resp.StatusCode, b)
	}
}

func spanPage(t *testing.T, srv *httptest.Server, query string) (int, api.SpanPage, string) {
	t.Helper()
	resp, b := do(t, srv, "GET", runPath+"/spans"+query, humanTok, "")
	var p api.SpanPage
	_ = json.Unmarshal(b, &p)
	return resp.StatusCode, p, errCode(t, b)
}

// Span paging at its edges.
func TestSpanPagingEdges(t *testing.T) {
	srv := newTestServer(t, nil)
	code, p1, _ := spanPage(t, srv, "?limit=5")
	if code != 200 || len(p1.Spans) != 5 || p1.NextCursor == "" {
		t.Fatalf("page 1: %d %+v", code, p1)
	}
	code, p2, _ := spanPage(t, srv, "?limit=5&cursor="+p1.NextCursor)
	if code != 200 || len(p2.Spans) != 2 || p2.NextCursor != "" {
		t.Errorf("page 2: %d %+v", code, p2)
	}
	past := base64.RawURLEncoding.EncodeToString([]byte(`{"o":99}`))
	if code, p, _ := spanPage(t, srv, "?cursor="+past); code != 200 || len(p.Spans) != 0 || p.NextCursor != "" || p.Spans == nil {
		t.Errorf("past the end: %d %+v", code, p)
	}
	if code, p, _ := spanPage(t, srv, "?limit=99999"); code != 200 || len(p.Spans) != 7 {
		t.Errorf("clamped limit: %d %d", code, len(p.Spans))
	}
	if code, _, ec := spanPage(t, srv, "?limit=-1"); code != 400 || ec != "invalid_limit" {
		t.Errorf("negative limit: %d %s", code, ec)
	}
	if code, _, ec := spanPage(t, srv, "?cursor=not-base64!"); code != 400 || ec != "invalid_cursor" {
		t.Errorf("garbage cursor: %d %s", code, ec)
	}
	neg := base64.RawURLEncoding.EncodeToString([]byte(`{"o":-5}`))
	if code, _, ec := spanPage(t, srv, "?cursor="+neg); code != 400 || ec != "invalid_cursor" {
		t.Errorf("negative offset: %d %s", code, ec)
	}
}

// manySpans returns more spans than one page may hold, so the limit clamp is observable.
type manySpans struct{ n int }

func (m manySpans) ListSpans(context.Context, source.RunRef) ([]source.Span, source.SourceStatus) {
	out := make([]source.Span, m.n)
	for i := range out {
		out[i] = source.Span{TraceID: "t1", SpanID: fmt.Sprintf("s%04d", i), Name: "op", Service: "svc"}
	}
	return out, source.StatusOK
}

// An oversized limit is clamped to MaxPageSize (500), not honoured.
func TestSpanLimitClampedTo500(t *testing.T) {
	srv := newTestServer(t, func(o *api.Ops, _ *verdicts.MemStore) { o.Spans = manySpans{n: 1200} })
	code, p, _ := spanPage(t, srv, "?limit=99999")
	if code != 200 || len(p.Spans) != api.MaxPageSize || api.MaxPageSize != 500 || p.NextCursor == "" {
		t.Fatalf("limit=99999: %d, %d spans, next_cursor %q; want 200, 500 spans and a next_cursor", code, len(p.Spans), p.NextCursor)
	}
	code, p2, _ := spanPage(t, srv, "?limit=99999&cursor="+p.NextCursor)
	if code != 200 || len(p2.Spans) != 500 || p2.NextCursor == "" || p2.Spans[0].SpanID != "s0500" {
		t.Errorf("page 2: %d, %d spans, next_cursor %q", code, len(p2.Spans), p2.NextCursor)
	}
}

type stuckSpans struct{}

func (stuckSpans) ListSpans(context.Context, source.RunRef) ([]source.Span, source.SourceStatus) {
	return nil, source.StatusTimeout
}

func TestSpansSourceFailure(t *testing.T) {
	srv := newTestServer(t, func(o *api.Ops, _ *verdicts.MemStore) { o.Spans = stuckSpans{} })
	resp, b := do(t, srv, "GET", runPath+"/spans", humanTok, "")
	if resp.StatusCode != 504 || errCode(t, b) != "traces_timeout" {
		t.Errorf("%d %s", resp.StatusCode, b)
	}
	// logs still work without the trace join, and say so
	resp, b = do(t, srv, "GET", runPath+"/logs", humanTok, "")
	var p api.LogPage
	_ = json.Unmarshal(b, &p)
	if resp.StatusCode != 200 || p.TraceJoin != source.StatusTimeout {
		t.Errorf("logs: %d %s", resp.StatusCode, b)
	}
}

func TestLogs(t *testing.T) {
	srv := newTestServer(t, nil)
	resp, b := do(t, srv, "GET", runPath+"/logs?level=error", humanTok, "")
	var p api.LogPage
	_ = json.Unmarshal(b, &p)
	if resp.StatusCode != 200 || len(p.Logs) != 1 || p.Logs[0].Level != "error" || p.TraceJoin != source.StatusOK || p.NextCursor != nil {
		t.Errorf("%d %s", resp.StatusCode, b)
	}
	resp, b = do(t, srv, "GET", runPath+"/logs?limit=1", humanTok, "")
	p = api.LogPage{}
	_ = json.Unmarshal(b, &p)
	if len(p.Logs) != 1 || p.NextCursor == nil || *p.NextCursor == "" {
		t.Errorf("paged logs: %s", b)
	}
	if !strings.Contains(string(b), `"next_cursor":"`) {
		t.Errorf("next_cursor missing: %s", b)
	}
	resp, b = do(t, srv, "GET", runPath+"/logs?limit=1&cursor="+*p.NextCursor, humanTok, "")
	p = api.LogPage{}
	_ = json.Unmarshal(b, &p)
	if resp.StatusCode != 200 || len(p.Logs) != 1 || p.NextCursor != nil || !strings.Contains(string(b), `"next_cursor":null`) {
		t.Errorf("last log page: %d %s", resp.StatusCode, b)
	}
	if resp, b := do(t, srv, "GET", runPath+"/logs?level=verbose", humanTok, ""); resp.StatusCode != 400 || errCode(t, b) != "invalid_level" {
		t.Errorf("bad level: %d %s", resp.StatusCode, b)
	}
}

func TestByAttempt(t *testing.T) {
	srv := newTestServer(t, nil)
	resp, _ := do(t, srv, "GET", "/v1/runs/by-attempt/attempt_01", humanTok, "")
	if resp.StatusCode != 302 || resp.Header.Get("Location") != runPath {
		t.Errorf("%d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp, b := do(t, srv, "GET", "/v1/runs/by-attempt/attempt_99", humanTok, ""); resp.StatusCode != 404 || errCode(t, b) != "attempt_not_found" {
		t.Errorf("unknown: %d %s", resp.StatusCode, b)
	}
	if resp, b := do(t, srv, "GET", "/v1/runs/by-attempt/run_01", humanTok, ""); resp.StatusCode != 400 || errCode(t, b) != "invalid_attempt_id" {
		t.Errorf("invalid: %d %s", resp.StatusCode, b)
	}
}

func TestLogPagingEdges(t *testing.T) {
	srv := newTestServer(t, nil)
	neg := base64.RawURLEncoding.EncodeToString([]byte(`{"o":-5}`))
	past := base64.RawURLEncoding.EncodeToString([]byte(`{"o":99}`))
	for query, want := range map[string]string{"?limit=-1": "invalid_limit", "?limit=abc": "invalid_limit",
		"?cursor=" + neg: "invalid_cursor", "?cursor=%21%21": "invalid_cursor"} {
		if resp, b := do(t, srv, "GET", runPath+"/logs"+query, humanTok, ""); resp.StatusCode != 400 || errCode(t, b) != want {
			t.Errorf("%s: %d %s", query, resp.StatusCode, b)
		}
	}
	resp, b := do(t, srv, "GET", runPath+"/logs?cursor="+past, humanTok, "")
	if resp.StatusCode != 200 || !strings.Contains(string(b), `"logs":[]`) || !strings.Contains(string(b), `"next_cursor":null`) {
		t.Errorf("past the end: %d %s", resp.StatusCode, b)
	}
}
