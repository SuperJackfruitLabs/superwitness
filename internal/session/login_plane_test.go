package session

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/testutil"
)

type noLookups struct{ t *testing.T }

func (n noLookups) Lookup(context.Context, string) (auth.PrincipalRecord, error) {
	n.t.Error("a principal lookup was made under the plane")
	return auth.PrincipalRecord{}, auth.ErrLookupUnavailable
}

func newPlaneRig(t *testing.T) (*rig, *testutil.PlaneStub) {
	t.Helper()
	var h http.Handler
	sw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }))
	t.Cleanup(sw.Close)
	plane := testutil.NewPlaneStub(t, sw.URL, "superwitness-web")
	r := &rig{sw: sw, store: NewMemStore(), logs: &bytes.Buffer{}, now: time.Now()}
	key, err := DeriveLoginKey([]byte(strings.Repeat("k", MinSecretBytes)))
	if err != nil {
		t.Fatal(err)
	}
	endpoints := PlaneEndpoints{URL: plane.URL, Resource: sw.URL}
	tokens := auth.NewPlaneVerifier(plane.URL, plane.URL+"/api/auth/jwks", sw.URL, nil)
	logger := slog.New(slog.NewJSONHandler(r.logs, nil))
	now := func() time.Time { return r.now }
	r.login = &Login{Endpoints: endpoints, ClientID: "superwitness-web",
		PublicURL: sw.URL, Origin: sw.URL, Key: key, Provider: "accounts.example", SubIsPrincipal: true,
		Tokens: tokens, Principals: noLookups{t},
		Sessions: &Manager{Store: r.store, Allowed: map[string]bool{"prn_human01": true, "prn_agent01": true}, Now: now,
			Logger: logger, Grants: &PlaneGrants{Endpoints: endpoints, ClientID: "superwitness-web", Tokens: tokens}},
		Logger: logger, Now: now}
	mux := http.NewServeMux()
	mux.Handle("/auth/", http.StripPrefix("/auth", r.login.Handler()))
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) { _, _ = io.WriteString(w, "app:"+req.URL.RequestURI()) })
	h = mux
	jar, _ := cookiejar.New(nil)
	r.client = &http.Client{Jar: jar}
	return r, plane
}

func TestPlaneSignIn(t *testing.T) {
	r, plane := newPlaneRig(t)
	plane.SignInAs("prn_human01", "human", "human01@example.com")
	if code, body := r.signIn(t, "/runs"); code != 200 || body != "app:/runs" {
		t.Fatalf("landed on %d %q", code, body)
	}
	p, err := r.login.Sessions.Resolve(context.Background(), r.sessionCookie())
	if err != nil || p.ID != "prn_human01" || p.Kind != auth.KindHuman || p.Tenant != "org_01" || p.Email != "human01@example.com" {
		t.Errorf("session principal = %+v %v", p, err)
	}
	a, tk := plane.LastAuthorize, plane.LastToken
	if a.Get("client_id") != "superwitness-web" || a.Get("resource") != r.sw.URL || a.Get("scope") != "openid offline_access" ||
		a.Get("redirect_uri") != r.sw.URL+"/auth/callback" {
		t.Errorf("authorize query = %v", a)
	}
	if tk.Get("resource") != r.sw.URL || tk.Get("client_id") != "superwitness-web" || tk.Get("client_secret") != "" {
		t.Errorf("token form = %v; want resource and the public client id, no secret", tk)
	}
	if plane.SawOrigin.Load() {
		t.Error("the code exchange carried an Origin header")
	}
	for _, rt := range plane.LiveRefreshTokens() {
		if strings.Contains(r.logs.String(), rt) {
			t.Error("the refresh token reached the logs")
		}
	}
}

func TestPlaneSignInRefusesNonHumans(t *testing.T) {
	r, plane := newPlaneRig(t)
	plane.SignInAs("prn_agent01", "agent", "")
	if code, _ := r.signIn(t, "/"); code != 403 || r.store.Len() != 0 || !strings.Contains(r.logs.String(), `"reason":"not_human"`) {
		t.Errorf("%d, sessions %d, logs %s", code, r.store.Len(), r.logs)
	}
}

func TestPlaneSignInRefusesWhenProductNotEnabled(t *testing.T) {
	r, plane := newPlaneRig(t)
	plane.SetEnt([]string{"superpipeline"})
	code, body := r.signIn(t, "/")
	if code != 403 || !strings.Contains(body, "Not authorised") || r.store.Len() != 0 {
		t.Errorf("%d %q", code, body)
	}
	if !strings.Contains(r.logs.String(), `"reason":"product_not_enabled"`) || !strings.Contains(r.logs.String(), `"org":"org_01"`) {
		t.Errorf("logs = %s", r.logs)
	}
}

func TestPlaneSignInOnlyListedPeople(t *testing.T) {
	r, plane := newPlaneRig(t)
	plane.SignInAs("prn_human02", "human", "")
	if code, _ := r.signIn(t, "/"); code != 403 || r.store.Len() != 0 {
		t.Errorf("%d, sessions %d", code, r.store.Len())
	}
}

func TestPlaneDownAtSignIn(t *testing.T) {
	r, plane := newPlaneRig(t)
	plane.Down.Store(true)
	code, body := r.signIn(t, "/")
	if code != 503 || !strings.Contains(body, "accounts.example sign-in is unavailable") {
		t.Errorf("%d %s", code, body)
	}
	if !strings.Contains(r.logs.String(), `"reason":"issuer_unavailable"`) {
		t.Errorf("logs = %s", r.logs)
	}
}

func TestPlaneTokenForAnotherAudienceIsRefused(t *testing.T) {
	r, plane := newPlaneRig(t)
	plane.AudienceOverride = "https://hub.example"
	if code, _ := r.signIn(t, "/"); code != 400 || r.store.Len() != 0 || !strings.Contains(r.logs.String(), `"reason":"token_invalid"`) {
		t.Errorf("%d, logs %s", code, r.logs)
	}
}
