// Package session is superwitness's browser sign-in: the AgentPod authorize-and-exchange flow,
// its own sessions (the browser never holds a hub token), and the cookies that carry them.
package session

import (
	"context"
	"errors"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
)

const (
	AbsoluteTTL = 12 * time.Hour   // a session ends this long after sign-in, used or not
	IdleTTL     = 2 * time.Hour    // or this long after it was last used
	TouchEvery  = time.Minute      // last_seen_at is written at most this often
	SweepEvery  = 10 * time.Minute // expired rows are deleted this often

	// Under the organization plane a session holds the plane's refresh token and refreshes it when
	// the access token it last got is past its life (at most AccessLife).
	AccessLife       = 5 * time.Minute
	UnreachableGrace = 10 * time.Minute // a plane that cannot be reached for this long ends the session
	RetryEvery       = 30 * time.Second // while the plane cannot be reached, a refresh is tried this often
)

// Session is one signed-in browser. IDHash is the sha256 of the cookie's 32 random bytes.
type Session struct {
	IDHash        []byte
	Principal     string
	PrincipalKind auth.PrincipalKind
	Tenant        string
	Email         string // "" when the hub sent none
	CreatedAt     time.Time
	LastSeenAt    time.Time
	ExpiresAt     time.Time
	Grant         PlaneGrant // zero outside the organization plane
}

// PlaneGrant is the organization plane's refresh-token grant behind a session. Sealed is the
// refresh token encrypted under a key derived from the session cookie, which the store never
// holds. AccessExpiresAt is when the next refresh is due. UnreachableSince is when a refresh
// first failed because the plane could not be reached, zero when the last one got an answer.
type PlaneGrant struct {
	Sealed           []byte
	AccessExpiresAt  time.Time
	UnreachableSince time.Time
}

// Expired reports whether s has passed its absolute or its idle limit at now.
func (s Session) Expired(now time.Time) bool {
	return !now.Before(s.ExpiresAt) || !now.Before(s.LastSeenAt.Add(IdleTTL))
}

var (
	ErrNotFound    = errors.New("session not found")
	ErrUnavailable = errors.New("session store unavailable")
)

type Store interface {
	Create(ctx context.Context, s Session) error
	Get(ctx context.Context, idHash []byte) (Session, error)
	Touch(ctx context.Context, idHash []byte, at time.Time) error
	// SetGrant replaces the session's plane grant. A missing session is not an error.
	SetGrant(ctx context.Context, idHash []byte, g PlaneGrant) error
	Delete(ctx context.Context, idHash []byte) error
	// Sweep deletes every session expired at now and says how many.
	Sweep(ctx context.Context, now time.Time) (int64, error)
}
