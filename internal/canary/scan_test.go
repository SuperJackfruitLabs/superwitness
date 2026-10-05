package canary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

var winStart = time.Date(2026, 10, 10, 2, 29, 0, 0, time.UTC)

func TestLogsScanCountsHitsWithoutEchoingTheNeedle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/select/logsql/query" {
			t.Errorf("path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if q := r.PostForm.Get("query"); q != "_time:[2026-10-10T02:29:00Z, 2026-10-10T02:50:00Z]" {
			t.Errorf("query %q", q)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer logs-token" {
			t.Errorf("auth %q", got)
		}
		fmt.Fprintln(w, `{"_time":"2026-10-10T02:31:00Z","_stream":"{service=\"agentpod-hub\"}","_msg":"claimed run_1"}`)
		fmt.Fprintln(w, `{"_time":"2026-10-10T02:32:00Z","_stream":"{service=\"superpipeline-api\"}","_msg":"POST /cards","body":"task swcanaryabc"}`)
		fmt.Fprintln(w)
	}))
	defer srv.Close()
	tok := filepath.Join(t.TempDir(), "tok")
	if err := os.WriteFile(tok, []byte("logs-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := LogsClient{BaseURL: srv.URL, TokenFile: tok}.Scan(context.Background(), winStart, winStart.Add(21*time.Minute),
		map[string]string{LabelPlain: "swcanaryabc", LabelControlRun: "run_1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 2 || res.Hits[LabelPlain] != 1 || res.Hits[LabelControlRun] != 1 {
		t.Fatalf("res = %+v", res)
	}
	if w := res.Where[LabelPlain]; !strings.Contains(w, "fields=body") || strings.Contains(w, "swcanaryabc") {
		t.Errorf("where = %q", w)
	}
}

func TestLogsScanFailsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer srv.Close()
	_, err := LogsClient{BaseURL: srv.URL}.Scan(context.Background(), winStart, winStart.Add(time.Minute), nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("err = %v", err)
	}
}

const hubTrace = `{"data":[{"traceID":"t1","spans":[
 {"spanID":"s1","operationName":"dispatch","processID":"p1","tags":[{"key":"run.id","type":"string","value":"run_1"}]},
 {"spanID":"s2","operationName":"attempt","processID":"p1","tags":[{"key":"attempt.id","type":"string","value":"attempt_1"}]},
 {"spanID":"s3","operationName":"turn","processID":"p1","tags":[{"key":"attempt.id","type":"string","value":"attempt_1"},{"key":"acp.seq_from","type":"int64","value":3}]}],
 "processes":{"p1":{"serviceName":"agentpod-hub","tags":[]}}}]}`

const leakyTrace = `{"data":[{"traceID":"t2","spans":[
 {"spanID":"s9","operationName":"POST /v1/boards/:id/cards","processID":"p1","tags":[{"key":"http.request.body","type":"string","value":"sk-swcanary-xyz"}]}],
 "processes":{"p1":{"serviceName":"superpipeline-api","tags":[]}}}]}`

func fakeJaeger(t *testing.T, perService map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/select/jaeger/api/services":
			var names []string
			for k := range perService {
				names = append(names, k)
			}
			sort.Strings(names)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": names})
		case r.URL.Path == "/select/jaeger/api/traces":
			qs := r.URL.Query()
			if tags := qs.Get("tags"); tags != "" {
				if tags != `{"run.id":"run_1"}` || qs.Get("service") != "superpipeline-api" {
					t.Errorf("tag search %s", r.URL.RawQuery)
				}
				fmt.Fprint(w, workersTrace)
				return
			}
			if qs.Get("start") == "" || qs.Get("end") == "" || qs.Get("limit") == "" {
				t.Errorf("search query %s", r.URL.RawQuery)
			}
			body, ok := perService[qs.Get("service")]
			if !ok {
				body = `{"data":[]}`
			}
			fmt.Fprint(w, body)
		case r.URL.Path == "/select/jaeger/api/traces/t1":
			fmt.Fprint(w, hubTrace)
		default:
			fmt.Fprint(w, `{"data":[]}`)
		}
	}))
}

