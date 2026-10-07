package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
)

// Manager issues and checks sessions. Allowed is SW_ALLOWED_PRINCIPALS, re-checked on every
// request, so taking a principal off the list ends its sessions at the next restart.
//
// Grants is set under the organization plane: each session then holds the plane's refresh token
// and lives only as long as that grant does.
type Manager struct {
	Store   Store
	Allowed map[string]bool
	Now     func() time.Time
	Grants  Grants
	Logger  *slog.Logger

	flights singleflight.Group // one refresh at a time per session
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
	return m.IssueGranted(ctx, p, TokenSet{})
}

// IssueGranted is Issue for a sign-in that came with tokens. Under the organization plane the
// refresh token is sealed into the session (see PlaneGrant), and the access token's life sets
// when the first refresh is due. The hub's token set is not kept.
func (m *Manager) IssueGranted(ctx context.Context, p auth.Principal, set TokenSet) (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	id, _ := hashToken(token)
	now := m.now()
	s := Session{IDHash: id, Principal: p.ID, PrincipalKind: p.Kind, Tenant: p.Tenant, Email: p.Email,
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(AbsoluteTTL)}
	if m.Grants != nil && set.Refresh != "" {
		sealed, err := sealRefresh(token, id, set.Refresh)
		if err != nil {
			return "", err
		}
		s.Grant = PlaneGrant{Sealed: sealed, AccessExpiresAt: now.Add(accessLife(set.ExpiresIn))}
	}
	if err := m.Store.Create(ctx, s); err != nil {
		return "", err
	}
	return token, nil
}

// Resolve implements auth.SessionResolver. Under the organization plane a session whose access
// token is past its life is refreshed first, and ends if the plane refuses.
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
	if m.Grants != nil {
		if len(s.Grant.Sealed) == 0 {
			// Issued before sessions held a grant: nothing binds it to the plane, so it ends.
			return auth.Principal{}, m.end(ctx, s, "no_grant")
		}
		if !now.Before(s.Grant.AccessExpiresAt) {
			if err := m.renew(ctx, token, id); err != nil {
				return auth.Principal{}, err
			}
		}
	}
	if now.Sub(s.LastSeenAt) >= TouchEvery {
		if err := m.Store.Touch(ctx, id, now); err != nil {
			return auth.Principal{}, auth.ErrSessionUnavailable
		}
	}
	return auth.Principal{ID: s.Principal, Kind: s.PrincipalKind, Tenant: s.Tenant, Email: s.Email}, nil
}

// renew refreshes the session's grant, once at a time per session: refresh tokens rotate, so two
// concurrent refreshes would spend one token twice. Concurrent requests wait for the one refresh
// and share its outcome. The flight is not tied to the first request's cancellation.
func (m *Manager) renew(ctx context.Context, token string, id []byte) error {
	_, err, _ := m.flights.Do(string(id), func() (any, error) {
		return nil, m.refresh(context.WithoutCancel(ctx), token, id)
	})
	return err
}

func (m *Manager) refresh(ctx context.Context, token string, id []byte) error {
	// Read again: a flight that just ended may already have refreshed this session.
	s, err := m.Store.Get(ctx, id)
	switch {
	case errors.Is(err, ErrNotFound):
		return auth.ErrSessionInvalid
	case err != nil:
		return auth.ErrSessionUnavailable
	}
	now := m.now()
	if now.Before(s.Grant.AccessExpiresAt) {
		return nil
	}
	rt, err := openRefresh(token, id, s.Grant.Sealed)
	if err != nil {
		return m.end(ctx, s, "grant_unreadable")
	}
	p, set, err := m.Grants.Refresh(ctx, rt)
	if err == nil && (p.ID != s.Principal || p.Kind != auth.KindHuman) {
		err = ErrGrantRefused // the grant now names someone else: not this session's
	}
	switch {
	case errors.Is(err, ErrGrantRefused):
		return m.end(ctx, s, "grant_refused")
	case err != nil:
		// Unreachable is not refused: keep the session for a bounded grace, and try again at most
		// every RetryEvery rather than on every request.
		since := s.Grant.UnreachableSince
		if since.IsZero() {
			since = now
		}
		if now.Sub(since) >= UnreachableGrace {
			return m.end(ctx, s, "issuer_unreachable")
		}
		m.log().Warn("auth.refresh_unavailable", "principal", s.Principal, "since", since, "err", err)
		g := s.Grant
		// The last retry falls on the grace's end, so the session never outlives it unchecked.
		next := now.Add(RetryEvery)
		if end := since.Add(UnreachableGrace); end.Before(next) {
			next = end
		}
		g.AccessExpiresAt, g.UnreachableSince = next, since
		if err := m.Store.SetGrant(ctx, id, g); err != nil {
			return auth.ErrSessionUnavailable
		}
		return nil
	}
	if set.Refresh == "" {
		set.Refresh = rt // not rotated: keep the one we hold
	}
	sealed, err := sealRefresh(token, id, set.Refresh)
	if err != nil {
		return auth.ErrSessionUnavailable
	}
	// If this write fails the rotated token is lost here, but not the session: the request is
	// told to retry, and a retry within the plane's 30-second replay window spends the old token
	// again and gets this same answer back.
	if err := m.Store.SetGrant(ctx, id, PlaneGrant{Sealed: sealed, AccessExpiresAt: now.Add(accessLife(set.ExpiresIn))}); err != nil {
		return auth.ErrSessionUnavailable
	}
	return nil
}

// end deletes a session whose grant is over and logs why.
func (m *Manager) end(ctx context.Context, s Session, reason string) error {
	if err := m.Store.Delete(ctx, s.IDHash); err != nil {
		return auth.ErrSessionUnavailable
	}
	m.log().Info("auth.session_ended", "principal", s.Principal, "reason", reason)
	return auth.ErrSessionInvalid
}

// Revoke ends the session behind token, if there is one, and says whose it was. Under the
// organization plane its refresh token is revoked at the plane first; a revocation that fails is
// logged and the session is dropped all the same.
func (m *Manager) Revoke(ctx context.Context, token string) (Session, error) {
	id, ok := hashToken(token)
	if !ok {
		return Session{}, ErrNotFound
	}
	s, err := m.Store.Get(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if m.Grants != nil && len(s.Grant.Sealed) > 0 {
		rt, err := openRefresh(token, id, s.Grant.Sealed)
		if err == nil {
			err = m.Grants.Revoke(ctx, rt)
		}
		if err != nil {
			m.log().Warn("auth.revoke_failed", "principal", s.Principal, "err", err)
		}
	}
	return s, m.Store.Delete(ctx, id)
}

func (m *Manager) log() *slog.Logger {
	if m.Logger != nil {
		return m.Logger
	}
	return slog.New(slog.DiscardHandler)
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
