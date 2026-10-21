package traces

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/contracts"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

var fixedNow = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func runRef(t *testing.T) source.RunRef {
	t.Helper()
	r, err := source.NewSuperpipelineRef("brd_01", "run_01")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSearchURL(t *testing.T) {
	a := New("http://vt:10428", "", nil)
	a.Now = func() time.Time { return fixedNow }
	want := "http://vt:10428/select/jaeger/api/traces?end=1791118800000000&limit=100&service=agentpod-hub" +
		"&start=1790510400000000&tags=%7B%22run.id%22%3A%22run_01%22%7D"
	if got := a.SearchURL("agentpod-hub", "run_01"); got != want {
		t.Errorf("SearchURL =\n %s\nwant\n %s", got, want)
	}
}

func jaegerServer(t *testing.T, byService map[string]string, status map[string]int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/select/jaeger/api/traces/") {
			id := strings.TrimPrefix(r.URL.Path, "/select/jaeger/api/traces/")
			for _, raw := range byService {
				var body jaegerResponse
				if err := json.Unmarshal([]byte(raw), &body); err != nil {
					t.Error(err)
					continue
				}
				for _, tr := range body.Data {
					if padHex(tr.TraceID, 32) == id {
						_ = json.NewEncoder(w).Encode(jaegerResponse{Data: []jaegerTrace{tr}})
						return
					}
				}
			}
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/select/jaeger/api/traces" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("tags"); got != `{"run.id":"run_01"}` {
			t.Errorf("tags = %s", got)
		}
		svc := r.URL.Query().Get("service")
		if code, ok := status[svc]; ok {
			w.WriteHeader(code)
			return
		}
		body, ok := byService[svc]
		if !ok {
			body = `{"data":[]}`
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestListSpansMergesServices(t *testing.T) {
	srv := jaegerServer(t, map[string]string{
		"agentpod-hub":        string(contracts.JaegerTraceHub),
		"agentpod-node-agent": string(contracts.JaegerTraceHub), // same trace again: must dedupe
		"superpipeline-api":   string(contracts.JaegerTraceWorkers),
	}, nil)
	a := New(srv.URL, "", srv.Client())
	spans, st := a.ListSpans(context.Background(), runRef(t))
	if st != source.StatusOK {
		t.Fatalf("status = %s", st)
	}
	if len(spans) != 4 {
		t.Fatalf("spans = %d, want 4", len(spans))
	}
	names := []string{spans[0].Name, spans[1].Name, spans[2].Name}
	if names[0] != "dispatch" || names[1] != "attempt" || names[2] != "turn" {
		t.Errorf("order = %v", names)
	}
	if spans[0].ParentSpanID != "" || spans[1].ParentSpanID != "00f067aa0ba902b7" {
		t.Errorf("parents = %q %q", spans[0].ParentSpanID, spans[1].ParentSpanID)
	}
	if spans[1].Attributes["acp.seq_from"] != "1" || spans[1].Attributes["attempt.id"] != "attempt_01" {
		t.Errorf("attrs = %v", spans[1].Attributes)
	}
	if spans[0].Service != "agentpod-hub" || spans[3].Service != "superpipeline-api" {
		t.Errorf("services = %s %s", spans[0].Service, spans[3].Service)
	}
	if spans[0].DurationMS != 300000 || !spans[0].Start.Equal(time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("timing = %v %v", spans[0].DurationMS, spans[0].Start)
	}
	f, _ := a.Fetch(context.Background(), runRef(t))
	if ids := f.Traces.TraceIDs; len(ids) != 2 || ids[0] != contracts.TraceHub || ids[1] != contracts.TraceWorkers {
		t.Errorf("trace ids = %v", ids)
	}
}

func TestListSpansFailures(t *testing.T) {
	for svcStatus, want := range map[int]source.SourceStatus{500: source.StatusUnavailable, 401: source.StatusUnauthorized} {
		srv := jaegerServer(t, nil, map[string]int{"superpipeline-api": svcStatus})
		a := New(srv.URL, "", srv.Client())
		if _, st := a.ListSpans(context.Background(), runRef(t)); st != want {
			t.Errorf("HTTP %d: %s, want %s", svcStatus, st, want)
		}
	}
	srv := jaegerServer(t, nil, map[string]int{"agentpod-node-agent": 404})
	a := New(srv.URL, "", srv.Client())
	if spans, st := a.ListSpans(context.Background(), runRef(t)); st != source.StatusOK || len(spans) != 0 {
		t.Errorf("a service that never reported: %s %d", st, len(spans))
	}
}

func TestEmptyIsOK(t *testing.T) {
	srv := jaegerServer(t, nil, nil)
	a := New(srv.URL, "", srv.Client())
	f, st := a.Fetch(context.Background(), runRef(t))
	if st != source.StatusOK || len(f.Traces.Spans) != 0 || f.Traces.TraceIDs == nil {
		t.Errorf("status = %s, fragment = %+v", st, f.Traces)
	}
}

func TestBearerAndPaddedIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer vt-read" {
			w.WriteHeader(401)
			return
		}
		fmt.Fprint(w, `{"data":[{"traceID":"bf92f3577b34da6a3ce929d0e0e4736","processes":{"p1":{"serviceName":"agentpod-hub"}},
		  "spans":[{"traceID":"bf92f3577b34da6a3ce929d0e0e4736","spanID":"f067aa0ba902b7","operationName":"dispatch",
		  "references":[],"startTime":1791108000000000,"duration":1,"processID":"p1","tags":[]}]}]}`)
	}))
	defer srv.Close()
	a := New(srv.URL, "vt-read", srv.Client())
	a.Services = []string{"agentpod-hub"}
	spans, st := a.ListSpans(context.Background(), runRef(t))
	if st != source.StatusOK || spans[0].TraceID != "0bf92f3577b34da6a3ce929d0e0e4736" || spans[0].SpanID != "00f067aa0ba902b7" {
		t.Errorf("status = %s, span = %+v", st, spans)
	}
}

