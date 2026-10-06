package api_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/fake"
)

const txPath = "/v1/runs/superpipeline/brd_01/run_01/transcript"

// recordingTranscripts answers as inner does (the development data set) and records every query.
// status, when set, is the answer instead; body, when set, is the page answer instead.
type recordingTranscripts struct {
	inner  source.TranscriptReader
	status source.SourceStatus
	body   json.RawMessage

	mu    sync.Mutex
	pages []source.TranscriptQuery
	items []source.ItemQuery
}

func (r *recordingTranscripts) TranscriptPage(ctx context.Context, q source.TranscriptQuery) (json.RawMessage, source.SourceStatus) {
	r.mu.Lock()
	r.pages = append(r.pages, q)
	r.mu.Unlock()
	switch {
	case r.status != "":
		return nil, r.status
	case r.body != nil:
		return r.body, source.StatusOK
	}
	return r.inner.TranscriptPage(ctx, q)
}

func (r *recordingTranscripts) TranscriptItem(ctx context.Context, q source.ItemQuery) (json.RawMessage, source.SourceStatus) {
	r.mu.Lock()
	r.items = append(r.items, q)
	r.mu.Unlock()
	if r.status != "" {
		return nil, r.status
	}
	return r.inner.TranscriptItem(ctx, q)
}

func (r *recordingTranscripts) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.pages) + len(r.items)
}

func (r *recordingTranscripts) lastPage(t *testing.T) source.TranscriptQuery {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pages) == 0 {
		t.Fatal("the hub was never asked for a page")
	}
	return r.pages[len(r.pages)-1]
}

func (r *recordingTranscripts) lastItem(t *testing.T) source.ItemQuery {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.items) == 0 {
		t.Fatal("the hub was never asked for an item")
	}
	return r.items[len(r.items)-1]
}

// transcriptServer is the app server with a recording reader over the development transcript.
// mutate may change Ops before the first request.
func transcriptServer(t *testing.T, mutate func(*api.Ops)) (*appServer, *recordingTranscripts) {
	t.Helper()
	d, err := fake.NewDev()
	if err != nil {
		t.Fatal(err)
	}
	rec := &recordingTranscripts{inner: d}
	a := newAppServerWith(t, nil, func(s *api.Server) {
		s.Ops.Transcripts = rec
		if mutate != nil {
			mutate(s.Ops)
		}
	})
	return a, rec
}

func ptr(n int64) *int64 { return &n }

// ledgerOf is the hub ledger for brd_01/run_01 holding the given attempts.
func ledgerOf(attempts ...source.LedgerAttempt) *fake.Source {
	board := "brd_01"
	return &fake.Source{SourceName: source.AgentPod, Fragment: source.Fragment{Ledger: &source.LedgerFragment{
		ExternalSource: "superpipeline", ExternalRunID: "run_01", BoardID: &board, Attempts: attempts}}}
}

func seqs(q source.TranscriptQuery) string {
	f, to := "nil", "nil"
	if q.SeqFrom != nil {
		f = jsonNum(*q.SeqFrom)
	}
	if q.SeqTo != nil {
		to = jsonNum(*q.SeqTo)
	}
	return f + ".." + to
}

