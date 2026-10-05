package session

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCookieAttributes(t *testing.T) {
	rec := httptest.NewRecorder()
	c := Cookies{Secure: true}
	c.Set(rec, c.LoginName(), "v", LoginTTL)
	got := rec.Header().Get("Set-Cookie")
	for _, want := range []string{"__Host-sw_login=v", "Path=/", "Max-Age=600", "HttpOnly", "Secure", "SameSite=Lax"} {
		if !strings.Contains(got, want) {
			t.Errorf("Set-Cookie %q lacks %s", got, want)
		}
	}
	if strings.Contains(got, "Domain=") {
		t.Errorf("a __Host- cookie must have no Domain: %q", got)
	}
	if c.SessionName() != "__Host-sw_session" || (Cookies{}).SessionName() != "sw_session" {
		t.Error("cookie names")
	}
}
