package session

import (
	"net/http"
	"time"
)

// LoginTTL is how long a login cookie lives.
const LoginTTL = 10 * time.Minute

// Cookies names and writes superwitness's two cookies. Secure is true when SW_PUBLIC_URL is
// https: the names then carry the __Host- prefix, which a browser accepts only with Secure,
// Path=/ and no Domain. Only loopback fake mode runs without it.
type Cookies struct{ Secure bool }

func (c Cookies) name(base string) string {
	if c.Secure {
		return "__Host-" + base
	}
	return base
}

func (c Cookies) SessionName() string { return c.name("sw_session") }
func (c Cookies) LoginName() string   { return c.name("sw_login") }

func (c Cookies) Set(w http.ResponseWriter, name, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: int(maxAge / time.Second),
		HttpOnly: true, Secure: c.Secure, SameSite: http.SameSiteLaxMode})
}

func (c Cookies) Clear(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: c.Secure, SameSite: http.SameSiteLaxMode})
}
