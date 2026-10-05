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
	if rec := send(h, "GET", "", "good", ""); rec.Code != 401 || seen || !strings.Contains(rec.Body.String(), "a hub-issued bearer token is required") {
		t.Errorf("cookie with sign-in off: %d %s", rec.Code, rec.Body)
	}
}
