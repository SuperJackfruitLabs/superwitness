package app

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignInGoesThroughTheIssuerInUse(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret")
	_ = os.WriteFile(secret, []byte(strings.Repeat("s", 48)), 0o600)
	base := func() map[string]string {
		return map[string]string{"SW_ALLOWED_PRINCIPALS": "prn_human01", "SW_APP_CLIENT_ID": "superwitness-web",
			"SW_SESSION_SECRET_FILE": secret}
	}
	authorize := func(env map[string]string) *url.URL {
		t.Helper()
		a, err := buildOffline(t, env)
		if err != nil {
			t.Fatal(err)
		}
		rec := hit(a.Handler, "GET", "/auth/login", "127.0.0.1:5000", "", "")
		u, err := url.Parse(rec.Header().Get("Location"))
		if rec.Code != 302 || err != nil {
			t.Fatalf("/auth/login: %d %q", rec.Code, rec.Header().Get("Location"))
		}
		return u
	}

	hub := base()
	hub["SW_HUB_URL"] = "http://hub.example"
	if u := authorize(hub); u.Host != "hub.example" || u.Path != "/api/auth/authorize" || u.Query().Get("client") != "superwitness-web" {
		t.Errorf("hub mode sent the browser to %s", u)
	}

	plane := base()
	plane["SW_ORG_PLANE_ISSUER"] = "https://accounts.example"
	plane["SW_ORG_PLANE_JWKS_URL"] = "https://accounts.example/api/auth/jwks"
	plane["SW_ORG_PLANE_URL"] = "https://accounts.example"
	u := authorize(plane)
	q := u.Query()
	if u.Host != "accounts.example" || u.Path != "/api/auth/oauth2/authorize" || q.Get("client_id") != "superwitness-web" ||
		q.Get("resource") != "http://127.0.0.1:8790" || q.Get("redirect_uri") != "http://127.0.0.1:8790/auth/callback" {
		t.Errorf("plane mode sent the browser to %s", u)
	}
}
