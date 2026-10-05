package session

import (
	"bytes"
	"context"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
)

// MinSecretBytes is the least SW_SESSION_SECRET_FILE may hold.
const MinSecretBytes = 32

// DeriveLoginKey turns the session secret into the login cookie's HMAC key.
func DeriveLoginKey(secret []byte) ([]byte, error) {
	if len(secret) < MinSecretBytes {
		return nil, fmt.Errorf("the session secret must be at least %d bytes, got %d", MinSecretBytes, len(secret))
	}
	return hkdf.Key(sha256.New, secret, nil, "superwitness login cookie v1", 32)
}

// Login is the browser sign-in: GET /auth/login, GET /auth/callback and POST /auth/logout,
// against the hub's authorize and token-exchange routes, with PKCE (S256).
type Login struct {
	HubURL     string // SW_HUB_URL, the token issuer
	ClientID   string // SW_APP_CLIENT_ID
	PublicURL  string // SW_PUBLIC_URL; the callback is {PublicURL}/auth/callback
	Key        []byte // DeriveLoginKey(secret)
	Tokens     auth.Authenticator
	Principals auth.PrincipalLookup
	Sessions   *Manager
	Cookies    Cookies
	HTTP       *http.Client
	Logger     *slog.Logger
	Now        func() time.Time
}

func (l *Login) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func (l *Login) log() *slog.Logger {
	if l.Logger != nil {
		return l.Logger
	}
	return slog.New(slog.DiscardHandler)
}

// Handler serves the three routes, to be mounted at /auth.
func (l *Login) Handler() http.Handler {
	r := chi.NewRouter()
	r.Get("/login", l.login)
	r.Get("/callback", l.callback)
	r.Post("/logout", l.logout)
	return r
}

func (l *Login) redirectURI() string { return l.PublicURL + "/auth/callback" }

// loginCookie is what the browser carries to the hub and back: the state the callback must
// echo, the PKCE verifier and where to go after. It is signed, not encrypted: it holds nothing
// the browser does not already see in the URL except the verifier, which is useless without
// the code.
type loginCookie struct {
	State    string `json:"s"`
	Verifier string `json:"v"`
	Next     string `json:"n"`
	Expires  int64  `json:"e"`
}

func mac(key []byte, body string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(body))
	return h.Sum(nil)
}

func (c loginCookie) seal(key []byte) string {
	b, _ := json.Marshal(c)
	body := base64.RawURLEncoding.EncodeToString(b)
	return body + "." + base64.RawURLEncoding.EncodeToString(mac(key, body))
}

var (
	errLoginInvalid = errors.New("login cookie invalid")
	errLoginExpired = errors.New("login cookie expired")
)

func openLoginCookie(v string, key []byte, now time.Time) (loginCookie, error) {
	body, sig, ok := strings.Cut(v, ".")
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if !ok || err != nil || !hmac.Equal(got, mac(key, body)) {
		return loginCookie{}, errLoginInvalid
	}
	b, err := base64.RawURLEncoding.DecodeString(body)
	var c loginCookie
	if err != nil || json.Unmarshal(b, &c) != nil || c.State == "" || c.Verifier == "" {
		return loginCookie{}, errLoginInvalid
	}
	if now.Unix() >= c.Expires {
		return loginCookie{}, errLoginExpired
	}
	return c, nil
}

