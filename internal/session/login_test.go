package session

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/testutil"
)

type lookupMap map[string]auth.PrincipalRecord

func (m lookupMap) Lookup(_ context.Context, key string) (auth.PrincipalRecord, error) {
	if r, ok := m[key]; ok {
		return r, nil
	}
	return auth.PrincipalRecord{}, auth.ErrPrincipalNotFound
}

// The hub's browser tokens name the hub account (hubuser_…); the principal is looked up.
var people = lookupMap{
	"hubuser_01":    {ID: "prn_human01", Kind: auth.KindHuman},
	"hubuser_02":    {ID: "prn_human02", Kind: auth.KindHuman},
	"hubuser_gone":  {ID: "prn_human09", Kind: auth.KindHuman, Suspended: true},
	"prn_agent01":   {ID: "prn_agent01", Kind: auth.KindAgent},
	"prn_service01": {ID: "prn_service01", Kind: auth.KindService},
}

type rig struct {
	hub    *testutil.HubStub
	sw     *httptest.Server
	login  *Login
	store  *MemStore
	logs   *bytes.Buffer
	client *http.Client
	now    time.Time
}

func newRig(t *testing.T) *rig {
	t.Helper()
	var h http.Handler
	sw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.ServeHTTP(w, r) }))
	t.Cleanup(sw.Close)
	r := &rig{sw: sw, hub: testutil.NewHubStub(t, sw.URL), store: NewMemStore(), logs: &bytes.Buffer{}, now: time.Now()}
	key, err := DeriveLoginKey([]byte(strings.Repeat("k", MinSecretBytes)))
	if err != nil {
		t.Fatal(err)
	}
	r.login = &Login{HubURL: r.hub.URL, ClientID: "superwitness-console", PublicURL: sw.URL, Key: key,
		Tokens: auth.NewVerifier(r.hub.URL, sw.URL, nil), Principals: people,
		Sessions: &Manager{Store: r.store, Allowed: map[string]bool{"prn_human01": true}},
		Logger:   slog.New(slog.NewJSONHandler(r.logs, nil)), Now: func() time.Time { return r.now }}
	mux := http.NewServeMux()
	mux.Handle("/auth/", http.StripPrefix("/auth", r.login.Handler()))
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) { _, _ = io.WriteString(w, "app:"+req.URL.RequestURI()) })
	h = mux
	jar, _ := cookiejar.New(nil)
	r.client = &http.Client{Jar: jar}
	return r
}

