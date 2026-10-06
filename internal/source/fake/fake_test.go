package fake

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

func TestSourceHonoursDeadline(t *testing.T) {
	s := &Source{SourceName: source.Traces, Delay: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, st := s.Fetch(ctx, source.RunRef{}); st != source.StatusTimeout {
		t.Errorf("status = %s", st)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Error("Fetch ignored the context")
	}
}

func TestSourceStatusAndMatch(t *testing.T) {
	s := &Source{SourceName: source.AgentPod, Status: source.StatusUnauthorized}
	if _, st := s.Fetch(context.Background(), source.RunRef{}); st != source.StatusUnauthorized {
		t.Errorf("status = %s", st)
	}
	m := &Source{SourceName: source.AgentPod, Match: func(r source.RunRef) bool { return r.RunID == "run_01" }}
	if _, st := m.Fetch(context.Background(), source.RunRef{RunID: "run_02"}); st != source.StatusNotFound {
		t.Errorf("miss = %s", st)
	}
	if f, st := m.Fetch(context.Background(), source.RunRef{RunID: "run_01"}); st != source.StatusOK || f.Source != source.AgentPod {
		t.Errorf("hit = %s %+v", st, f)
	}
	if len(m.Seen()) != 2 {
		t.Errorf("seen = %d", len(m.Seen()))
	}
}

func TestDevDataSet(t *testing.T) {
	d, err := NewDev()
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := source.NewSuperpipelineRef(DevBoard, DevRun)
	ctx := context.Background()

	f, st := d.SP.Fetch(ctx, ref)
	if st != source.StatusOK || f.Run.Run.AgentPrincipalID != DevExecutor {
		t.Errorf("superpipeline: %s %+v", st, f.Run)
	}
	other, _ := source.NewSuperpipelineRef(DevBoard, "run_99")
	if _, st := d.AP.Fetch(ctx, other); st != source.StatusNotFound {
		t.Errorf("unknown run on hub: %s", st)
	}
	if f, st := d.Traces.Fetch(ctx, other); st != source.StatusOK || len(f.Traces.Spans) != 0 {
		t.Errorf("unknown run on traces should be ok and empty: %s", st)
	}
	spans, _ := d.ListSpans(ctx, ref)
	if len(spans) != 7 {
		t.Errorf("spans = %d", len(spans))
	}
	if ids := source.TraceIDsOf(spans); len(ids) != 2 || ids[0] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace ids = %v", ids)
	}
	errs, _ := d.ListLogs(ctx, ref, source.LogQuery{Level: "error", Limit: 10})
	if len(errs) != 1 {
		t.Errorf("error lines = %d", len(errs))
	}
	page, _ := d.ListLogs(ctx, ref, source.LogQuery{Offset: 1, Limit: 10})
	if len(page) != 1 {
		t.Errorf("offset page = %d", len(page))
	}
	link, st := d.ResolveAttempt(ctx, DevAttempt)
	if r, ok := link.Ref(); st != source.StatusOK || !ok || r.RunID != DevRun {
		t.Errorf("attempt link: %s %+v", st, link)
	}
	if p, err := d.Lookup(ctx, DevGrader); err != nil || p.Kind != auth.KindAgent {
		t.Errorf("grader = %+v %v", p, err)
	}
	if p, err := d.Lookup(ctx, "hubuser_7f3a"); err != nil || p.ID != DevHuman2 {
		t.Errorf("hub sub = %+v %v", p, err)
	}
	if _, err := d.Lookup(ctx, "prn_nobody"); !errors.Is(err, auth.ErrPrincipalNotFound) {
		t.Errorf("unknown principal err = %v", err)
	}
}

