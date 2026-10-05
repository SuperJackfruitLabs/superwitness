package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/config"
)

// buildOffline builds the fake-mode app; the pool connects lazily, so no database is needed.
func buildOffline(t *testing.T, env map[string]string) (*App, error) {
	t.Helper()
	env["SW_FAKE_SOURCES"] = "1"
	env["SW_DATABASE_URL"] = "postgres://u:p@127.0.0.1:1/db"
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	a, err := Build(context.Background(), cfg, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if a != nil {
		t.Cleanup(a.Close)
	}
	return a, err
}

func hit(h http.Handler, method, target, peer, cf, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = peer
	if cf != "" {
		req.Header.Set("CF-Connecting-IP", cf)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSignInOffAnswersTheOffPage(t *testing.T) {
	a, err := buildOffline(t, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	rec := hit(a.Handler, "GET", "/auth/login", "127.0.0.1:5000", "", "")
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "Sign-in is off") {
		t.Errorf("sign-in off: %d %.200s", rec.Code, rec.Body.String())
	}
	// Bearer tokens work as before, with sign-in off.
	if rec := hit(a.Handler, "GET", "/v1/me", "127.0.0.1:5000", "", "dev:prn_human01:human"); rec.Code != 200 {
		t.Errorf("bearer /v1/me: %d", rec.Code)
	}
}

func TestOneTrustListForEdgeGuardAndClientIP(t *testing.T) {
	// Unset SW_TRUSTED_PROXIES: this host's own addresses guard /mcp and key the /auth limit.
	a, err := buildOffline(t, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if rec := hit(a.Handler, "POST", "/mcp", "127.0.0.1:5000", "198.51.100.7", "dev:prn_agent01:agent"); rec.Code != 404 {
		t.Errorf("an edge request for /mcp: %d; want 404", rec.Code)
	}
	// Trusted peer: the limit keys on CF-Connecting-IP, so two visitors do not share a bucket.
	for i := 0; i < 20; i++ {
		hit(a.Handler, "GET", "/auth/login", "127.0.0.1:5000", "198.51.100.7", "")
	}
	if rec := hit(a.Handler, "GET", "/auth/login", "127.0.0.1:5000", "198.51.100.7", ""); rec.Code != 429 {
		t.Errorf("21st /auth call from one visitor: %d; want 429", rec.Code)
	}
	if rec := hit(a.Handler, "GET", "/auth/login", "127.0.0.1:5000", "198.51.100.8", ""); rec.Code == 429 {
		t.Error("another visitor shares the first one's bucket")
	}
}

func TestExplicitTrustedProxiesGovernBoth(t *testing.T) {
	a, err := buildOffline(t, map[string]string{"SW_TRUSTED_PROXIES": "192.0.2.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	if rec := hit(a.Handler, "POST", "/mcp", "192.0.2.9:5000", "198.51.100.7", "dev:prn_agent01:agent"); rec.Code != 404 {
		t.Errorf("edge request via the configured proxy: %d; want 404", rec.Code)
	}
	// Loopback is no longer trusted: its CF-Connecting-IP is ignored, so /mcp stays reachable.
	if rec := hit(a.Handler, "POST", "/mcp", "127.0.0.1:5000", "198.51.100.7", "dev:prn_agent01:agent"); rec.Code == 404 {
		t.Errorf("loopback was treated as the edge: %d", rec.Code)
	}
}

func TestSignInRefusesAnUnusableSessionSecret(t *testing.T) {
	dir := t.TempDir()
	short := filepath.Join(dir, "short")
	_ = os.WriteFile(short, []byte("tiny"), 0o600)
	for name, path := range map[string]string{"short": short, "missing": filepath.Join(dir, "nope")} {
		_, err := buildOffline(t, map[string]string{"SW_ALLOWED_PRINCIPALS": "prn_human01", "SW_APP_CLIENT_ID": "c",
			"SW_HUB_URL": "http://hub.example", "SW_SESSION_SECRET_FILE": path})
		if err == nil || !strings.Contains(err.Error(), "SW_SESSION_SECRET_FILE") {
			t.Errorf("%s secret: err = %v", name, err)
		}
	}
}

// Sign-out compares the browser's Origin with SW_PUBLIC_URL's normalised origin, as the gate
// does for session writes: a public URL written with capitals and the default port must not
// make every sign-out a 403.
func TestSignOutAcceptsTheNormalisedPublicOrigin(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret")
	_ = os.WriteFile(secret, []byte(strings.Repeat("s", 48)), 0o600)
	a, err := buildOffline(t, map[string]string{"SW_ALLOWED_PRINCIPALS": "prn_human01", "SW_APP_CLIENT_ID": "c",
		"SW_HUB_URL": "http://hub.example", "SW_SESSION_SECRET_FILE": secret,
		"SW_PUBLIC_URL": "https://App.Example.com:443"})
	if err != nil {
		t.Fatal(err)
	}
	logout := func(origin string) int {
		req := httptest.NewRequest("POST", "/auth/logout", nil)
		req.RemoteAddr = "127.0.0.1:5000"
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		a.Handler.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := logout("https://app.example.com"); code != http.StatusNoContent {
		t.Errorf("sign-out from https://app.example.com: %d; want 204", code)
	}
	for _, bad := range []string{"", "https://App.Example.com:443", "https://evil.example"} {
		if code := logout(bad); code != http.StatusForbidden {
			t.Errorf("sign-out with Origin %q: %d; want 403", bad, code)
		}
	}
}
