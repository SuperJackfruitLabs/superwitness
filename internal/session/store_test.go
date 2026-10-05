package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
)

var (
	t0    = time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	human = auth.Principal{ID: "prn_human01", Kind: auth.KindHuman, Tenant: "tenant_01", Email: "human01@example.com"}
)

// storeContract holds every Store to the same behaviour; the sweep included.
func storeContract(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	mk := func(b byte, created, seen time.Time) Session {
		id := make([]byte, 32)
		id[0] = b
		return Session{IDHash: id, Principal: "prn_human01", PrincipalKind: auth.KindHuman, Tenant: "tenant_01",
			CreatedAt: created, LastSeenAt: seen, ExpiresAt: created.Add(AbsoluteTTL)}
	}
	now := t0.Add(13 * time.Hour)
	live := mk(1, now.Add(-time.Hour), now.Add(-time.Minute))
	old := mk(2, t0, now.Add(-time.Minute))                     // past 12h
	idle := mk(3, now.Add(-3*time.Hour), now.Add(-2*time.Hour)) // exactly 2h idle
	for _, x := range []Session{live, old, idle} {
		if err := s.Create(ctx, x); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Get(ctx, live.IDHash)
	if err != nil || got.Principal != "prn_human01" || got.Email != "" || !got.ExpiresAt.Equal(live.ExpiresAt) {
		t.Errorf("get = %+v %v", got, err)
	}
	if err := s.Touch(ctx, live.IDHash, now); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(ctx, live.IDHash); !got.LastSeenAt.Equal(now) {
		t.Errorf("touch: last_seen_at = %v", got.LastSeenAt)
	}
	if n, err := s.Sweep(ctx, now); err != nil || n != 2 {
		t.Errorf("sweep deleted %d %v; want 2 (the 12h-old and the 2h-idle)", n, err)
	}
	if _, err := s.Get(ctx, live.IDHash); err != nil {
		t.Errorf("the sweep took a live session: %v", err)
	}
	if err := s.Delete(ctx, live.IDHash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, live.IDHash); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
}

func TestMemStoreContract(t *testing.T) { storeContract(t, NewMemStore()) }
