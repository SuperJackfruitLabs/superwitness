package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
)

// Manager issues and checks sessions. Allowed is SW_ALLOWED_PRINCIPALS, re-checked on every
// request, so taking a principal off the list ends its sessions at the next restart.
type Manager struct {
	Store   Store
	Allowed map[string]bool
	Now     func() time.Time
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

// hashToken reads a cookie value: 32 random bytes, base64url. ok is false for anything else.
func hashToken(token string) ([]byte, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return sum[:], true
}

// Issue creates a session for p and returns the cookie value. Only the value's hash is stored.
func (m *Manager) Issue(ctx context.Context, p auth.Principal) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	id, _ := hashToken(token)
	now := m.now()
	err := m.Store.Create(ctx, Session{IDHash: id, Principal: p.ID, PrincipalKind: p.Kind, Tenant: p.Tenant, Email: p.Email,
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(AbsoluteTTL)})
	if err != nil {
		return "", err
	}
	return token, nil
}

// Resolve implements auth.SessionResolver.
func (m *Manager) Resolve(ctx context.Context, token string) (auth.Principal, error) {
	id, ok := hashToken(token)
	if !ok {
		return auth.Principal{}, auth.ErrSessionInvalid
	}
	s, err := m.Store.Get(ctx, id)
	switch {
	case errors.Is(err, ErrNotFound):
		return auth.Principal{}, auth.ErrSessionInvalid
	case err != nil:
		return auth.Principal{}, auth.ErrSessionUnavailable
	}
	now := m.now()
	if s.Expired(now) {
		_ = m.Store.Delete(ctx, id) // the sweep would remove it anyway
		return auth.Principal{}, auth.ErrSessionInvalid
	}
	if !m.Allowed[s.Principal] {
		return auth.Principal{}, auth.ErrSessionNotAllowed
	}
	if now.Sub(s.LastSeenAt) >= TouchEvery {
		if err := m.Store.Touch(ctx, id, now); err != nil {
			return auth.Principal{}, auth.ErrSessionUnavailable
		}
	}
	return auth.Principal{ID: s.Principal, Kind: s.PrincipalKind, Tenant: s.Tenant, Email: s.Email}, nil
}

// Revoke ends the session behind token, if there is one, and says whose it was.
func (m *Manager) Revoke(ctx context.Context, token string) (Session, error) {
	id, ok := hashToken(token)
	if !ok {
		return Session{}, ErrNotFound
	}
	s, err := m.Store.Get(ctx, id)
	if err != nil {
		return Session{}, err
	}
	return s, m.Store.Delete(ctx, id)
}

// Sweep deletes expired sessions every interval until ctx ends.
func (m *Manager) Sweep(ctx context.Context, every time.Duration, logger *slog.Logger) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := m.Store.Sweep(ctx, m.now())
			if err != nil {
				logger.Warn("session sweep failed", "err", err)
			} else if n > 0 {
				logger.Info("session sweep", "deleted", n)
			}
		}
	}
}
