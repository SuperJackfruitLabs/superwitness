// Package api is superwitness's /v1 HTTP surface. Ops holds the operations; the HTTP
// handlers and the MCP tools are both thin layers over Ops, so the two cannot drift.
package api

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

type APIError struct {
	Status    int    `json:"-"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
	Index     *int   `json:"index,omitempty"` // the refused report's position in a POST /v1/runs batch
	RetryIn   int    `json:"-"`               // seconds for Retry-After on a 429
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func AsAPIError(err error) *APIError {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae
	}
	var ve *verdicts.Error
	if errors.As(err, &ve) {
		return &APIError{Status: ve.Status, Code: ve.Code, Message: ve.Message, Retryable: ve.Retryable}
	}
	return &APIError{Status: http.StatusInternalServerError, Code: "internal", Message: "internal error"}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	ae := AsAPIError(err)
	switch {
	case ae.RetryIn > 0:
		w.Header().Set("Retry-After", strconv.Itoa(ae.RetryIn))
	case ae.Retryable && ae.Status == http.StatusServiceUnavailable:
		w.Header().Set("Retry-After", "5")
	}
	writeJSON(w, ae.Status, map[string]any{"error": ae})
}

func sourceError(name source.Name, st source.SourceStatus) *APIError {
	switch st {
	case source.StatusTimeout:
		return &APIError{Status: http.StatusGatewayTimeout, Code: string(name) + "_timeout",
			Message: string(name) + " did not answer in time", Retryable: true}
	case source.StatusUnauthorized:
		return &APIError{Status: http.StatusBadGateway, Code: string(name) + "_unauthorized",
			Message: string(name) + " refused superwitness's credential"}
	default:
		return &APIError{Status: http.StatusServiceUnavailable, Code: string(name) + "_unavailable",
			Message: string(name) + " is unavailable", Retryable: true}
	}
}

func badRequest(code, msg string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: code, Message: msg}
}

// retryAfterSeconds is a wait as Retry-After sends it: whole seconds, rounded up, and never 0,
// so a client that waits exactly that long finds a token back.
func retryAfterSeconds(wait time.Duration) int {
	return max(1, int(math.Ceil(wait.Seconds())))
}
