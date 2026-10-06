package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// planeTokenServer mimics POST /api/token/service: Bearer svc_01:s3cret, body {"audience": …}.
func planeTokenServer(t *testing.T, hits *atomic.Int32, status int, answer func(aud string, n int32) string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/token/service" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer svc_01:s3cret" {
			t.Errorf("Authorization = %q", got)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		var body struct {
			Audience string `json:"audience"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Audience == "" {
			t.Errorf("body: %v %+v", err, body)
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answer(body.Audience, n)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func okAnswer(aud string, n int32) string {
	return `{"access_token":"tok-` + strconv.Itoa(int(n)) + `@` + aud + `","token_type":"Bearer","expires_in":300}`
}

func TestPlaneTokenSourceAsksForItsAudience(t *testing.T) {
	var hits atomic.Int32
	srv := planeTokenServer(t, &hits, http.StatusOK, okAnswer)
	ts := NewPlaneTokenSource(srv.URL+"/", "svc_01:s3cret", "https://hub.example", srv.Client())
	a, err := ts.Token(context.Background())
	if err != nil || a != "tok-1@https://hub.example" {
		t.Fatalf("token = %q, %v", a, err)
	}
	if b, _ := ts.Token(context.Background()); b != a || hits.Load() != 1 {
		t.Errorf("not cached: %q, hits %d", b, hits.Load())
	}
}

func TestPlaneTokenSourceRefreshesBeforeExpiry(t *testing.T) {
	var hits atomic.Int32
	srv := planeTokenServer(t, &hits, http.StatusOK, okAnswer)
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	ts := NewPlaneTokenSource(srv.URL, "svc_01:s3cret", "https://hub.example", srv.Client())
	ts.Now = func() time.Time { return now }
	_, _ = ts.Token(context.Background())
	now = now.Add(4*time.Minute + 31*time.Second) // inside the 30 s refresh margin of a 300 s token
	if tok, _ := ts.Token(context.Background()); !strings.HasPrefix(tok, "tok-2@") {
		t.Errorf("not refreshed: %q", tok)
	}
}

func TestPlaneTokenSourceRejectedOnlyOnRefusals(t *testing.T) {
	for _, code := range []int{400, 401, 403, 404, 423} {
		var hits atomic.Int32
		srv := planeTokenServer(t, &hits, code, okAnswer)
		_, err := NewPlaneTokenSource(srv.URL, "svc_01:s3cret", "https://hub.example", srv.Client()).Token(context.Background())
		if !errors.Is(err, ErrTokenRejected) || strings.Contains(err.Error(), "s3cret") {
			t.Errorf("HTTP %d: %v", code, err)
		}
	}
	var hits atomic.Int32
	srv := planeTokenServer(t, &hits, http.StatusBadGateway, okAnswer)
	_, err := NewPlaneTokenSource(srv.URL, "svc_01:s3cret", "https://hub.example", srv.Client()).Token(context.Background())
	if err == nil || errors.Is(err, ErrTokenRejected) {
		t.Errorf("HTTP 502 must be an outage, not a refusal: %v", err)
	}
}

func TestPlaneTokenSourceRefusesMalformedAnswers(t *testing.T) {
	for name, body := range map[string]string{
		"no token":      `{"token_type":"Bearer","expires_in":300}`,
		"not a bearer":  `{"access_token":"x","token_type":"DPoP","expires_in":300}`,
		"no lifetime":   `{"access_token":"x","token_type":"Bearer","expires_in":0}`,
		"the hub shape": `{"token":"x","expiresIn":300}`,
		"not json":      `<html>`,
	} {
		var hits atomic.Int32
		srv := planeTokenServer(t, &hits, http.StatusOK, func(string, int32) string { return body })
		if _, err := NewPlaneTokenSource(srv.URL, "svc_01:s3cret", "https://hub.example", srv.Client()).Token(context.Background()); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestPlaneTokenSourceInvalidate(t *testing.T) {
	var hits atomic.Int32
	srv := planeTokenServer(t, &hits, http.StatusOK, okAnswer)
	ts := NewPlaneTokenSource(srv.URL, "svc_01:s3cret", "https://hub.example", srv.Client())
	_, _ = ts.Token(context.Background())
	ts.Invalidate()
	if tok, _ := ts.Token(context.Background()); !strings.HasPrefix(tok, "tok-2@") {
		t.Errorf("after Invalidate: %q", tok)
	}
}

func TestParseServiceCredential(t *testing.T) {
	if got, err := ParseServiceCredential("svc_01:s3cret"); err != nil || got != "svc_01:s3cret" {
		t.Errorf("valid: %q %v", got, err)
	}
	for _, bad := range []string{"svc_01", "svc_01:", "svc_:s3cret", "dev_01:s3cret", "svc_01:s3 cret", "s3cret"} {
		_, err := ParseServiceCredential(bad)
		if err == nil || strings.Contains(err.Error(), "s3") {
			t.Errorf("%q: %v (must refuse, and never echo the secret)", bad, err)
		}
	}
}

func TestReadServiceCredential(t *testing.T) {
	dir := t.TempDir()
	ok := filepath.Join(dir, "ok")
	_ = os.WriteFile(ok, []byte("svc_01:s3cret\n"), 0o600)
	if got, err := ReadServiceCredential(ok); err != nil || got != "svc_01:s3cret" {
		t.Errorf("%q %v", got, err)
	}
	open := filepath.Join(dir, "open")
	_ = os.WriteFile(open, []byte("svc_01:s3cret\n"), 0o644)
	if _, err := ReadServiceCredential(open); err == nil {
		t.Error("a group-readable credential was accepted")
	}
}
