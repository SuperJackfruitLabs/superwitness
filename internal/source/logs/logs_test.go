package logs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

const hubTrace = "4bf92f3577b34da6a3ce929d0e0e4736"

func runRef(t *testing.T, traceIDs ...string) source.RunRef {
	t.Helper()
	r, err := source.NewSuperpipelineRef("brd_01", "run_01")
	if err != nil {
		t.Fatal(err)
	}
	return r.WithTraceIDs(traceIDs)
}

func TestRunFilter(t *testing.T) {
	if got := RunFilter(runRef(t)); got != `("run.id":="run_01")` {
		t.Errorf("no traces: %s", got)
	}
	got := RunFilter(runRef(t, hubTrace, `") or *`))
	if got != `("run.id":="run_01" or trace_id:in("4bf92f3577b34da6a3ce929d0e0e4736"))` {
		t.Errorf("with traces: %s", got)
	}
}

func TestBuildListQuery(t *testing.T) {
	q, err := BuildListQuery(runRef(t, hubTrace), source.LogQuery{Level: "error", Offset: 10, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	want := `_time:7d ("run.id":="run_01" or trace_id:in("4bf92f3577b34da6a3ce929d0e0e4736")) ` +
		`(severity_number:range[17, 24] or severity_text:i("error")) | sort by (_time) | offset 10 | limit 5`
	if q != want {
		t.Errorf("query =\n %s\nwant\n %s", q, want)
	}
	if _, err := BuildListQuery(runRef(t), source.LogQuery{Level: "verbose", Limit: 5}); !errors.Is(err, ErrInvalidLevel) {
		t.Errorf("err = %v", err)
	}
}

func vlServer(t *testing.T, wantQuery string, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/select/logsql/query" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = r.ParseForm()
		if wantQuery != "" && r.Form.Get("query") != wantQuery {
			t.Errorf("query =\n %s\nwant\n %s", r.Form.Get("query"), wantQuery)
		}
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestListLogsParsesLines(t *testing.T) {
	body := `{"_time":"2026-10-04T10:00:03Z","_msg":"attempt opened","service.name":"agentpod-hub","severity_text":"INFO","severity_number":"9","trace_id":"` + hubTrace + `","span_id":"00f067aa0ba902b8","run.id":"run_01"}
{"_time":"2026-10-04T10:02:00.5Z","_msg":"bridge heartbeat failed","service.name":"agentpod-hub","severity_number":"17","trace_id":"` + hubTrace + `"}
`
	want := `_time:7d ("run.id":="run_01" or trace_id:in("` + hubTrace + `")) | sort by (_time) | offset 0 | limit 100`
	srv := vlServer(t, want, body, 0)
	a := New(NewClient(srv.URL, "", srv.Client()))
	lines, st := a.ListLogs(context.Background(), runRef(t, hubTrace), source.LogQuery{Limit: 100})
	if st != source.StatusOK || len(lines) != 2 {
		t.Fatalf("status = %s, lines = %d", st, len(lines))
	}
	if lines[0].Level != "info" || lines[0].Service != "agentpod-hub" || lines[0].TraceID != hubTrace || lines[0].Message != "attempt opened" || lines[0].RunID != "run_01" {
		t.Errorf("line 0 = %+v", lines[0])
	}
	if lines[1].Level != "error" || lines[1].At.Nanosecond() != 500_000_000 {
		t.Errorf("line 1 = %+v", lines[1])
	}
}

func TestFetchCounts(t *testing.T) {
	srv := vlServer(t, `_time:7d ("run.id":="run_01") | stats count() as n`, `{"n":"2"}`+"\n", 0)
	a := New(NewClient(srv.URL, "", srv.Client()))
	f, st := a.Fetch(context.Background(), runRef(t))
	if st != source.StatusOK || f.Logs.Count != 2 {
		t.Errorf("status = %s, fragment = %+v", st, f.Logs)
	}
}

func TestQueryFailures(t *testing.T) {
	srv := vlServer(t, "", "", 401)
	if _, st := NewClient(srv.URL, "", srv.Client()).Query(context.Background(), "*"); st != source.StatusUnauthorized {
		t.Errorf("401: %s", st)
	}
	srv = vlServer(t, "", "{not json\n", 0)
	if _, st := NewClient(srv.URL, "", srv.Client()).Query(context.Background(), "*"); st != source.StatusUnavailable {
		t.Errorf("malformed: %s", st)
	}
}
