package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sessions map[string]Principal

func (s sessions) Resolve(_ context.Context, tok string) (Principal, error) {
	switch tok {
	case "gone":
		return Principal{}, ErrSessionNotAllowed
	case "down":
		return Principal{}, ErrSessionUnavailable
	}
	p, ok := s[tok]
	if !ok {
		return Principal{}, ErrSessionInvalid
	}
	return p, nil
}

const origin = "https://superwitness.example"

func gateServer(t *testing.T) (http.Handler, *Principal, *bool) {
	t.Helper()
	var seen Principal
	var via bool
	g := Gate{Bearer: DevAuthenticator{}, Sessions: sessions{"good": {ID: "prn_human01", Kind: KindHuman}},
		Cookie: "sw_session", Origin: origin}
	return g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = PrincipalFrom(r.Context())
		via = ViaSession(r.Context())
		w.WriteHeader(204)
	})), &seen, &via
}

func send(h http.Handler, method, bearer, cookie, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/v1/x", strings.NewReader("{}"))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "sw_session", Value: cookie})
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGateSession(t *testing.T) {
	h, seen, via := gateServer(t)
	if rec := send(h, "GET", "", "good", ""); rec.Code != 204 || seen.ID != "prn_human01" || !*via {
		t.Errorf("session read: %d %+v via=%v", rec.Code, *seen, *via)
	}
	for name, c := range map[string]struct {
		cookie string
		code   int
		body   string
	}{
		"no cookie":        {"", 401, "unauthenticated"},
		"unknown session":  {"nope", 401, "unauthenticated"},
		"no longer listed": {"gone", 403, "not_authorised"},
		"store down":       {"down", 503, "store_unavailable"},
	} {
		if rec := send(h, "GET", "", c.cookie, ""); rec.Code != c.code || !strings.Contains(rec.Body.String(), c.body) {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
}

func TestGateBearerWinsOverCookie(t *testing.T) {
	h, seen, via := gateServer(t)
	if rec := send(h, "POST", "dev:prn_agent01:agent", "good", ""); rec.Code != 204 || seen.ID != "prn_agent01" || *via {
		t.Errorf("bearer and cookie: %d %+v via=%v; want the bearer's principal", rec.Code, *seen, *via)
	}
	if rec := send(h, "GET", "garbage", "good", ""); rec.Code != 401 {
		t.Errorf("a bad bearer beside a good cookie: %d; the cookie must not rescue it", rec.Code)
	}
}

func TestGateOriginOnSessionWrites(t *testing.T) {
	h, _, _ := gateServer(t)
	for name, c := range map[string]struct {
		method, bearer, cookie, origin string
		code                           int
	}{
		"session write, right origin":  {"POST", "", "good", origin, 204},
		"session write, no origin":     {"POST", "", "good", "", 403},
		"session write, other origin":  {"POST", "", "good", "https://evil.example", 403},
		"session write, origin prefix": {"POST", "", "good", origin + ".evil.example", 403},
		"session read, no origin":      {"GET", "", "good", "", 204},
		"bearer write, no origin":      {"POST", "dev:prn_human01:human", "", "", 204},
	} {
		rec := send(h, c.method, c.bearer, c.cookie, c.origin)
		if rec.Code != c.code || (c.code == 403 && !strings.Contains(rec.Body.String(), "origin_mismatch")) {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
}

func TestGateWithoutSessionsIsTheOldMiddleware(t *testing.T) {
	var seen bool
	h := Gate{Bearer: DevAuthenticator{}}.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = true
	}))
	if rec := send(h, "GET", "", "good", ""); rec.Code != 401 || seen || !strings.Contains(rec.Body.String(), "a bearer token is required") {
		t.Errorf("cookie with sign-in off: %d %s", rec.Code, rec.Body)
	}
}

func TestPublicOrigin(t *testing.T) {
	for in, want := range map[string]string{
		"https://superwitness.example":            "https://superwitness.example",
		"https://superwitness.example/":           "https://superwitness.example",
		"HTTPS://SuperWitness.Example/app/?q=1#f": "https://superwitness.example",
		"https://superwitness.example:443":        "https://superwitness.example",
		"http://127.0.0.1:8080":                   "http://127.0.0.1:8080",
		"http://127.0.0.1:80/":                    "http://127.0.0.1",
		"https://superwitness.example:8443":       "https://superwitness.example:8443",
		"http://[2001:DB8::1]:8080":               "http://[2001:db8::1]:8080",
	} {
		if got, err := PublicOrigin(in); err != nil || got != want {
			t.Errorf("PublicOrigin(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "superwitness.example", "ftp://superwitness.example", "https://", "https://user@superwitness.example", "://x"} {
		if got, err := PublicOrigin(bad); err == nil {
			t.Errorf("PublicOrigin(%q) = %q; want an error", bad, got)
		}
	}
}

func TestNewGateRefusesSessionsWithoutAnOrigin(t *testing.T) {
	ss := sessions{"good": {ID: "prn_human01", Kind: KindHuman}}
	if _, err := NewGate(DevAuthenticator{}, ss, "sw_session", ""); err == nil {
		t.Error("sessions with no public URL: no error")
	}
	if _, err := NewGate(DevAuthenticator{}, ss, "sw_session", "not a url"); err == nil {
		t.Error("sessions with a bad public URL: no error")
	}
	g, err := NewGate(DevAuthenticator{}, ss, "sw_session", "HTTPS://SuperWitness.Example/")
	if err != nil || g.Origin != origin {
		t.Errorf("NewGate origin = %q, %v; want %q", g.Origin, err, origin)
	}
	if g, err := NewGate(DevAuthenticator{}, nil, "", ""); err != nil || g.Sessions != nil {
		t.Errorf("bearer-only gate: %+v %v", g, err)
	}
}

func TestGateWithAnEmptyOriginRefusesEverySessionWrite(t *testing.T) {
	// A Gate built by hand with no Origin must not let a session write through just because the
	// request carries no Origin header either.
	g := Gate{Bearer: DevAuthenticator{}, Sessions: sessions{"good": {ID: "prn_human01", Kind: KindHuman}}, Cookie: "sw_session"}
	h := g.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	if rec := send(h, "POST", "", "good", ""); rec.Code != 403 || !strings.Contains(rec.Body.String(), "origin_mismatch") {
		t.Errorf("session write, empty Origin on both sides: %d %s", rec.Code, rec.Body.String())
	}
	if rec := send(h, "GET", "", "good", ""); rec.Code != 204 {
		t.Errorf("session read: %d", rec.Code)
	}
}
