package verdicts

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

// MigrateAttemptTimeout bounds one migration attempt, so a Postgres that accepts the TCP
// connection but never answers cannot stall the retry loop.
const MigrateAttemptTimeout = 30 * time.Second

// StorePinger is a Store that can report its own reachability for /health.
type StorePinger interface {
	Store
	source.Pinger
}

// Gated holds a Store back until its schema has been migrated (a verdict Postgres
// that is down must not take the read path with it). Until Open, every call fails with
// ErrUnavailable and Ping reports unavailable, so run documents still render with
// sources.verdicts "unavailable" and writes get 503 store_unavailable.
type Gated struct {
	Inner StorePinger
	ready atomic.Bool
}

func (g *Gated) Open()       { g.ready.Store(true) }
func (g *Gated) Ready() bool { return g.ready.Load() }

func (g *Gated) Insert(ctx context.Context, v Verdict) (Verdict, bool, error) {
	if !g.Ready() {
		return Verdict{}, false, ErrUnavailable
	}
	return g.Inner.Insert(ctx, v)
}

func (g *Gated) GetByKey(ctx context.Context, key string) (Verdict, error) {
	if !g.Ready() {
		return Verdict{}, ErrUnavailable
	}
	return g.Inner.GetByKey(ctx, key)
}

func (g *Gated) Get(ctx context.Context, id string) (Verdict, error) {
	if !g.Ready() {
		return Verdict{}, ErrUnavailable
	}
	return g.Inner.Get(ctx, id)
}

func (g *Gated) HasSuccessor(ctx context.Context, id string) (bool, error) {
	if !g.Ready() {
		return false, ErrUnavailable
	}
	return g.Inner.HasSuccessor(ctx, id)
}

func (g *Gated) ListCurrent(ctx context.Context, subjects []SubjectKey) ([]Verdict, error) {
	if !g.Ready() {
		return nil, ErrUnavailable
	}
	return g.Inner.ListCurrent(ctx, subjects)
}

func (g *Gated) RubricExists(ctx context.Context, id string, version int) (bool, error) {
	if !g.Ready() {
		return false, ErrUnavailable
	}
	return g.Inner.RubricExists(ctx, id, version)
}

func (g *Gated) InsertRubric(ctx context.Context, r Rubric) error {
	if !g.Ready() {
		return ErrUnavailable
	}
	return g.Inner.InsertRubric(ctx, r)
}

func (g *Gated) Ping(ctx context.Context) source.SourceStatus {
	if !g.Ready() {
		return source.StatusUnavailable
	}
	return g.Inner.Ping(ctx)
}

// OpenWhenMigrated calls migrate until it succeeds, then opens g. Between failed attempts it
// waits minWait, doubling up to maxWait, and logs each failure. It returns once g is open or
// ctx ends.
func (g *Gated) OpenWhenMigrated(ctx context.Context, migrate func(context.Context) error,
	minWait, maxWait time.Duration, logger *slog.Logger) {
	wait := minWait
	for attempt := 1; ; attempt++ {
		actx, cancel := context.WithTimeout(ctx, MigrateAttemptTimeout)
		err := migrate(actx)
		cancel()
		if err == nil {
			g.Open()
			logger.Info("verdict store migrated; accepting verdicts", "attempt", attempt)
			return
		}
		if ctx.Err() != nil {
			return
		}
		logger.Warn("verdict store migration failed; verdicts unavailable until it succeeds",
			"attempt", attempt, "retry_in", wait.String(), "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, maxWait)
	}
}