// signIn follows the whole flow and returns the last answer.
func (r *rig) signIn(t *testing.T, next string) (int, string) {
	t.Helper()
	resp, err := r.client.Get(r.sw.URL + "/auth/login?next=" + url.QueryEscape(next))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (r *rig) sessionCookie() string {
	u, _ := url.Parse(r.sw.URL)
	for _, c := range r.client.Jar.Cookies(u) {
		if c.Name == "sw_session" {
			return c.Value
		}
	}
	return ""
}

// stepTo follows redirects by hand until the next hop is the callback, and returns its URL.
func (r *rig) callbackURL(t *testing.T) string {
	t.Helper()
	stop := *r.client
	stop.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := stop.Get(r.sw.URL + "/auth/login")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	resp, err = stop.Get(resp.Header.Get("Location")) // the hub's authorize
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.Header.Get("Location")
}

func (r *rig) get(t *testing.T, u string) (int, string) {
	t.Helper()
	resp, err := r.client.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestSignIn(t *testing.T) {
	r := newRig(t)
	r.hub.SignInAs("hubuser_01", "human", "human01@example.com")
	code, body := r.signIn(t, "/runs/superpipeline/brd_01/run_01?tab=logs")
	if code != 200 || body != "app:/runs/superpipeline/brd_01/run_01?tab=logs" {
		t.Fatalf("landed on %d %q", code, body)
	}
	p, err := r.login.Sessions.Resolve(context.Background(), r.sessionCookie())
	if err != nil || p.ID != "prn_human01" || p.Kind != auth.KindHuman || p.Email != "human01@example.com" {
		t.Errorf("session principal = %+v %v; want the resolved prn_human01", p, err)
	}
	if r.hub.SawOrigin.Load() {
		t.Error("the code exchange carried an Origin header")
	}
	if !strings.Contains(r.logs.String(), `"msg":"auth.signin","principal":"prn_human01"`) {
		t.Errorf("logs = %s", r.logs)
	}
	if strings.Contains(r.logs.String(), r.sessionCookie()) {
		t.Error("the session cookie was logged")
	}
}

func TestOpenRedirectThroughNext(t *testing.T) {
	r := newRig(t)
	for _, next := range []string{"//evil.example/x", "https://evil.example", "/\\evil.example", "javascript:alert(1)",
		"evil.example", ""} {
		if code, body := r.signIn(t, next); code != 200 || body != "app:/" {
			t.Errorf("next=%q landed on %d %q; want app:/", next, code, body)
		}
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"/":                             "/",
		"/runs?status=failed":           "/runs?status=failed",
		"/rubrics/press/1":              "/rubrics/press/1",
		"//evil.example":                "/",
		"/\\evil.example":               "/",
		"https://evil.example/":         "/",
		"/x\ny":                         "/",
		"x":                             "/",
		"/" + strings.Repeat("a", 2048): "/",
	} {
		if got := SafeNext(in); got != want {
			t.Errorf("SafeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStateMismatch(t *testing.T) {
	r := newRig(t)
	cb, _ := url.Parse(r.callbackURL(t))
	q := cb.Query()
	q.Set("state", "not-the-state")
	cb.RawQuery = q.Encode()
	if code, body := r.get(t, cb.String()); code != 400 || !strings.Contains(body, "Sign-in expired") {
		t.Errorf("wrong state: %d %s", code, body)
	}
	if r.store.Len() != 0 || !strings.Contains(r.logs.String(), `"reason":"state_mismatch"`) {
		t.Errorf("sessions %d, logs %s", r.store.Len(), r.logs)
	}
}

// A sign-in started in a second tab replaces the first tab's login cookie: the first tab's
// callback then fails cleanly, and nobody is signed in by it.
func TestSecondTabSupersedesTheFirst(t *testing.T) {
	r := newRig(t)
	first := r.callbackURL(t)
	second := r.callbackURL(t)
	if code, body := r.get(t, first); code != 400 || !strings.Contains(body, "Sign-in expired") || r.store.Len() != 0 {
		t.Errorf("the first tab's callback: %d %s", code, body)
	}
	if code, body := r.get(t, second); code != 400 {
		t.Errorf("the second tab's callback after the first used the cookie up: %d %s", code, body)
	}
}

func TestExpiredLoginCookie(t *testing.T) {
	r := newRig(t)
	cb := r.callbackURL(t)
	r.now = r.now.Add(LoginTTL + time.Second)
	if code, body := r.get(t, cb); code != 400 || !strings.Contains(body, "Sign-in expired") || r.store.Len() != 0 {
		t.Errorf("after 10 minutes: %d %s", code, body)
	}
}

// The login cookie is signed: one whose destination was rewritten is refused, so it cannot be
// used to send a fresh session somewhere else.
func TestTamperedOrMissingLoginCookie(t *testing.T) {
	r := newRig(t)
	cb := r.callbackURL(t)
	u, _ := url.Parse(r.sw.URL)
	for _, c := range r.client.Jar.Cookies(u) {
		if c.Name != "sw_login" {
			continue
		}
		body, sig, _ := strings.Cut(c.Value, ".")
		raw, _ := base64.RawURLEncoding.DecodeString(body)
		var lc loginCookie
		_ = json.Unmarshal(raw, &lc)
		lc.Next = "https://evil.example/"
		b, _ := json.Marshal(lc)
		r.client.Jar.SetCookies(u, []*http.Cookie{{Name: "sw_login", Value: base64.RawURLEncoding.EncodeToString(b) + "." + sig, Path: "/"}})
	}
	stop := *r.client
	stop.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := stop.Get(cb)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 || resp.Header.Get("Location") != "" || r.store.Len() != 0 {
		t.Errorf("a rewritten login cookie: %d to %q, %d sessions", resp.StatusCode, resp.Header.Get("Location"), r.store.Len())
	}
	fresh := newRig(t)
	if code, _ := fresh.get(t, fresh.sw.URL+"/auth/callback?code=x&state=y"); code != 400 {
		t.Errorf("no login cookie: %d", code)
	}
}

func TestOnlyListedPeopleSignIn(t *testing.T) {
	for name, who := range map[string][2]string{
		"a person not listed":  {"hubuser_02", "human"},
		"a suspended person":   {"hubuser_gone", "human"},
		"an agent":             {"prn_agent01", "agent"},
		"a service":            {"prn_service01", "service"},
		"nobody the hub knows": {"hubuser_none", "human"},
	} {
		r := newRig(t)
		// Everyone but prn_human02 is listed, so each refusal below is its own guard's doing.
		r.login.Sessions.Allowed = map[string]bool{"prn_human01": true, "prn_human09": true, "prn_agent01": true, "prn_service01": true}
		r.hub.SignInAs(who[0], who[1], "")
		if code, body := r.signIn(t, "/"); code != 403 || !strings.Contains(body, "Not authorised") {
			t.Errorf("%s: %d %s", name, code, body)
		}
		if r.store.Len() != 0 || !strings.Contains(r.logs.String(), `"msg":"auth.signin_refused"`) {
			t.Errorf("%s: sessions %d, logs %s", name, r.store.Len(), r.logs)
		}
	}
}

func TestHubDownAtSignIn(t *testing.T) {
	r := newRig(t)
	r.hub.Down.Store(true)
	if code, body := r.signIn(t, "/"); code != 503 || !strings.Contains(body, "AgentPod sign-in is unavailable") {
		t.Errorf("%d %s", code, body)
	}
}

func TestStoreDownAtSignIn(t *testing.T) {
	r := newRig(t)
	r.store.Fail = ErrUnavailable
	if code, body := r.signIn(t, "/"); code != 503 || !strings.Contains(body, "reach the database") {
		t.Errorf("%d %s", code, body)
	}
}

func TestLogout(t *testing.T) {
	r := newRig(t)
	r.signIn(t, "/")
	tok := r.sessionCookie()
	post := func(origin string) int {
		req, _ := http.NewRequest("POST", r.sw.URL+"/auth/logout", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := r.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := post("https://evil.example"); code != 403 || r.store.Len() != 1 {
		t.Errorf("cross-site sign-out: %d, sessions %d", code, r.store.Len())
	}
	if code := post(r.sw.URL); code != 204 || r.store.Len() != 0 || r.sessionCookie() != "" {
		t.Errorf("sign-out: %d, sessions %d, cookie %q", code, r.store.Len(), r.sessionCookie())
	}
	if _, err := r.login.Sessions.Resolve(context.Background(), tok); err == nil {
		t.Error("the old session still resolves")
	}
	if !strings.Contains(r.logs.String(), `"msg":"auth.signout","principal":"prn_human01"`) {
		t.Errorf("logs = %s", r.logs)
	}
}

func TestDeriveLoginKeyNeedsEnoughSecret(t *testing.T) {
	if _, err := DeriveLoginKey([]byte(strings.Repeat("k", MinSecretBytes-1))); err == nil {
		t.Error("31 bytes accepted")
	}
}

func TestCallbackWithoutCodeOrCookieIsLoggedWithItsReason(t *testing.T) {
	r := newRig(t)
	cb, _ := url.Parse(r.callbackURL(t))
	q := cb.Query()
	q.Del("code")
	cb.RawQuery = q.Encode()
	if code, body := r.get(t, cb.String()); code != 400 || !strings.Contains(body, "Sign-in failed") {
		t.Errorf("no code: %d %s", code, body)
	}
	fresh := newRig(t)
	fresh.get(t, fresh.sw.URL+"/auth/callback?code=x&state=y")
	for name, rg := range map[string]*rig{"no_code": r, "login_missing": fresh} {
		if !strings.Contains(rg.logs.String(), `"reason":"`+name+`"`) {
			t.Errorf("%s not logged: %s", name, rg.logs)
		}
	}
}
