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
	Delete(ctx context.Context, idHash []byte) error
	// Sweep deletes every session expired at now and says how many.
	Sweep(ctx context.Context, now time.Time) (int64, error)
}
