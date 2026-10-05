// Package auth holds hub identity: caller principals, the outbound service-token client,
// and offline verification of hub-issued JWTs.
package auth

import (
	"context"
	"errors"
)

type PrincipalKind string

const (
	KindHuman   PrincipalKind = "human"
	KindAgent   PrincipalKind = "agent"
	KindService PrincipalKind = "service"
)

// Principal is an authenticated caller. Kind is the hub-signed principalKind claim,
// which the hub sets from the principal record when it mints the token.
type Principal struct {
	ID     string
	Kind   PrincipalKind
	Tenant string
	Actor  string // RFC 8693 act.sub when a service spoke for the subject; recorded, never trusted for reach
}

// PrincipalRecord is the hub's principal record as superwitness needs it.
type PrincipalRecord struct {
	ID   string // prn_…
	Kind PrincipalKind
}

// PrincipalLookup reads a principal record from the hub's principals route. It accepts a
// prn_… id or a hub auth user id (superpipeline's decided_by_hub_sub) and always answers with the prn_….
type PrincipalLookup interface {
	Lookup(ctx context.Context, idOrHubSub string) (PrincipalRecord, error)
}

var (
	ErrPrincipalNotFound = errors.New("principal not found")
	ErrLookupUnavailable = errors.New("principal lookup unavailable")
)

type ctxKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}
