package app

import (
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/config"
	"github.com/SuperJackfruitLabs/superwitness/internal/session"
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
		q.Get("resource") != "http://127.0.0.1:8790" || q.Get("redirect_uri") != "http://127.0.0.1:8790/auth/callback" ||
		q.Get("scope") != "openid offline_access" {
		t.Errorf("plane mode sent the browser to %s", u)
	}
}

// Under the plane each session is bound to the plane's refresh-token grant; under the hub, never.
func TestOnlyPlaneSessionsHoldAGrant(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret")
	_ = os.WriteFile(secret, []byte(strings.Repeat("s", 48)), 0o600)
	cfg := config.Config{AllowedPrincipals: []string{"prn_human01"}, AppClientID: "superwitness-web",
		SessionSecretFile: secret, PublicURL: "https://witness.example", HubURL: "https://hub.example"}
	logger := slog.New(slog.DiscardHandler)
	m, err := signIn(cfg, &api.Server{}, wiring{}, nil, nil, http.DefaultClient, nil, logger)
	if err != nil || m == nil || m.Grants != nil {
		t.Fatalf("hub mode: %v, grants %v", err, m)
	}
	cfg.OrgPlaneIssuer, cfg.OrgPlaneURL = "https://accounts.example", "https://accounts.example"
	cfg.OrgPlaneJWKSURL = "https://accounts.example/api/auth/jwks"
	m, err = signIn(cfg, &api.Server{}, wiring{}, nil, nil, http.DefaultClient, nil, logger)
	if err != nil || m == nil {
		t.Fatal(err)
	}
	g, ok := m.Grants.(*session.PlaneGrants)
	if !ok || g.ClientID != "superwitness-web" || g.Endpoints.URL != "https://accounts.example" ||
		g.Endpoints.Resource != "https://witness.example" || g.Tokens == nil {
		t.Errorf("plane mode grants = %+v", m.Grants)
	}
}
