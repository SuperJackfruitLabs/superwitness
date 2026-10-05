package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
)

type Server struct {
	Ops    *Ops
	Auth   auth.Authenticator
	Health http.Handler // GET /health
	MCP    http.Handler // /mcp
	Web    http.Handler // the run page
	Logger *slog.Logger // audit lines; nil discards them
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	if s.Health != nil {
		r.Method(http.MethodGet, "/health", s.Health)
	}
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(s.Auth))
		r.Get("/v1/runs/superpipeline/{boardId}/{runId}", s.getRun)
		r.Get("/v1/runs/superpipeline/{boardId}/{runId}/spans", s.listSpans)
		r.Get("/v1/runs/superpipeline/{boardId}/{runId}/logs", s.listLogs)
		r.Get("/v1/runs/by-attempt/{attemptId}", s.byAttempt)
		r.Post("/v1/verdicts", s.postVerdict)
		r.Get("/v1/runs", s.getRuns)
		r.Post("/v1/runs", s.postRuns)
		if s.MCP != nil {
			r.Handle("/mcp", s.MCP)
		}
	})
	r.NotFound(s.notFound)
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, &APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "method not allowed"})
	})
	return r
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
