package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

const maxVerdictBody = 64 << 10

func (o *Ops) RecordVerdict(ctx context.Context, caller auth.Principal, req verdicts.Request) (verdicts.Verdict, bool, error) {
	v, created, err := o.Verdicts.Record(ctx, caller, req)
	if err != nil {
		return verdicts.Verdict{}, false, AsAPIError(err)
	}
	return v, created, nil
}

func (s *Server) postVerdict(w http.ResponseWriter, r *http.Request) {
	caller, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		writeError(w, &APIError{Status: http.StatusUnauthorized, Code: "unauthenticated", Message: "no caller"})
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxVerdictBody))
	dec.DisallowUnknownFields()
	var req verdicts.Request
	if err := dec.Decode(&req); err != nil {
		msg := "the body must be one JSON verdict request"
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			msg = "the body is larger than 64 KiB"
		}
		writeError(w, badRequest("invalid_json", msg+": "+err.Error()))
		return
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeError(w, badRequest("invalid_json", "the body must hold exactly one JSON object"))
		return
	}
	v, created, err := s.Ops.RecordVerdict(r.Context(), caller, req)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, v)
}
