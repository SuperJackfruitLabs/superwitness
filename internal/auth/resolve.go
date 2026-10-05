package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrPrincipalSuspended is a token whose principal the hub has suspended.
var ErrPrincipalSuspended = errors.New("principal suspended")

// Resolving verifies a token with Inner, then, when its subject is not a principal id, resolves
// it through Principals. The hub's tokens for people (GET /api/auth/token and the browser
// exchange) carry the person's hub account id as sub, while agent and service tokens carry the
// prn_ id. Resolving makes every caller a prn_ id, so a person is the same judge, and the same
// allowlist entry, whether they come with a bearer token or a session.
type Resolving struct {
	Inner      Authenticator
	Principals PrincipalLookup
}

func (a Resolving) Verify(ctx context.Context, tok string) (Principal, error) {
	p, err := a.Inner.Verify(ctx, tok)
	if err != nil || strings.HasPrefix(p.ID, "prn_") {
		return p, err
	}
	rec, err := a.Principals.Lookup(ctx, p.ID)
	switch {
	case errors.Is(err, ErrPrincipalNotFound):
		return Principal{}, fmt.Errorf("%w: %w", ErrUnauthenticated, ErrPrincipalNotFound)
	case err != nil:
		return Principal{}, fmt.Errorf("%w: %v", ErrLookupUnavailable, err)
	case rec.Suspended:
		return Principal{}, fmt.Errorf("%w: %w", ErrUnauthenticated, ErrPrincipalSuspended)
	case rec.Kind != p.Kind:
		return Principal{}, fmt.Errorf("%w: the token says %s, the principal record %s", ErrUnauthenticated, p.Kind, rec.Kind)
	}
	p.ID = rec.ID
	return p, nil
}
