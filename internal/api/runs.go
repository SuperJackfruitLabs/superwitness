package api

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/runs"
)

func (s *Server) log() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.New(slog.DiscardHandler)
}

// postRuns is the run registry's ingest. Only a service principal holding runs:write may report.
func (s *Server) postRuns(w http.ResponseWriter, r *http.Request) {
	caller, _ := auth.PrincipalFrom(r.Context())
	results, err := s.reportRuns(r, caller)
	if err != nil {
		ae := AsAPIError(err)
		attrs := []any{"principal", caller.ID, "code", ae.Code}
		if ae.Index != nil {
			attrs = append(attrs, "index", *ae.Index)
		}
		s.log().Warn("runs.report_rejected", attrs...)
		writeError(w, ae)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) reportRuns(r *http.Request, caller auth.Principal) ([]ReportResult, error) {
	if caller.Kind != auth.KindService {
		return nil, &APIError{Status: http.StatusForbidden, Code: "service_principal_required",
			Message: "runs are reported by service principals only"}
	}
	if !caller.HasScope(runs.WriteScope) {
		return nil, &APIError{Status: http.StatusForbidden, Code: "insufficient_scope",
			Message: "the token does not carry the " + runs.WriteScope + " scope"}
	}
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, runs.MaxBodyBytes))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return nil, &APIError{Status: http.StatusRequestEntityTooLarge, Code: "body_too_large", Message: "the body is larger than 256 KiB"}
	}
	if err != nil {
		return nil, badRequest("invalid_json", "the body could not be read")
	}
	batch, isBatch, err := runs.ParseBody(body, s.Ops.now())
	var be *runs.BodyError
	var ve *runs.ValidationError
	switch {
	case errors.As(err, &be):
		return nil, badRequest("invalid_json", be.Msg)
	case errors.As(err, &ve):
		ae := &APIError{Status: http.StatusUnprocessableEntity, Code: "invalid_report", Message: ve.Error()}
		if ve.Index >= 0 {
			idx := ve.Index
			ae.Index = &idx
		}
		return nil, ae
	case err != nil:
		return nil, badRequest("invalid_json", err.Error())
	}
	results, err := s.Ops.ReportRuns(r.Context(), caller, batch)
	var ae *APIError
	if !isBatch && errors.As(err, &ae) {
		ae.Index = nil // a single report's errors carry no position
	}
	return results, err
}

func (s *Server) getRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := RunQuery{Source: q.Get("source"), Scope: q.Get("scope"), Status: q["status"], Executor: q.Get("executor"),
		Since: q.Get("since"), Until: q.Get("until"), Cursor: q.Get("cursor")}
	switch q.Get("needs_verdict") {
	case "", "false":
	case "true":
		query.NeedsVerdict = true
	default:
		writeError(w, badRequest("invalid_needs_verdict", "needs_verdict is true or false"))
		return
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, badRequest("invalid_limit", "limit must be a positive integer"))
			return
		}
		query.Limit = n
	}
	page, err := s.Ops.ListRuns(r.Context(), query)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}