func TestTracesTraceConvertsJaegerSpans(t *testing.T) {
	srv := fakeJaeger(t, map[string]string{"agentpod-hub": hubTrace})
	defer srv.Close()
	spans, err := TracesClient{BaseURL: srv.URL}.Trace(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 3 || spans[2].Service != "agentpod-hub" || spans[2].Operation != "turn" || spans[2].Tags["acp.seq_from"] != "3" {
		t.Fatalf("spans = %+v", spans)
	}
	if _, err := (TracesClient{BaseURL: srv.URL}).Trace(context.Background(), "missing"); err == nil {
		t.Error("missing trace accepted")
	}
}

func TestTracesScanLocatesALeakAndTheControl(t *testing.T) {
	srv := fakeJaeger(t, map[string]string{"agentpod-hub": hubTrace, "superpipeline-api": leakyTrace})
	defer srv.Close()
	res, err := TracesClient{BaseURL: srv.URL, SearchLimit: 50}.Scan(context.Background(), winStart, winStart.Add(time.Hour),
		map[string]string{LabelSecretMarker: "sk-swcanary-xyz", LabelControlRun: "run_1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Hits[LabelControlRun] != 1 || res.Hits[LabelSecretMarker] != 1 || res.Truncated {
		t.Fatalf("res = %+v", res)
	}
	if w := res.Where[LabelSecretMarker]; w != "service=superpipeline-api span=POST /v1/boards/:id/cards key=http.request.body" {
		t.Errorf("where = %q", w)
	}
}

const workersTrace = `{"data":[{"traceID":"t3","spans":[
 {"spanID":"w1","operationName":"POST /v1/boards/:id/runs/:runId/activities","processID":"p1","tags":[{"key":"run.id","type":"string","value":"run_1"}]}],
 "processes":{"p1":{"serviceName":"superpipeline-api","tags":[]}}}]}`

func TestTracesFindByTagReturnsSpansAndTheQuery(t *testing.T) {
	srv := fakeJaeger(t, map[string]string{"agentpod-hub": hubTrace})
	defer srv.Close()
	spans, query, err := TracesClient{BaseURL: srv.URL}.FindByTag(context.Background(), "superpipeline-api", "run.id", "run_1",
		winStart, winStart.Add(time.Hour))
	if err != nil || len(spans) != 1 || spans[0].Service != "superpipeline-api" || spans[0].Tags["run.id"] != "run_1" {
		t.Fatalf("spans=%+v err=%v", spans, err)
	}
	if !strings.HasPrefix(query, "GET /select/jaeger/api/traces?") || !strings.Contains(query, "tags=") {
		t.Errorf("query = %q", query)
	}
}

func TestTracesScanReportsTruncationAndEmptyIndex(t *testing.T) {
	srv := fakeJaeger(t, map[string]string{"agentpod-hub": hubTrace})
	defer srv.Close()
	res, err := TracesClient{BaseURL: srv.URL, SearchLimit: 1}.Scan(context.Background(), winStart, winStart.Add(time.Hour), nil)
	if err != nil || !res.Truncated {
		t.Fatalf("res = %+v err = %v", res, err)
	}
	empty := fakeJaeger(t, map[string]string{})
	defer empty.Close()
	if _, err := (TracesClient{BaseURL: empty.URL}).Scan(context.Background(), winStart, winStart.Add(time.Hour), nil); err == nil {
		t.Error("an index with no services was accepted")
	}
}

func scanWith(hits map[string]int) ScanResult {
	s := newScanResult()
	s.Scanned = 100
	for k, v := range hits {
		s.Hits[k] = v
		s.Where[k] = "somewhere"
	}
	return s
}

func TestCheckMarkerAbsent(t *testing.T) {
	cleanLogs := scanWith(map[string]int{LabelControlRun: 2})
	cleanTraces := scanWith(map[string]int{LabelControlRun: 1})
	truncated := cleanTraces
	truncated.Truncated = true
	cases := map[string]struct {
		logs, traces       ScanResult
		logsErr, tracesErr error
		pass               bool
		detail             string
	}{
		"clean":                {cleanLogs, cleanTraces, nil, nil, true, "marker absent"},
		"control via trace id": {scanWith(map[string]int{LabelControlTrace: 1}), cleanTraces, nil, nil, true, "marker absent"},
		"logs error":           {cleanLogs, cleanTraces, errors.New("HTTP 502"), nil, false, "absence unproven"},
		"traces error":         {cleanLogs, cleanTraces, nil, errors.New("HTTP 502"), false, "absence unproven"},
		"truncated":            {cleanLogs, truncated, nil, nil, false, "hit its limit"},
		"blind logs":           {scanWith(nil), cleanTraces, nil, nil, false, "may be blind"},
		"blind traces":         {cleanLogs, scanWith(nil), nil, nil, false, "may be blind"},
		"plain in logs":        {scanWith(map[string]int{LabelControlRun: 1, LabelPlain: 3}), cleanTraces, nil, nil, false, "marker_plain in 3 log line(s)"},
		"secret in span":       {cleanLogs, scanWith(map[string]int{LabelControlRun: 1, LabelSecretMarker: 1}), nil, nil, false, "marker_secret in 1 span(s)"},
		"leak beats blindness": {scanWith(map[string]int{LabelPlain: 1}), scanWith(nil), nil, nil, false, "content leaked"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := CheckMarkerAbsent(tc.logs, tc.logsErr, tc.traces, tc.tracesErr)
			if c.ID != "9" || c.Pass != tc.pass || !strings.Contains(c.Detail, tc.detail) {
				t.Fatalf("got %+v", c)
			}
		})
	}
}

func TestWhereNeverCarriesANeedle(t *testing.T) {
	const m = "swcanaryleak"
	logSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, `{"_time":"2026-10-10T02:31:00Z","_stream":"{pod=\"x-`+m+`\"}","_msg":"hello `+m+`"}`)
	}))
	defer logSrv.Close()
	lres, err := LogsClient{BaseURL: logSrv.URL}.Scan(context.Background(), winStart, winStart.Add(time.Minute), map[string]string{LabelPlain: m})
	if err != nil || lres.Hits[LabelPlain] != 1 {
		t.Fatalf("logs res=%+v err=%v", lres, err)
	}
	trace := `{"data":[{"traceID":"t9","spans":[{"spanID":"s","operationName":"POST /cards/` + m + `","processID":"p1","tags":[]}],"processes":{"p1":{"serviceName":"svc","tags":[]}}}]}`
	trSrv := fakeJaeger(t, map[string]string{"svc": trace})
	defer trSrv.Close()
	tres, err := TracesClient{BaseURL: trSrv.URL}.Scan(context.Background(), winStart, winStart.Add(time.Hour), map[string]string{LabelPlain: m})
	if err != nil || tres.Hits[LabelPlain] != 1 {
		t.Fatalf("traces res=%+v err=%v", tres, err)
	}
	for name, w := range map[string]string{"logs": lres.Where[LabelPlain], "traces": tres.Where[LabelPlain]} {
		if strings.Contains(w, m) || !strings.Contains(w, "<"+LabelPlain+">") {
			t.Errorf("%s where = %q", name, w)
		}
	}
	c := CheckMarkerAbsent(lres, nil, tres, nil)
	if c.Pass || strings.Contains(c.Detail, m) {
		t.Errorf("detail = %q", c.Detail)
	}
}