func jsonNum(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestTranscriptThroughASession(t *testing.T) {
	a, rec := transcriptServer(t, nil)
	resp, b := a.call(t, "GET", txPath, "", "cookie-human01", "", "")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var page struct {
		AttemptID  string            `json:"attempt_id"`
		SessionID  string            `json:"session_id"`
		Items      []json.RawMessage `json:"items"`
		Redactions int               `json:"redactions"`
	}
	if err := json.Unmarshal([]byte(b), &page); err != nil || page.AttemptID != "attempt_01" || page.SessionID != fake.DevSession ||
		len(page.Items) != 6 || page.Redactions != 2 {
		t.Errorf("page = %+v (%v)", page, err)
	}
	q := rec.lastPage(t)
	if q.SessionID != fake.DevSession || seqs(q) != "1..9" || q.Cursor != "" {
		t.Errorf("hub query = %+v %s; want the whole attempt, 1..9", q, seqs(q))
	}
	if q.OnBehalfOf != "prn_human01" {
		t.Errorf("OnBehalfOf = %q, want the session's principal", q.OnBehalfOf)
	}
}

func TestTranscriptAccess(t *testing.T) {
	a, rec := transcriptServer(t, nil)
	for _, c := range []struct {
		name, bearer, cookie string
		want                 int
	}{
		{"an evidence-only token", "dev:prn_grader01:agent:evidence:read", "", 403},
		{"a person's token without the scope", "dev:prn_human01:human", "", 403},
		{"a reporter's token", "dev:prn_reporter01:service:runs:write", "", 403},
	} {
		resp, b := a.call(t, "GET", txPath, c.bearer, c.cookie, "", "")
		if resp.StatusCode != c.want || errCode(t, []byte(b)) != "transcripts_forbidden" {
			t.Errorf("%s: %d %s", c.name, resp.StatusCode, b)
		}
		resp, b = a.call(t, "GET", txPath+"/items/3", c.bearer, c.cookie, "", "")
		if resp.StatusCode != c.want || errCode(t, []byte(b)) != "transcripts_forbidden" {
			t.Errorf("%s, item: %d %s", c.name, resp.StatusCode, b)
		}
	}
	if n := rec.calls(); n != 0 {
		t.Fatalf("a refused caller reached the hub %d times", n)
	}
	resp, b := a.call(t, "GET", txPath, "dev:prn_grader01:agent:evidence:read,transcripts:read", "", "", "")
	if resp.StatusCode != 200 || !strings.Contains(b, `"attempt_id":"attempt_01"`) {
		t.Errorf("a token granted transcripts:read: %d %s", resp.StatusCode, b)
	}
	if q := rec.lastPage(t); q.OnBehalfOf != "prn_grader01" {
		t.Errorf("OnBehalfOf = %q, want the token's principal", q.OnBehalfOf)
	}
	if resp, b := a.call(t, "GET", txPath, "", "cookie-human01", "", ""); resp.StatusCode != 200 {
		t.Errorf("a session: %d %s", resp.StatusCode, b)
	}
	if resp, _ := a.call(t, "GET", txPath, "", "", "", ""); resp.StatusCode != 401 {
		t.Errorf("no credential: %d", resp.StatusCode)
	}
}

func TestTranscriptAttempts(t *testing.T) {
	ended := source.LedgerAttempt{ID: "attempt_01", SessionID: "acps_01", StartSeq: 1, EndSeq: ptr(9)}
	live := source.LedgerAttempt{ID: "attempt_02", SessionID: "acps_02", StartSeq: 1}
	a, rec := transcriptServer(t, func(o *api.Ops) { o.Join.AgentPod = ledgerOf(ended, live) })
	rec.body = json.RawMessage(`{"session_id":"acps_02","seq_from":1,"seq_to":4,"items":[],"next_cursor":null,"redactions":0,"truncated_fields":0}`)
	for _, c := range []struct {
		query string
		want  int
		code  string
	}{
		{"", 400, "attempt_required"},
		{"?attempt=attempt_09", 404, "attempt_not_found"},
		{"?attempt=bogus", 400, "invalid_attempt_id"},
	} {
		resp, b := a.call(t, "GET", txPath+c.query, "", "cookie-human01", "", "")
		if resp.StatusCode != c.want || errCode(t, []byte(b)) != c.code {
			t.Errorf("%q: %d %s, want %d %s", c.query, resp.StatusCode, b, c.want, c.code)
		}
	}
	if n := rec.calls(); n != 0 {
		t.Fatalf("an unresolved attempt reached the hub %d times", n)
	}
	resp, b := a.call(t, "GET", txPath+"?attempt=attempt_02", "", "cookie-human01", "", "")
	if resp.StatusCode != 200 || !strings.HasPrefix(b, `{"attempt_id":"attempt_02",`) {
		t.Fatalf("live attempt: %d %s", resp.StatusCode, b)
	}
	if q := rec.lastPage(t); q.SessionID != "acps_02" || seqs(q) != "1..nil" {
		t.Errorf("live attempt asked the hub for %s %s; want 1..nil (the session's live end)", q.SessionID, seqs(q))
	}
	if resp, _ := a.call(t, "GET", txPath+"?attempt=attempt_02&seq_to=500", "", "cookie-human01", "", ""); resp.StatusCode != 200 ||
		seqs(rec.lastPage(t)) != "1..500" {
		t.Errorf("a live attempt's seq_to goes to the hub: %d %s", resp.StatusCode, seqs(rec.lastPage(t)))
	}
	rec.body, rec.status = nil, source.StatusBadRange
	if resp, b := a.call(t, "GET", txPath+"?attempt=attempt_02&seq_to=500", "", "cookie-human01", "", ""); resp.StatusCode != 400 ||
		errCode(t, []byte(b)) != "range_outside_attempt" {
		t.Errorf("past the live end: %d %s", resp.StatusCode, b)
	}
}

func TestTranscriptLedger(t *testing.T) {
	for _, c := range []struct {
		name   string
		ledger *fake.Source
		want   int
		code   string
	}{
		{"the hub ledger does not know the run", &fake.Source{SourceName: source.AgentPod, Status: source.StatusNotFound}, 404, "attempt_not_found"},
		{"the hub ledger is down", &fake.Source{SourceName: source.AgentPod, Status: source.StatusUnavailable}, 503, "source_unavailable"},
		{"a run with no attempts", ledgerOf(), 404, "attempt_not_found"},
		{"an attempt with no session", ledgerOf(source.LedgerAttempt{ID: "attempt_01", StartSeq: 1, EndSeq: ptr(9)}), 404, "session_not_found"},
	} {
		a, rec := transcriptServer(t, func(o *api.Ops) { o.Join.AgentPod = c.ledger })
		resp, b := a.call(t, "GET", txPath, "", "cookie-human01", "", "")
		if resp.StatusCode != c.want || errCode(t, []byte(b)) != c.code {
			t.Errorf("%s: %d %s", c.name, resp.StatusCode, b)
		}
		if rec.calls() != 0 {
			t.Errorf("%s: the hub was asked for content", c.name)
		}
	}
}

func TestTranscriptRange(t *testing.T) {
	a, rec := transcriptServer(t, nil) // attempt_01 is 1..9
	for _, c := range []struct {
		path string
		code string
	}{
		{txPath + "?seq_from=0", "range_outside_attempt"},
		{txPath + "?seq_to=10", "range_outside_attempt"},
		{txPath + "?seq_from=10", "range_outside_attempt"},
		{txPath + "?seq_from=5&seq_to=3", "range_outside_attempt"},
		{txPath + "?seq_from=x", "invalid_seq"},
		{txPath + "?seq_to=-1", "invalid_seq"},
		{txPath + "/items/12", "range_outside_attempt"},
		{txPath + "/items/0", "range_outside_attempt"},
		{txPath + "/items/three", "invalid_seq"},
	} {
		resp, b := a.call(t, "GET", c.path, "", "cookie-human01", "", "")
		if resp.StatusCode != 400 || errCode(t, []byte(b)) != c.code {
			t.Errorf("%s: %d %s, want 400 %s", c.path, resp.StatusCode, b, c.code)
		}
	}
	if n := rec.calls(); n != 0 {
		t.Fatalf("a refused range reached the hub %d times", n)
	}
	for query, want := range map[string]string{"?seq_from=3": "3..9", "?seq_to=4": "1..4", "?seq_from=3&seq_to=4": "3..4", "?seq_from=9&seq_to=9": "9..9"} {
		if resp, b := a.call(t, "GET", txPath+query, "", "cookie-human01", "", ""); resp.StatusCode != 200 {
			t.Errorf("%s: %d %s", query, resp.StatusCode, b)
		}
		if got := seqs(rec.lastPage(t)); got != want {
			t.Errorf("%s asked the hub for %s, want %s", query, got, want)
		}
	}
	if resp, _ := a.call(t, "GET", txPath+"?cursor=opaque-1", "", "cookie-human01", "", ""); resp.StatusCode != 200 || rec.lastPage(t).Cursor != "opaque-1" {
		t.Errorf("the cursor did not reach the hub: %+v", rec.lastPage(t))
	}
}

func TestTranscriptHubErrors(t *testing.T) {
	for st, want := range map[source.SourceStatus]struct {
		status int
		code   string
	}{
		source.StatusUnavailable:  {503, "source_unavailable"},
		source.StatusTimeout:      {503, "source_unavailable"},
		source.StatusNotFound:     {404, "session_not_found"},
		source.StatusTooLarge:     {413, "item_too_large"},
		source.StatusUnauthorized: {502, "hub_refused"},
		source.StatusBadRange:     {400, "range_outside_attempt"},
	} {
		a, rec := transcriptServer(t, nil)
		rec.status = st
		for _, p := range []string{txPath, txPath + "/items/3?full=1"} {
			resp, b := a.call(t, "GET", p, "", "cookie-human01", "", "")
			if resp.StatusCode != want.status || errCode(t, []byte(b)) != want.code {
				t.Errorf("hub %s on %s: %d %s, want %d %s", st, p, resp.StatusCode, b, want.status, want.code)
			}
			if resp.Header.Get("Cache-Control") != "no-store" {
				t.Errorf("hub %s on %s: an error was cacheable", st, p)
			}
		}
	}
	a, rec := transcriptServer(t, nil)
	rec.body = json.RawMessage(`["not","an","object"]`)
	if resp, b := a.call(t, "GET", txPath, "", "cookie-human01", "", ""); resp.StatusCode != 503 || errCode(t, []byte(b)) != "source_unavailable" {
		t.Errorf("a hub body that is not an object: %d %s", resp.StatusCode, b)
	}
	a, _ = transcriptServer(t, func(o *api.Ops) { o.Transcripts = nil })
	if resp, b := a.call(t, "GET", txPath, "", "cookie-human01", "", ""); resp.StatusCode != 503 || errCode(t, []byte(b)) != "source_unavailable" {
		t.Errorf("no reader wired: %d %s", resp.StatusCode, b)
	}
}

func TestTranscriptItem(t *testing.T) {
	a, rec := transcriptServer(t, nil)
	resp, b := a.call(t, "GET", txPath+"/items/3?full=1", "", "cookie-human01", "", "")
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v %s", resp.StatusCode, resp.Header, b)
	}
	var item struct {
		AttemptID string         `json:"attempt_id"`
		Item      map[string]any `json:"item"`
	}
	if err := json.Unmarshal([]byte(b), &item); err != nil || item.AttemptID != "attempt_01" || item.Item["id"] != "tc_01" {
		t.Fatalf("item = %+v (%v)", item, err)
	}
	if !strings.Contains(b, "- change 1199: a line of the development changelog") {
		t.Error("the item is not whole")
	}
	if q := rec.lastItem(t); q.SessionID != fake.DevSession || q.SeqFrom != 3 || !q.Full || q.OnBehalfOf != "prn_human01" {
		t.Errorf("hub query = %+v", q)
	}
	if _, _ = a.call(t, "GET", txPath+"/items/3", "", "cookie-human01", "", ""); rec.lastItem(t).Full {
		t.Error("full sent to the hub when the caller did not ask")
	}
}

