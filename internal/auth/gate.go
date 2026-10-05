package auth

import (
	"context"
	"errors"
)

// SessionResolver turns a session cookie's value into the principal it was issued to.
type SessionResolver interface {
	Resolve(ctx context.Context, token string) (Principal, error)
}

var (
	ErrSessionInvalid     = errors.New("session unknown or ended")
	ErrSessionNotAllowed  = errors.New("the session's principal is no longer allowed")
	ErrSessionUnavailable = errors.New("session store unavailable")
)
