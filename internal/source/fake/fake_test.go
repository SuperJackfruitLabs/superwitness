package fake

import (
	"context"
	"errors"
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
	if len(spans) != 4 {
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