func randomToken() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// Challenge is PKCE's S256: base64url(sha256(verifier)).
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// SafeNext keeps a post-sign-in destination on this origin: a path starting with one "/", with
// no backslash or control character. Anything else becomes "/".
func SafeNext(next string) string {
	if next == "" || next[0] != '/' || strings.HasPrefix(next, "//") || len(next) > 2048 ||
		strings.ContainsFunc(next, func(r rune) bool { return r == '\\' || r < 0x20 || r == 0x7f }) {
		return "/"
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil {
		return "/"
	}
	return next
}

func (l *Login) login(w http.ResponseWriter, r *http.Request) {
	lc := loginCookie{State: randomToken(), Verifier: randomToken(), Next: SafeNext(r.URL.Query().Get("next")),
		Expires: l.now().Add(LoginTTL).Unix()}
	l.Cookies.Set(w, l.Cookies.LoginName(), lc.seal(l.Key), LoginTTL)
	q := url.Values{}
	q.Set("client", l.ClientID)
	q.Set("redirect_uri", l.redirectURI())
	q.Set("state", lc.State)
	q.Set("code_challenge", Challenge(lc.Verifier))
	q.Set("code_challenge_method", "S256")
	http.Redirect(w, r, l.HubURL+"/api/auth/authorize?"+q.Encode(), http.StatusFound)
}

func (l *Login) refuse(w http.ResponseWriter, status int, reason string, p pageData, attrs ...any) {
	l.log().Warn("auth.signin_refused", append([]any{"reason", reason}, attrs...)...)
	render(w, status, p)
}

func (l *Login) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ck, cerr := r.Cookie(l.Cookies.LoginName())
	l.Cookies.Clear(w, l.Cookies.LoginName()) // one use, whatever happens next
	if cerr != nil {
		l.refuse(w, http.StatusBadRequest, "login_missing", pageExpired)
		return
	}
	lc, err := openLoginCookie(ck.Value, l.Key, l.now())
	switch {
	case errors.Is(err, errLoginExpired):
		l.refuse(w, http.StatusBadRequest, "login_expired", pageExpired)
		return
	case err != nil:
		l.refuse(w, http.StatusBadRequest, "login_invalid", pageExpired)
		return
	}
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(lc.State)) != 1 {
		l.refuse(w, http.StatusBadRequest, "state_mismatch", pageExpired)
		return
	}
	code := q.Get("code")
	if code == "" {
		l.refuse(w, http.StatusBadRequest, "no_code", pageFailed)
		return
	}
	tok, err := l.exchange(r.Context(), code, lc.Verifier)
	if errors.Is(err, errHubUnavailable) {
		l.refuse(w, http.StatusServiceUnavailable, "hub_unavailable", pageHubDown)
		return
	}
	if err != nil {
		l.refuse(w, http.StatusBadRequest, "exchange_refused", pageFailed)
		return
	}
	p, err := l.Tokens.Verify(r.Context(), tok)
	if err != nil {
		l.refuse(w, http.StatusBadRequest, "token_invalid", pageFailed)
		return
	}
	// The exchanged token's sub is the hub account, not always the principal id: resolve it.
	rec, err := l.Principals.Lookup(r.Context(), p.ID)
	switch {
	case errors.Is(err, auth.ErrPrincipalNotFound):
		l.refuse(w, http.StatusForbidden, "no_principal", pageNotAuthorised)
		return
	case err != nil:
		l.refuse(w, http.StatusServiceUnavailable, "hub_unavailable", pageHubDown)
		return
	}
	if rec.Suspended {
		l.refuse(w, http.StatusForbidden, "suspended", pageNotAuthorised, "principal", rec.ID)
		return
	}
	if p.Kind != auth.KindHuman || rec.Kind != auth.KindHuman { // the token's claim and the record must both say human
		l.refuse(w, http.StatusForbidden, "not_human", pageNotAuthorised, "principal", rec.ID)
		return
	}
	p.ID = rec.ID
	if !l.Sessions.Allowed[p.ID] {
		l.refuse(w, http.StatusForbidden, "not_allowlisted", pageNotAuthorised, "principal", p.ID)
		return
	}
	token, err := l.Sessions.Issue(r.Context(), p)
	if err != nil {
		l.refuse(w, http.StatusServiceUnavailable, "store_unavailable", pageDatabase, "principal", p.ID)
		return
	}
	l.Cookies.Set(w, l.Cookies.SessionName(), token, AbsoluteTTL)
	l.log().Info("auth.signin", "principal", p.ID)
	http.Redirect(w, r, lc.Next, http.StatusSeeOther)
}

var (
	errHubUnavailable  = errors.New("hub unavailable")
	errExchangeRefused = errors.New("hub refused the code")
)

// exchange trades the code for a token, server to server. It sends no Origin header: the hub
// refuses an exchange that carries one, because only a page would.
func (l *Login) exchange(ctx context.Context, code, verifier string) (string, error) {
	body, _ := json.Marshal(map[string]string{"code": code, "code_verifier": verifier, "redirect_uri": l.redirectURI()})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.HubURL+"/api/auth/token/exchange", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	hc := l.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", errHubUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return "", fmt.Errorf("%w: HTTP %d", errHubUnavailable, resp.StatusCode)
	}
	var out struct {
		Token string `json:"token"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out) != nil || out.Token == "" {
		return "", errExchangeRefused
	}
	return out.Token, nil
}

func (l *Login) logout(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != l.PublicURL {
		auth.WriteError(w, http.StatusForbidden, "origin_mismatch", "sign out from the app")
		return
	}
	if c, err := r.Cookie(l.Cookies.SessionName()); err == nil {
		s, err := l.Sessions.Revoke(r.Context(), c.Value)
		switch {
		case err == nil:
			l.log().Info("auth.signout", "principal", s.Principal)
		case errors.Is(err, ErrNotFound): // already gone, or never a session: signed out either way
		default:
			// The server-side session may still be valid, so do not say 204; the cookie goes anyway.
			l.Cookies.Clear(w, l.Cookies.SessionName())
			attrs := []any{}
			if s.Principal != "" { // known only when the store answered the read and failed the delete
				attrs = append(attrs, "principal", s.Principal)
			}
			l.log().Warn("auth.signout_failed", attrs...)
			auth.WriteError(w, http.StatusServiceUnavailable, "store_unavailable", "could not end the session; try again")
			return
		}
	}
	l.Cookies.Clear(w, l.Cookies.SessionName())
	w.WriteHeader(http.StatusNoContent)
}
