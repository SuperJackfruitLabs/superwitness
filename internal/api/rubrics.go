package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

var (
	rubricID      = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	rubricVersion = regexp.MustCompile(`^[1-9][0-9]{0,8}$`)
)

type RubricView struct {
	ID              string          `json:"id"`
	Version         int             `json:"version"`
	Standard        string          `json:"standard"` // what a verdict names: rubric:<id>@<version>
	Name            string          `json:"name"`
	Scale           json.RawMessage `json:"scale"`
	RecognisedScale *verdicts.Scale `json:"recognised_scale"` // null: the app lists the rubric but cannot offer it
	Body            *string         `json:"body,omitempty"`
	CreatedBy       string          `json:"created_by"`
	CreatedAt       time.Time       `json:"created_at"`
}

func rubricView(r verdicts.Rubric, withBody bool) RubricView {
	v := RubricView{ID: r.ID, Version: r.Version, Standard: fmt.Sprintf("rubric:%s@%d", r.ID, r.Version), Name: r.Name,
		Scale: r.Scale, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC()}
	if sc, ok := verdicts.RecogniseScale(r.Scale); ok {
		v.RecognisedScale = &sc
	}
	if withBody {
		body := r.Body
		v.Body = &body
	}
	return v
}

func rubricsDown() *APIError {
	return &APIError{Status: http.StatusServiceUnavailable, Code: "store_unavailable",
		Message: "the verdict store is unavailable; retry", Retryable: true}
}

func (o *Ops) ListRubrics(ctx context.Context) ([]RubricView, error) {
	rs, err := o.Rubrics.ListRubrics(ctx)
	if err != nil {
		return nil, rubricsDown()
	}
	out := make([]RubricView, 0, len(rs))
	for _, r := range rs {
		out = append(out, rubricView(r, false))
	}
	return out, nil
}

func (o *Ops) GetRubric(ctx context.Context, id, version string) (RubricView, error) {
	if !rubricID.MatchString(id) || !rubricVersion.MatchString(version) {
		return RubricView{}, badRequest("invalid_rubric_ref", "a rubric is /v1/rubrics/<id>/<version>: id of letters, digits, _, . and -, version 1 or more")
	}
	n, _ := strconv.Atoi(version)
	r, err := o.Rubrics.GetRubric(ctx, id, n)
	switch {
	case errors.Is(err, verdicts.ErrRubricNotFound):
		return RubricView{}, &APIError{Status: http.StatusNotFound, Code: "rubric_not_found", Message: fmt.Sprintf("no rubric %s version %d", id, n)}
	case err != nil:
		return RubricView{}, rubricsDown()
	}
	return rubricView(r, true), nil
}

func (s *Server) listRubrics(w http.ResponseWriter, r *http.Request) {
	rs, err := s.Ops.ListRubrics(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rubrics": rs})
}

func (s *Server) getRubric(w http.ResponseWriter, r *http.Request) {
	v, err := s.Ops.GetRubric(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "version"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
