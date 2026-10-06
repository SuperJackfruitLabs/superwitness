package agentpod

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

const pageBody = `{"session_id":"acps_01","seq_from":3,"seq_to":9,"items":[{"kind":"message","seq_from":3,"seq_to":3,"text":"hi","redactions":0}],"next_cursor":null,"redactions":0,"truncated_fields":0}`

// recordingHub answers every request with status and body, and hands each request to seen.
func recordingHub(t *testing.T, status int, body string) (*httptest.Server, chan *http.Request) {
	t.Helper()
	seen := make(chan *http.Request, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Clone(context.Background())
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func i64(n int64) *int64 { return &n }

func TestTranscriptPageRequest(t *testing.T) {
	srv, seen := recordingHub(t, 200, pageBody)
	a := New(srv.URL, &auth.StaticTokenSource{Value: "tok-hub"}, nil)
	body, st := a.TranscriptPage(context.Background(), source.TranscriptQuery{
		SessionID: "acps_01", SeqFrom: i64(3), SeqTo: i64(9), Cursor: "c2", OnBehalfOf: "prn_human01"})
	if st != source.StatusOK || string(body) != pageBody {
		t.Fatalf("status %s, body %s: want the hub's body untouched", st, body)
	}
	r := <-seen
	if r.URL.Path != "/api/evidence/sessions/acps_01/transcript" {
		t.Errorf("path = %s", r.URL.Path)
	}
	if q := r.URL.Query(); q.Get("seq_from") != "3" || q.Get("seq_to") != "9" || q.Get("cursor") != "c2" || q.Has("limit") {
		t.Errorf("query = %s", r.URL.RawQuery)
	}
	if r.Header.Get("Authorization") != "Bearer tok-hub" {
		t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
	}
	if r.Header.Get("X-On-Behalf-Of") != "prn_human01" {
		t.Errorf("X-On-Behalf-Of = %q, want the caller's prn_ id", r.Header.Get("X-On-Behalf-Of"))
	}
}

func TestTranscriptPageLeavesOutWhatIsUnset(t *testing.T) {
	srv, seen := recordingHub(t, 200, pageBody)
	a := New(srv.URL, &auth.StaticTokenSource{Value: "tok-hub"}, nil)
	if _, st := a.TranscriptPage(context.Background(), source.TranscriptQuery{SessionID: "acps_01", OnBehalfOf: "hubuser_01"}); st != source.StatusOK {
		t.Fatalf("status %s", st)
	}
	r := <-seen
	if r.URL.RawQuery != "" {
		t.Errorf("query = %q, want none", r.URL.RawQuery)
	}
	if _, set := r.Header["X-On-Behalf-Of"]; set {
		t.Errorf("X-On-Behalf-Of sent for a caller that is not a prn_ id: %q", r.Header.Get("X-On-Behalf-Of"))
	}
}

func TestTranscriptItemRequest(t *testing.T) {
	srv, seen := recordingHub(t, 200, `{"session_id":"acps_01","item":{"kind":"tool_call","seq_from":7,"seq_to":8,"redactions":0}}`)
	a := New(srv.URL, &auth.StaticTokenSource{Value: "tok-hub"}, nil)
	if _, st := a.TranscriptItem(context.Background(), source.ItemQuery{SessionID: "acps_01", SeqFrom: 7, Full: true, OnBehalfOf: "prn_human01"}); st != source.StatusOK {
		t.Fatalf("status %s", st)
	}
	r := <-seen
	if r.URL.Path != "/api/evidence/sessions/acps_01/transcript/items/7" || r.URL.RawQuery != "full=1" {
		t.Errorf("url = %s", r.URL)
	}
	if r.Header.Get("X-On-Behalf-Of") != "prn_human01" {
		t.Errorf("X-On-Behalf-Of = %q", r.Header.Get("X-On-Behalf-Of"))
	}
	if _, st := a.TranscriptItem(context.Background(), source.ItemQuery{SessionID: "acps_01", SeqFrom: 7}); st != source.StatusOK {
		t.Fatalf("status %s", st)
	}
	if r := <-seen; r.URL.RawQuery != "" {
		t.Errorf("full sent when not asked: %q", r.URL.RawQuery)
	}
}

func TestTranscriptItemForwardsTheRangeItWasShownIn(t *testing.T) {
	srv, seen := recordingHub(t, 200, `{"session_id":"acps_01","item":{"kind":"tool_call","seq_from":3,"seq_to":4,"redactions":0}}`)
	a := New(srv.URL, &auth.StaticTokenSource{Value: "tok-hub"}, nil)
	if _, st := a.TranscriptItem(context.Background(), source.ItemQuery{SessionID: "acps_01", SeqFrom: 3, Full: true, RangeFrom: i64(3), RangeTo: i64(4)}); st != source.StatusOK {
		t.Fatalf("status %s", st)
	}
	r := <-seen
	if r.URL.Path != "/api/evidence/sessions/acps_01/transcript/items/3" || r.URL.RawQuery != "full=1&seq_from=3&seq_to=4" {
		t.Errorf("url = %s", r.URL)
	}
}

func TestTranscriptStatuses(t *testing.T) {
	for _, c := range []struct {
		name       string
		status     int
		body       string
		want       source.SourceStatus
		invalidate bool
	}{
		{"ok", 200, pageBody, source.StatusOK, false},
		{"200 that is not JSON", 200, "<html>", source.StatusUnavailable, false},
		{"the hub's not found", 404, `{"error":"not_found"}`, source.StatusNotFound, false},
		{"bare 404: the route is not deployed", 404, "404 page not found", source.StatusUnavailable, false},
		{"too large", 413, `{"error":"item_too_large"}`, source.StatusTooLarge, false},
		{"bad range", 400, `{"error":"bad_range"}`, source.StatusBadRange, false},
		{"another 400", 400, `{"error":"bad_cursor"}`, source.StatusUnavailable, false},
		{"forbidden: no transcripts:read in the grant", 403, `{"error":"forbidden"}`, source.StatusUnauthorized, true},
		{"unauthorized", 401, `{"error":"unauthorized"}`, source.StatusUnauthorized, true},
		{"server error", 500, `{"error":"internal"}`, source.StatusUnavailable, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv, _ := recordingHub(t, c.status, c.body)
			tokens := &auth.StaticTokenSource{Value: "tok-hub"}
			a := New(srv.URL, tokens, nil)
			body, st := a.TranscriptPage(context.Background(), source.TranscriptQuery{SessionID: "acps_01"})
			if st != c.want {
				t.Errorf("status = %s, want %s", st, c.want)
			}
			if st != source.StatusOK && body != nil {
				t.Errorf("a refused read returned a body: %s", body)
			}
			if got := tokens.Invalidations() == 1; got != c.invalidate {
				t.Errorf("token invalidated = %v, want %v", got, c.invalidate)
			}
		})
	}
}

func TestTranscriptItemOverTheCapIsUnavailable(t *testing.T) {
	big := `"` + strings.Repeat("a", maxTranscriptItem) + `"`
	srv, _ := recordingHub(t, 200, `{"session_id":"acps_01","item":{"text":`+big+`}}`)
	a := New(srv.URL, &auth.StaticTokenSource{Value: "tok-hub"}, nil)
	if body, st := a.TranscriptItem(context.Background(), source.ItemQuery{SessionID: "acps_01", SeqFrom: 1}); st != source.StatusUnavailable || body != nil {
		t.Errorf("status %s, %d bytes: want unavailable and no body", st, len(body))
	}
}

func TestTranscriptTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()
	a := New(srv.URL, &auth.StaticTokenSource{Value: "tok-hub"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, st := a.TranscriptPage(ctx, source.TranscriptQuery{SessionID: "acps_01"}); st != source.StatusTimeout {
		t.Errorf("status = %s, want timeout", st)
	}
}
