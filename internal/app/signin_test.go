//go:build integration

package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/config"
	"github.com/SuperJackfruitLabs/superwitness/internal/testutil"
)

// The built binary in fake mode, signing in through a stand-in hub: sign in, read, judge with
// the session, revise with a bearer token for the same person, sign out.
func TestSignInEndToEnd(t *testing.T) {
	roles := testutil.StartPostgresWithRoles(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	public := "http://" + ln.Addr().String()
	hub := testutil.NewHubStub(t, public)
	secret := filepath.Join(t.TempDir(), "session-secret")
	if err := os.WriteFile(secret, []byte(strings.Repeat("s", 48)), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(envOf(map[string]string{"SW_FAKE_SOURCES": "1", "SW_LISTEN": ln.Addr().String(),
		"SW_PUBLIC_URL": public, "SW_HUB_URL": hub.URL, "SW_APP_CLIENT_ID": "superwitness-console",
		"SW_ALLOWED_PRINCIPALS": "prn_human01", "SW_SESSION_SECRET_FILE": secret,
		"SW_DATABASE_URL": roles.AppDSN, "SW_MIGRATE_DATABASE_URL": roles.OwnerDSN}))
	if err != nil {
		t.Fatal(err)
	}
	a, err := Build(context.Background(), cfg, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	srv := httptest.NewUnstartedServer(a.Handler)
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	do := func(c *http.Client, method, path, bearer, origin, body string) (int, string) {
		req, _ := http.NewRequest(method, public+path, strings.NewReader(body))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, b := do(http.DefaultClient, "GET", "/health", "", "", ""); strings.Contains(b, `"verdicts":"ok"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never migrated")
		}
		time.Sleep(100 * time.Millisecond)
	}
	roles.GrantRuntime(t)

	// Someone the hub knows but the allowlist does not.
	hub.SignInAs("hubuser_7f3a", "human", "")
	if code, body := do(browser, "GET", "/auth/login?next=/rubrics", "", "", ""); code != 403 || !strings.Contains(body, "Not authorised") {
		t.Fatalf("an unlisted person: %d %s", code, body)
	}

	hub.SignInAs("hubuser_01", "human", "human01@example.com")
	if code, body := do(browser, "GET", "/auth/login?next=/rubrics", "", "", ""); code != 200 || !strings.Contains(body, `<div id="root">`) {
		t.Fatalf("sign-in did not land on the app: %d %.200s", code, body)
	}
	if code, body := do(browser, "GET", "/v1/me", "", "", ""); code != 200 || !strings.Contains(body, `"principal":"prn_human01"`) || !strings.Contains(body, `"via":"session"`) {
		t.Fatalf("me: %d %s", code, body)
	}
	verdict := `{"idempotency_key":"k-signin","subject_kind":"run","subject_ref":"superpipeline:brd_01/run_01","standard":"stage:review","value":{"decision":"pass"}}`
	if code, body := do(browser, "POST", "/v1/verdicts", "", "", verdict); code != 403 || !strings.Contains(body, "origin_mismatch") {
		t.Errorf("a session write without Origin: %d %s", code, body)
	}
	code, body := do(browser, "POST", "/v1/verdicts", "", public, verdict)
	var v struct{ ID, Judge string }
	_ = json.Unmarshal([]byte(body), &v)
	if code != 201 || v.Judge != "prn_human01" {
		t.Fatalf("a session write: %d %s", code, body)
	}
	// The same person with a bearer token whose sub is their hub account, as the hub issues it:
	// resolved to prn_human01, so they may revise their own verdict.
	revise := `{"idempotency_key":"k-signin-2","subject_kind":"run","subject_ref":"superpipeline:brd_01/run_01","standard":"stage:review","value":{"decision":"fail"},"supersedes":"` + v.ID + `"}`
	if code, body := do(http.DefaultClient, "POST", "/v1/verdicts", "dev:hubuser_01:human", "", revise); code != 201 || !strings.Contains(body, `"judge":"prn_human01"`) {
		t.Errorf("revise with a bearer token for the same person: %d %s", code, body)
	}
	// With the hub gone, the open session still reads: it needs neither the hub nor its keys.
	hub.Stop()
	if code, body := do(browser, "GET", "/v1/runs", "", "", ""); code != 200 {
		t.Errorf("a session read with the hub down: %d %s", code, body)
	}
	if code, _ := do(browser, "POST", "/auth/logout", "", public, ""); code != 204 {
		t.Errorf("sign out: %d", code)
	}
	if code, _ := do(browser, "GET", "/v1/me", "", "", ""); code != 401 {
		t.Errorf("after sign-out: %d", code)
	}
}
