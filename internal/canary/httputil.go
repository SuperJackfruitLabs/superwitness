package canary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func orDefault(hc *http.Client) *http.Client {
	if hc == nil {
		return http.DefaultClient
	}
	return hc
}

// apiError carries the status and the server's error code, never the body: a body can
// echo the request, and the request carries the planted marker.
type apiError struct {
	Status int
	Code   string
}

func (e *apiError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("HTTP %d %s", e.Status, e.Code)
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

func errorCode(raw []byte) string {
	var a struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(raw, &a) != nil || len(a.Error) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(a.Error, &s) == nil {
		return s
	}
	var o struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(a.Error, &o) == nil {
		return o.Code
	}
	return ""
}

func doJSON(ctx context.Context, hc *http.Client, method, url, token string, body any, want int, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := orDefault(hc).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != want {
		return &apiError{Status: resp.StatusCode, Code: errorCode(raw)}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}
