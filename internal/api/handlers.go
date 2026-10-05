package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

func runRef(w http.ResponseWriter, r *http.Request) (source.RunRef, bool) {
	ref, err := source.NewSuperpipelineRef(chi.URLParam(r, "boardId"), chi.URLParam(r, "runId"))
	if err != nil {
		writeError(w, badRequest("invalid_run_ref", err.Error()))
		return source.RunRef{}, false
	}
	return ref, true
}

func limitParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	v := r.URL.Query().Get("limit")
	if v == "" {
		return 0, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		writeError(w, badRequest("invalid_limit", "limit must be a positive integer"))
		return 0, false
	}
	return n, true
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	ref, ok := runRef(w, r)
	if !ok {
		return
	}
	doc, err := s.Ops.GetRun(r.Context(), ref)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) listSpans(w http.ResponseWriter, r *http.Request) {
	ref, ok := runRef(w, r)
	if !ok {
		return
	}
	limit, ok := limitParam(w, r)
	if !ok {
		return
	}
	page, err := s.Ops.ListSpans(r.Context(), ref, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) listLogs(w http.ResponseWriter, r *http.Request) {
	ref, ok := runRef(w, r)
	if !ok {
		return
	}
	limit, ok := limitParam(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	page, err := s.Ops.ListLogs(r.Context(), ref, q.Get("cursor"), q.Get("level"), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) byAttempt(w http.ResponseWriter, r *http.Request) {
	ref, err := s.Ops.ResolveAttempt(r.Context(), chi.URLParam(r, "attemptId"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", "/v1/runs/superpipeline/"+ref.BoardID+"/"+ref.RunID)
	w.WriteHeader(http.StatusFound)
}
