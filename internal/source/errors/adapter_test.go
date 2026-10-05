package errors

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/logs"
)

const hubTrace = "4bf92f3577b34da6a3ce929d0e0e4736"

func TestQuery(t *testing.T) {
	r, _ := source.NewSuperpipelineRef("brd_01", "run_01")
	want := `_time:7d ("run.id":="run_01" or trace_id:in("` + hubTrace + `")) ` +
		`(severity_number:range[17, 24] or severity_text:i("error")) | sort by (_time) | limit 50`
	if got := Query(r.WithTraceIDs([]string{hubTrace})); got != want {
		t.Errorf("query =\n %s\nwant\n %s", got, want)
	}
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"_time":"2026-10-04T10:02:00Z","_msg":"bridge heartbeat failed","service.name":"agentpod-hub","severity_text":"ERROR","trace_id":"`+hubTrace+`"}`+"\n")
	}))
	defer srv.Close()
	a := New(logs.NewClient(srv.URL, "", srv.Client()))
	r, _ := source.NewSuperpipelineRef("brd_01", "run_01")
	f, st := a.Fetch(context.Background(), r)
	if st != source.StatusOK || len(f.Errors.Errors) != 1 {
		t.Fatalf("status = %s, fragment = %+v", st, f.Errors)
	}
	e := f.Errors.Errors[0]
	if e.Service != "agentpod-hub" || e.Message != "bridge heartbeat failed" || e.TraceID != hubTrace || e.At.IsZero() {
		t.Errorf("error = %+v", e)
	}
}

func TestFetchUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer srv.Close()
	r, _ := source.NewSuperpipelineRef("brd_01", "run_01")
	if _, st := New(logs.NewClient(srv.URL, "", srv.Client())).Fetch(context.Background(), r); st != source.StatusUnavailable {
		t.Errorf("status = %s", st)
	}
}
