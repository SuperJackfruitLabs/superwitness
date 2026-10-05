package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/runs"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

// me answers who the caller is, as the app needs to show it and to know which verdicts are theirs.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	via := "bearer"
	if auth.ViaSession(r.Context()) {
		via = "session"
	}
	var email *string
	if p.Email != "" {
		email = &p.Email
	}
	writeJSON(w, http.StatusOK, map[string]any{"principal": p.ID, "kind": p.Kind, "email": email, "via": via})
}

// HistoryVerdict is a recorded verdict with the id of the verdict that supersedes it, if any.
type HistoryVerdict struct {
	verdicts.Verdict
	SupersededBy *string `json:"superseded_by"`
}

// VerdictHistory is every verdict on one subject, superseded ones included, oldest first.
func (o *Ops) VerdictHistory(ctx context.Context, kind, ref string) ([]HistoryVerdict, error) {
	norm, err := verdicts.NormalizeSubject(verdicts.SubjectKind(kind), ref)
	if err != nil {
		return nil, AsAPIError(err)
	}
	vs, err := o.History.ListSubject(ctx, verdicts.SubjectKey{Kind: verdicts.SubjectKind(kind), Ref: norm})
	if err != nil {
		return nil, rubricsDown()
	}
	next := map[string]string{}
	for _, v := range vs {
		if v.Supersedes != nil {
			next[*v.Supersedes] = v.ID
		}
	}
	out := make([]HistoryVerdict, 0, len(vs))
	for _, v := range vs {
		h := HistoryVerdict{Verdict: v}
		if id, ok := next[v.ID]; ok {
			h.SupersededBy = &id
		}
		out = append(out, h)
	}
	return out, nil
}

func (s *Server) verdictHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	vs, err := s.Ops.VerdictHistory(r.Context(), q.Get("subject_kind"), q.Get("subject_ref"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"verdicts": vs})
}

func (o *Ops) ListScopes(ctx context.Context) ([]runs.Scope, error) {
	sc, err := o.Runs.Scopes(ctx)
	if errors.Is(err, runs.ErrUnavailable) {
		return nil, registryDown()
	}
	return sc, err
}

func (s *Server) listScopes(w http.ResponseWriter, r *http.Request) {
	sc, err := s.Ops.ListScopes(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scopes": sc})
}
