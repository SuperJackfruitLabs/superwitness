package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newManager() (*Manager, *MemStore, *clock) {
	st, c := NewMemStore(), &clock{t0}
	return &Manager{Store: st, Allowed: map[string]bool{"prn_human01": true}, Now: c.now}, st, c
}

func TestIssueAndResolve(t *testing.T) {
	m, st, _ := newManager()
	tok, err := m.Issue(context.Background(), human)
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 43 || st.Len() != 1 {
		t.Fatalf("token %q (%d chars), %d rows", tok, len(tok), st.Len())
	}
	id, _ := hashToken(tok)
	if s, _ := st.Get(context.Background(), id); string(s.IDHash) == tok || s.Email != "human01@example.com" || !s.ExpiresAt.Equal(t0.Add(12*time.Hour)) {
		t.Errorf("stored session = %+v", s)
	}
	p, err := m.Resolve(context.Background(), tok)
	if err != nil || p.ID != human.ID || p.Kind != human.Kind || p.Tenant != human.Tenant || p.Email != human.Email {
		t.Errorf("resolve = %+v %v", p, err)
	}
	for _, bad := range []string{"", "x", tok + "x", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if _, err := m.Resolve(context.Background(), bad); !errors.Is(err, auth.ErrSessionInvalid) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

// A session in steady use still ends 12 hours after sign-in.
func TestAbsoluteCap(t *testing.T) {
	m, _, c := newManager()
	tok, _ := m.Issue(context.Background(), human)
	for c.t = t0; c.t.Before(t0.Add(12 * time.Hour)); c.t = c.t.Add(30 * time.Minute) {
		if _, err := m.Resolve(context.Background(), tok); err != nil {
			t.Fatalf("at %v: %v", c.t.Sub(t0), err)
		}
	}
	c.t = t0.Add(12 * time.Hour)
	if _, err := m.Resolve(context.Background(), tok); !errors.Is(err, auth.ErrSessionInvalid) {
		t.Errorf("at 12h: %v; want the session ended", err)
	}
}

func TestIdleTimeout(t *testing.T) {
	m, st, c := newManager()
	tok, _ := m.Issue(context.Background(), human)
	c.t = t0.Add(2*time.Hour - time.Second)
	if _, err := m.Resolve(context.Background(), tok); err != nil {
		t.Fatalf("just under 2h idle: %v", err)
	}
	c.t = c.t.Add(2 * time.Hour)
	if _, err := m.Resolve(context.Background(), tok); !errors.Is(err, auth.ErrSessionInvalid) {
		t.Errorf("2h idle: %v", err)
	}
	if st.Len() != 0 {
		t.Errorf("an expired session was left behind")
	}
}

func TestTouchAtMostOnceAMinute(t *testing.T) {
	m, st, c := newManager()
	tok, _ := m.Issue(context.Background(), human)
	for i := range 120 { // two minutes of requests, one a second
		c.t = t0.Add(time.Duration(i) * time.Second)
		if _, err := m.Resolve(context.Background(), tok); err != nil {
			t.Fatal(err)
		}
	}
	if st.Touches != 1 {
		t.Errorf("touches = %d over two minutes; want 1 (at the one-minute mark)", st.Touches)
	}
}

func TestAllowlistIsCheckedEveryRequest(t *testing.T) {
	m, _, _ := newManager()
	tok, _ := m.Issue(context.Background(), human)
	delete(m.Allowed, "prn_human01")
	if _, err := m.Resolve(context.Background(), tok); !errors.Is(err, auth.ErrSessionNotAllowed) {
		t.Errorf("after removal from the allowlist: %v", err)
	}
}

func TestRevoke(t *testing.T) {
	m, st, _ := newManager()
	tok, _ := m.Issue(context.Background(), human)
	s, err := m.Revoke(context.Background(), tok)
	if err != nil || s.Principal != "prn_human01" || st.Len() != 0 {
		t.Fatalf("revoke: %+v %v, %d rows", s, err, st.Len())
	}
	if _, err := m.Resolve(context.Background(), tok); !errors.Is(err, auth.ErrSessionInvalid) {
		t.Errorf("after sign-out: %v", err)
	}
}

func TestStoreDown(t *testing.T) {
	m, st, _ := newManager()
	tok, _ := m.Issue(context.Background(), human)
	st.Fail = ErrUnavailable
	if _, err := m.Resolve(context.Background(), tok); !errors.Is(err, auth.ErrSessionUnavailable) {
		t.Errorf("store down: %v", err)
	}
}
