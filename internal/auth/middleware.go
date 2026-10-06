package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
)

// Middleware authenticates the bearer token. The token's aud must be this service, which
// the issuer only mints for clients granted it, so every verified kind is admitted.
func Middleware(a Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			tok = strings.TrimSpace(tok)
			if !ok || tok == "" {
				unauthorized(w, "a bearer token is required")
				return
			}
			p, err := a.Verify(r.Context(), tok)
			if errors.Is(err, ErrLookupUnavailable) {
				w.Header().Set("Retry-After", "5")
				writeAuthError(w, http.StatusServiceUnavailable, "principal_unresolved",
					"the hub could not say which principal this token names; retry")
				return
			}
			var ne *NotEnabledError
			if errors.As(err, &ne) {
				WriteNotEnabled(w, ne.Org)
				return
			}
			if err != nil {
				unauthorized(w, "the bearer token is not valid for this service")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// WriteNotEnabled answers the issuer contract's entitlement refusal, in its shape rather than the
// API's: 403 {"error":"product_not_enabled","org":"<org_ id>"}.
func WriteNotEnabled(w http.ResponseWriter, org string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "product_not_enabled", "org": org})
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="superwitness"`)
	writeAuthError(w, http.StatusUnauthorized, "unauthenticated", msg)
}

// WriteError answers with the API's JSON error shape, for handlers outside the gate (sign-out).
func WriteError(w http.ResponseWriter, status int, code, msg string) {
	writeAuthError(w, status, code, msg)
}

func writeAuthError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

// DevAuthenticator accepts "dev:<principal id>:<kind>" with an optional fourth segment of
// comma-separated scopes, "dev:<id>:<kind>:<scope>,<scope>". Only SW_FAKE_SOURCES mode wires
// it, and config refuses that mode on a non-loopback listen address.
type DevAuthenticator struct{}

func (DevAuthenticator) Verify(_ context.Context, tok string) (Principal, error) {
	parts := strings.SplitN(tok, ":", 4)
	if len(parts) < 3 || parts[0] != "dev" || parts[1] == "" {
		return Principal{}, ErrUnauthenticated
	}
	k := PrincipalKind(parts[2])
	if k != KindHuman && k != KindAgent && k != KindService {
		return Principal{}, ErrUnauthenticated
	}
	p := Principal{ID: parts[1], Kind: k, Tenant: "dev"}
	if len(parts) == 4 {
		p.Scopes = strings.Split(parts[3], ",")
		if slices.Contains(p.Scopes, "") {
			return Principal{}, ErrUnauthenticated
		}
	}
	return p, nil
}