func TestListSpansJoinsFullTrace(t *testing.T) {
	const id = "cb0a0022bba3718d014901a38054eb09"
	var lookups atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer vt-read" {
			t.Error("missing bearer")
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/select/jaeger/api/traces" {
			fmt.Fprint(w, `{"data":[{"traceID":"`+strings.ToUpper(id)+`","spans":[{"traceID":"`+id+`","spanID":"1","operationName":"heartbeat","processID":"hub"}],"processes":{"hub":{"serviceName":"agentpod-hub"}}}]}`)
			return
		}
		if r.URL.Path != "/select/jaeger/api/traces/"+id {
			t.Error(r.URL.Path)
			http.NotFound(w, r)
			return
		}
		lookups.Add(1)
		fmt.Fprint(w, `{"data":[{"traceID":"`+id+`","processes":{"hub":{"serviceName":"agentpod-hub"},"library":{"serviceName":"superlibrary"}},"spans":[
   {"traceID":"`+id+`","spanID":"6ada289a01d51af8","operationName":"library.read","startTime":2,"processID":"library","references":[{"refType":"CHILD_OF","traceID":"`+id+`","spanID":"1"}],"tags":[{"key":"tool.name","value":"library.read"}]},
   {"traceID":"`+id+`","spanID":"1","operationName":"heartbeat","startTime":1,"processID":"hub"},
   {"traceID":"`+id+`","spanID":"1","operationName":"heartbeat","startTime":1,"processID":"hub"}]}]}`)
	}))
	defer srv.Close()
	a := New(srv.URL, "vt-read", srv.Client())
	spans, st := a.ListSpans(context.Background(), runRef(t))
	if st != source.StatusOK || len(spans) != 2 {
		t.Fatalf("status=%s spans=%+v", st, spans)
	}
	if lookups.Load() != 1 {
		t.Fatalf("lookups=%d", lookups.Load())
	}
	child := spans[1]
	if spans[0].Name != "heartbeat" || child.SpanID != "6ada289a01d51af8" || child.Service != "superlibrary" || child.ParentSpanID != "0000000000000001" || child.Attributes["tool.name"] != "library.read" {
		t.Fatalf("joined spans=%+v", spans)
	}
	if _, exists := child.Attributes["run.id"]; exists {
		t.Error("child must prove join without run.id")
	}
}

