package verdicts

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

type pingMem struct{ *MemStore }

func (pingMem) Ping(context.Context) source.SourceStatus { return source.StatusOK }

func TestGatedIsUnavailableUntilOpen(t *testing.T) {
	ctx := context.Background()
	g := &Gated{Inner: pingMem{NewMemStore()}}
	if _, _, err := g.Insert(ctx, verdict("vrd_a", "k", nil, time.Now())); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Insert before open: %v", err)
	}
	if _, err := g.ListCurrent(ctx, nil); !errors.Is(err, ErrUnavailable) {
		t.Errorf("ListCurrent before open: %v", err)
	}
	if _, err := g.GetByKey(ctx, "k"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("GetByKey before open: %v", err)
	}
	if st := g.Ping(ctx); st != source.StatusUnavailable {
		t.Errorf("Ping before open: %s", st)
	}
	svc := &Service{Store: g, Subjects: &fakeSubjects{run: Subject{Found: true, Complete: true}}}
	_, _, err := svc.Record(ctx, human, req("k1"))
	wantCode(t, err, 503, "store_unavailable")

	g.Open()
	if _, created, err := g.Insert(ctx, verdict("vrd_a", "k", nil, time.Now())); err != nil || !created {
		t.Errorf("Insert after open: %v %v", created, err)
	}
	if st := g.Ping(ctx); st != source.StatusOK {
		t.Errorf("Ping after open: %s", st)
	}
}

func TestOpenWhenMigratedRetriesWithBackoff(t *testing.T) {
	g := &Gated{Inner: pingMem{NewMemStore()}}
	var logs bytes.Buffer
	calls := 0
	migrate := func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("connection refused")
		}
		return nil
	}
	start := time.Now()
	g.OpenWhenMigrated(context.Background(), migrate, 10*time.Millisecond, 15*time.Millisecond,
		slog.New(slog.NewTextHandler(&logs, nil)))
	if !g.Ready() || calls != 3 {
		t.Fatalf("ready=%v calls=%d", g.Ready(), calls)
	}
	if d := time.Since(start); d < 25*time.Millisecond { // 10ms, then min(20ms, 15ms)
		t.Errorf("returned after %s; the backoff was not applied", d)
	}
	if n := strings.Count(logs.String(), "migration failed"); n != 2 {
		t.Errorf("logged %d failures, want 2:\n%s", n, logs.String())
	}
}

func TestOpenWhenMigratedStopsWithContext(t *testing.T) {
	g := &Gated{Inner: pingMem{NewMemStore()}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		g.OpenWhenMigrated(ctx, func(context.Context) error { return errors.New("down") },
			time.Hour, time.Hour, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the retry loop ignored cancellation")
	}
	if g.Ready() {
		t.Error("opened without a successful migration")
	}
}
