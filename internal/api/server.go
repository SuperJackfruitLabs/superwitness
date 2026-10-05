package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/ratelimit"
)

type Server struct {
	Ops    *Ops
	Auth   auth.Authenticator
	Health http.Handler // GET /health
	MCP    http.Handler // /mcp
	Web    http.Handler // the app
	Logger *slog.Logger // audit lines; nil discards them

	// Sign-in. Sessions nil means bearer tokens only, as before.
	Sessions      auth.SessionResolver
	SessionCookie string       // the session cookie's name
	PublicURL     string       // SW_PUBLIC_URL: the Origin a session write must carry
	Login         http.Handler // /auth/*; nil when sign-in is off
	Limits        *Limits      // nil: no rate limits (tests)

	// TrustedProxies (SW_TRUSTED_PROXIES) say which peers are the public edge's tunnel connector.
	// A request from one that carries CF-Connecting-IP came in over the internet, and /mcp is not
	// served to it. nil means this host's own addresses, as for the rate limits' client IP.
	TrustedProxies []netip.Prefix
}

// Handler is Build for callers that have already checked the Server; it panics on a
// misconfiguration Build would report.
func (s *Server) Handler() http.Handler {
	h, err := s.Build()
	if err != nil {
		panic(err)
	}
	return h
}

// Build returns the router. It refuses sessions without a public origin to hold writes to.
func (s *Server) Build() (http.Handler, error) {
	gate, err := auth.NewGate(s.Auth, s.Sessions, s.SessionCookie, s.PublicURL)
	if err != nil {
		return nil, err
	}
	trusted := s.TrustedProxies
	if trusted == nil {
		if trusted, err = ratelimit.HostPrefixes(); err != nil {
			return nil, fmt.Errorf("SW_TRUSTED_PROXIES: reading this host's addresses: %w", err)
		}
	}
	r := chi.NewRouter()
	r.Use(s.hideMCPFromEdge(trusted), middleware.Recoverer)
	if s.Health != nil {
		r.Method(http.MethodGet, "/health", s.Health)
	}
	r.Group(func(r chi.Router) {
		r.Use(gate.Middleware, s.limitSessionWrites)
		r.Get("/v1/me", s.me)
		r.Get("/v1/runs/superpipeline/{boardId}/{runId}", s.getRun)
		r.Get("/v1/runs/superpipeline/{boardId}/{runId}/spans", s.listSpans)
		r.Get("/v1/runs/superpipeline/{boardId}/{runId}/logs", s.listLogs)
		r.Get("/v1/runs/by-attempt/{attemptId}", s.byAttempt)
		r.Post("/v1/verdicts", s.postVerdict)
		r.Get("/v1/verdicts", s.verdictHistory)
		r.Post("/v1/runs", s.postRuns)
		r.Get("/v1/runs", s.getRuns)
		r.Get("/v1/scopes", s.listScopes)
		r.Get("/v1/rubrics", s.listRubrics)
		r.Get("/v1/rubrics/{id}/{version}", s.getRubric)
	})
	if s.MCP != nil {
		// Bearer tokens only: an agent's tool calls never ride on a person's browser session.
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(s.Auth))
			r.Handle("/mcp", s.MCP)
		})
	}
	if s.Login != nil {
		r.Mount("/auth", s.limitByIP(s.Login))
	}
	r.NotFound(s.notFound)
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, &APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "method not allowed"})
	})
	return r, nil
}

// hideMCPFromEdge answers 404 for /mcp, and for any spelling of it, to a request that came
// through the public edge, before authentication. MCP is for agents on the tailnet; a token
// leaked to the internet must not reach it, and a 401 would say it is there.
func (s *Server) hideMCPFromEdge(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if mcpLike(r.URL.Path) && ratelimit.ViaEdge(r, trusted) {
				writeError(w, &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "no such route"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// mcpLike reports whether a decoded path names /mcp or below it once case, repeated slashes and
// dot segments are ignored. The router itself matches only exactly /mcp; this is wider on purpose.
func mcpLike(p string) bool {
	c := path.Clean("/" + strings.ToLower(p))
	return c == "/mcp" || strings.HasPrefix(c, "/mcp/")
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	api := strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/v1" ||
		r.URL.Path == "/mcp" || strings.HasPrefix(r.URL.Path, "/mcp/")
	if s.Web != nil && !api && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		s.Web.ServeHTTP(w, r)
		return
	}
	writeError(w, &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "no such route"})
}
