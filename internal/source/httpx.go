package source

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
)

const maxBody = 8 << 20

// GetJSON GETs url and decodes a 200 body into out. A 404 is not_found only when
// isNotFound recognises the body as the contract's own not-found answer; a bare 404
// (a route not deployed yet) is unavailable, so a missing route never reads as a missing run.
func GetJSON(ctx context.Context, hc *http.Client, url, bearer string, out any, isNotFound func([]byte) bool) (int, SourceStatus) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, StatusUnavailable
	}
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, ClassifyErr(ctx, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return resp.StatusCode, ClassifyErr(ctx, err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		if out != nil {
			dec := json.NewDecoder(bytes.NewReader(body))
			dec.UseNumber()
			if err := dec.Decode(out); err != nil {
				return resp.StatusCode, StatusUnavailable
			}
		}
		return resp.StatusCode, StatusOK
	case http.StatusUnauthorized, http.StatusForbidden:
		return resp.StatusCode, StatusUnauthorized
	case http.StatusNotFound:
		if isNotFound != nil && isNotFound(body) {
			return resp.StatusCode, StatusNotFound
		}
		return resp.StatusCode, StatusUnavailable
	default:
		return resp.StatusCode, StatusUnavailable
	}
}

// PingURL reports reachability: any 2xx is ok.
func PingURL(ctx context.Context, hc *http.Client, url, bearer string) SourceStatus {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return StatusUnavailable
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return ClassifyErr(ctx, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return StatusOK
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return StatusUnauthorized
	default:
		return StatusUnavailable
	}
}

func ClassifyErr(ctx context.Context, err error) SourceStatus {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return StatusTimeout
	}
	return StatusUnavailable
}

// TokenStatus maps a TokenSource failure to the status the source reports.
func TokenStatus(ctx context.Context, err error) SourceStatus {
	if errors.Is(err, auth.ErrTokenRejected) {
		return StatusUnauthorized
	}
	return ClassifyErr(ctx, err)
}