func TestFullTraceFailureDoesNotReturnPartialSeed(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		code       int
		want       source.SourceStatus
	}{
		{"unauthorized", "", 401, source.StatusUnauthorized},
		{"unavailable", "", 500, source.StatusUnavailable},
		{"missing", "", 404, source.StatusUnavailable},
		{"empty", `{"data":[]}`, 200, source.StatusUnavailable},
		{"foreign", `{"data":[{"traceID":"2","spans":[{"traceID":"2"}]}]}`, 200, source.StatusUnavailable},
		{"foreign-span", `{"data":[{"traceID":"1","spans":[{"traceID":"2"}]}]}`, 200, source.StatusUnavailable},
		{"malformed", `{`, 200, source.StatusUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/select/jaeger/api/traces" {
					fmt.Fprint(w, `{"data":[{"traceID":"1","spans":[{"traceID":"1","spanID":"1"}]}]}`)
					return
				}
				w.WriteHeader(tc.code)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			a := New(srv.URL, "", srv.Client())
			spans, st := a.ListSpans(context.Background(), runRef(t))
			if st != tc.want || spans != nil {
				t.Fatalf("status=%s spans=%+v", st, spans)
			}
		})
	}
}

func TestInvalidDiscoveredTraceIDIsUnavailable(t *testing.T) {
	for _, id := range []string{"", "0", "../other", strings.Repeat("a", 33), "not-hex"} {
		t.Run(id, func(t *testing.T) {
			var full atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/select/jaeger/api/traces" {
					full.Add(1)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"traceID": id}}})
			}))
			defer srv.Close()
			a := New(srv.URL, "", srv.Client())
			spans, st := a.ListSpans(context.Background(), runRef(t))
			if st != source.StatusUnavailable || spans != nil || full.Load() != 0 {
				t.Fatalf("status=%s spans=%+v full=%d", st, spans, full.Load())
			}
		})
	}
}

func TestFullTraceLookupsBoundConcurrency(t *testing.T) {
	var active, maximum atomic.Int32
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/select/jaeger/api/traces" {
			fmt.Fprint(w, `{"data":[{"traceID":"1"},{"traceID":"2"},{"traceID":"3"},{"traceID":"4"},{"traceID":"5"}]}`)
			return
		}
		n := active.Add(1)
		for old := maximum.Load(); n > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, n) {
				break
			}
		}
		defer active.Add(-1)
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		id := strings.TrimPrefix(r.URL.Path, "/select/jaeger/api/traces/")
		fmt.Fprintf(w, `{"data":[{"traceID":%q,"spans":[{"traceID":%q,"spanID":"1"}]}]}`, id, id)
	}))
	defer srv.Close()
	a := New(srv.URL, "", srv.Client())
	done := make(chan source.SourceStatus, 1)
	go func() { _, st := a.ListSpans(context.Background(), runRef(t)); done <- st }()
	for range 4 {
		<-entered
	}
	close(release)
	if st := <-done; st != source.StatusOK || maximum.Load() != 4 {
		t.Fatalf("status=%s max=%d", st, maximum.Load())
	}
}

func TestFullTraceLookupUsesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/select/jaeger/api/traces" {
			fmt.Fprint(w, `{"data":[{"traceID":"1"}]}`)
			return
		}
		cancel()
		<-r.Context().Done()
	}))
	defer srv.Close()
	a := New(srv.URL, "", srv.Client())
	spans, st := a.ListSpans(ctx, runRef(t))
	if st != source.StatusUnavailable || spans != nil {
		t.Fatalf("status=%s spans=%+v", st, spans)
	}
}