func TestDevTranscript(t *testing.T) {
	d, err := NewDev()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	type page struct {
		SessionID       string           `json:"session_id"`
		SeqFrom         int64            `json:"seq_from"`
		SeqTo           int64            `json:"seq_to"`
		Items           []map[string]any `json:"items"`
		NextCursor      *string          `json:"next_cursor"`
		Redactions      int              `json:"redactions"`
		TruncatedFields int              `json:"truncated_fields"`
	}
	read := func(q source.TranscriptQuery) (page, source.SourceStatus) {
		t.Helper()
		b, st := d.TranscriptPage(ctx, q)
		var p page
		if st == source.StatusOK {
			if err := json.Unmarshal(b, &p); err != nil {
				t.Fatalf("page is not JSON: %v", err)
			}
		}
		return p, st
	}
	n := func(v int64) *int64 { return &v }

	whole, st := read(source.TranscriptQuery{SessionID: DevSession})
	if st != source.StatusOK || whole.SessionID != DevSession || whole.SeqFrom != 1 || whole.SeqTo != 9 || len(whole.Items) != 6 ||
		whole.NextCursor != nil || whole.Redactions != 2 || whole.TruncatedFields != 1 {
		t.Fatalf("whole session: %s %+v", st, whole)
	}
	kinds := []string{}
	for _, it := range whole.Items {
		kinds = append(kinds, it["kind"].(string))
	}
	if strings.Join(kinds, ",") != "prompt,reasoning,tool_call,permission,tool_call,message" {
		t.Errorf("kinds = %v", kinds)
	}
	if text := whole.Items[0]["text"].(string); !strings.Contains(text, "[redacted:anthropic-key]") || !strings.Contains(text, "[redacted:authorization]") {
		t.Errorf("prompt = %q, want both redaction markers", text)
	}
	cut := whole.Items[2]["output"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(map[string]any)
	if cut["truncated"] != true || len(cut["head"].(string)) != 16<<10 || cut["bytes"].(float64) <= 16<<10 {
		t.Errorf("the cut field = truncated %v, head %d bytes, bytes %v", cut["truncated"], len(cut["head"].(string)), cut["bytes"])
	}
	if whole.Items[4]["status"] != "failed" || whole.Items[4]["id"] != "tc_02" {
		t.Errorf("item 5 = %+v", whole.Items[4])
	}

	one, _ := read(source.TranscriptQuery{SessionID: DevSession, SeqFrom: n(3), SeqTo: n(4)})
	if len(one.Items) != 1 || one.Items[0]["id"] != "tc_01" || one.Items[0]["partial"] != nil {
		t.Errorf("3..4 = %+v", one.Items)
	}
	tail, _ := read(source.TranscriptQuery{SessionID: DevSession, SeqFrom: n(4)})
	if len(tail.Items) != 4 || tail.Items[0]["id"] != "tc_01" || tail.Items[0]["partial"] != true {
		t.Errorf("4..9 = %+v, want tc_01 first and partial", tail.Items)
	}
	if _, st := read(source.TranscriptQuery{SessionID: "acps_99"}); st != source.StatusNotFound {
		t.Errorf("another session: %s", st)
	}
	if _, st := read(source.TranscriptQuery{SessionID: DevSession, SeqTo: n(10)}); st != source.StatusBadRange {
		t.Errorf("past the session's end: %s", st)
	}

	b, st := d.TranscriptItem(ctx, source.ItemQuery{SessionID: DevSession, SeqFrom: 3, Full: true})
	var item struct {
		SessionID string         `json:"session_id"`
		Item      map[string]any `json:"item"`
	}
	if st != source.StatusOK || json.Unmarshal(b, &item) != nil || item.SessionID != DevSession {
		t.Fatalf("item 3: %s %s", st, b)
	}
	full := item.Item["output"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if len(full) <= 16<<10 || !strings.HasSuffix(full, "- change 1199: a line of the development changelog\n") {
		t.Errorf("item 3's output: %d bytes, ends %q", len(full), full[max(0, len(full)-60):])
	}
	if _, st := d.TranscriptItem(ctx, source.ItemQuery{SessionID: DevSession, SeqFrom: 4}); st != source.StatusNotFound {
		t.Errorf("seq 4 starts no item: %s", st)
	}
}