func TestTranscriptPassesTheHubBodyThrough(t *testing.T) {
	a, rec := transcriptServer(t, nil)
	body := `{"session_id":"acps_01", "items":[{"kind":"message","seq_from":1,"seq_to":1,"text":"<b>é & \"q\"</b>","redactions":0}],` +
		`"next_cursor":null,"redactions":0,"truncated_fields":0,"extra":{"k":[1,2.50]}}`
	rec.body = json.RawMessage(body)
	resp, b := a.call(t, "GET", txPath, "", "cookie-human01", "", "")
	if want := `{"attempt_id":"attempt_01",` + body[1:]; resp.StatusCode != 200 || b != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
	rec.body = json.RawMessage(" {}\n")
	if _, b := a.call(t, "GET", txPath, "", "cookie-human01", "", ""); b != `{"attempt_id":"attempt_01"}` {
		t.Errorf("an empty object: %s", b)
	}
}

func TestTranscriptItemRange(t *testing.T) {
	a, rec := transcriptServer(t, nil) // attempt_01 is 1..9
	if resp, b := a.call(t, "GET", txPath+"/items/3?seq_from=3&seq_to=4&full=1", "", "cookie-human01", "", ""); resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if q := rec.lastItem(t); q.RangeFrom == nil || q.RangeTo == nil || *q.RangeFrom != 3 || *q.RangeTo != 4 {
		t.Errorf("the range the item was shown in did not reach the hub: %+v", q)
	}
	if resp, b := a.call(t, "GET", txPath+"/items/3?seq_from=2", "", "cookie-human01", "", ""); resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if q := rec.lastItem(t); q.RangeFrom == nil || *q.RangeFrom != 2 || q.RangeTo == nil || *q.RangeTo != 9 {
		t.Errorf("an open end is the attempt's: %+v", q)
	}
	if resp, _ := a.call(t, "GET", txPath+"/items/3", "", "cookie-human01", "", ""); resp.StatusCode != 200 {
		t.Fatal("item without a range")
	}
	if q := rec.lastItem(t); q.RangeFrom != nil || q.RangeTo != nil {
		t.Errorf("no range given, yet one was sent: %+v", q)
	}
	before := rec.calls()
	for _, c := range []struct{ query, code string }{
		{"?seq_from=0", "range_outside_attempt"},
		{"?seq_to=10", "range_outside_attempt"},
		{"?seq_to=x", "invalid_seq"},
	} {
		resp, b := a.call(t, "GET", txPath+"/items/3"+c.query, "", "cookie-human01", "", "")
		if resp.StatusCode != 400 || errCode(t, []byte(b)) != c.code {
			t.Errorf("%s: %d %s, want 400 %s", c.query, resp.StatusCode, b, c.code)
		}
	}
	if rec.calls() != before {
		t.Error("a refused range reached the hub")
	}
}
