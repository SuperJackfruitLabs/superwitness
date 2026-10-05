package api

import (
	"net/http"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/ratelimit"
)

// Limits are the in-process rate limits. Any field may be nil, which turns that limit off.
type Limits struct {
	Auth          *ratelimit.Limiter // /auth/*, per client IP
	SessionWrites *ratelimit.Limiter // writes made with a session, per principal
	Reports       *ratelimit.Limiter // POST /v1/runs, per principal
	ClientIP      func(*http.Request) string
}

// The limits, as numbers: per minute, and the burst a bucket holds.
const (
	AuthPerMinute, AuthBurst                  = 10, 20
	SessionWritesPerMinute, SessionWriteBurst = 30, 10
	ReportsPerMinute, ReportBurst             = 600, 100
)

// rateLimited is the 429 for a request over its limit; Retry-After says when a token is back.
func rateLimited(wait time.Duration) *APIError {
	return &APIError{Status: http.StatusTooManyRequests, Code: "rate_limited", Retryable: true,
		Message: "too many requests; retry after Retry-After seconds", RetryIn: retryAfterSeconds(wait)}
}

func (s *Server) limitByIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l := s.Limits; l != nil && l.Auth != nil && l.ClientIP != nil {
			if ok, wait := l.Auth.Allow(l.ClientIP(r)); !ok {
				writeError(w, rateLimited(wait))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) limitSessionWrites(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l := s.Limits; l != nil && l.SessionWrites != nil && auth.ViaSession(r.Context()) && !auth.SafeMethod(r.Method) {
			p, _ := auth.PrincipalFrom(r.Context())
			if ok, wait := l.SessionWrites.Allow(p.ID); !ok {
				writeError(w, rateLimited(wait))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
