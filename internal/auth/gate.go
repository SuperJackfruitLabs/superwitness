package auth

import (
	"context"
	"errors"
	"net/http"
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

type viaSessionKey struct{}

// ViaSession reports whether the request was admitted by a session cookie, not a bearer token.
func ViaSession(ctx context.Context) bool {
	v, _ := ctx.Value(viaSessionKey{}).(bool)
	return v
}

// Gate admits a request by its bearer token or, when it carries no Authorization header at all,
// by its session cookie. A bearer token is verified exactly as Middleware does and the cookie is
// then ignored. Writes made with a session must carry Origin, equal to the public URL.
type Gate struct {
	Bearer   Authenticator
	Sessions SessionResolver // nil: bearer tokens only
	Cookie   string          // the session cookie's name
	Origin   string          // SW_PUBLIC_URL
}

func (g Gate) Middleware(next http.Handler) http.Handler {
	bearer := Middleware(g.Bearer)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.Sessions == nil || r.Header.Get("Authorization") != "" {
			bearer.ServeHTTP(w, r)
			return
		}
		c, err := r.Cookie(g.Cookie)
		if err != nil || c.Value == "" {
			unauthorized(w, "sign in, or send a hub-issued bearer token")
			return
		}
		p, err := g.Sessions.Resolve(r.Context(), c.Value)
		switch {
		case errors.Is(err, ErrSessionNotAllowed):
			writeAuthError(w, http.StatusForbidden, "not_authorised", "this account is not allowed to use superwitness")
			return
		case errors.Is(err, ErrSessionUnavailable):
			w.Header().Set("Retry-After", "5")
			writeAuthError(w, http.StatusServiceUnavailable, "store_unavailable", "the session store is unavailable; retry")
			return
		case err != nil:
			unauthorized(w, "the session has ended; sign in again")
			return
		}
		if !SafeMethod(r.Method) && r.Header.Get("Origin") != g.Origin {
			writeAuthError(w, http.StatusForbidden, "origin_mismatch", "a write made with a session must come from "+g.Origin)
			return
		}
		ctx := context.WithValue(WithPrincipal(r.Context(), p), viaSessionKey{}, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SafeMethod is a method that changes nothing: GET, HEAD or OPTIONS.
func SafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}
