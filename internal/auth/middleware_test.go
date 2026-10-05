package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddleware(t *testing.T) {
	var seen Principal
	h := Middleware(DevAuthenticator{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = PrincipalFrom(r.Context())
		w.WriteHeader(204)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/v1/x", nil))
	if rec.Code != 401 || rec.Header().Get("WWW-Authenticate") == "" || !strings.Contains(rec.Body.String(), `"unauthenticated"`) {
		t.Errorf("no token: %d %q", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest("GET", "/v1/x", nil)
	req.Header.Set("Authorization", "Bearer dev:prn_canary:service")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 204 || seen.Kind != KindService {
		t.Errorf("service (the canary) must be admitted: %d %+v", rec.Code, seen)
	}

	req = httptest.NewRequest("GET", "/v1/x", nil)
	req.Header.Set("Authorization", "Bearer garbage")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("garbage token: %d", rec.Code)
	}

	req = httptest.NewRequest("GET", "/v1/x", nil)
	req.Header.Set("Authorization", "Bearer dev:prn_agent01:agent")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 204 || seen.ID != "prn_agent01" || seen.Kind != KindAgent {
		t.Errorf("agent: %d %+v", rec.Code, seen)
	}
}

func TestDevAuthenticatorRejectsMalformed(t *testing.T) {
	for _, tok := range []string{"", "dev", "dev::agent", "dev:prn_a:robot", "prod:prn_a:agent", "dev:prn_a:agent:x"} {
		if _, err := (DevAuthenticator{}).Verify(t.Context(), tok); err == nil {
			t.Errorf("%q accepted", tok)
		}
	}
}
