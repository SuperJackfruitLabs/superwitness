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
// against an issuer's authorize and token routes (the hub's or the organization plane's), with
// PKCE (S256).
type Login struct {
	Endpoints      Endpoints // the issuer's authorize and token routes: HubEndpoints or PlaneEndpoints
	HubURL         string    // SW_HUB_URL: used as HubEndpoints when Endpoints is nil
	SubIsPrincipal bool      // the issuer's sub is always a prn_ id (the organization plane): no lookup
	Provider       string    // what the pages call the sign-in service; empty means "AgentPod"
	ClientID       string    // SW_APP_CLIENT_ID
	PublicURL      string    // SW_PUBLIC_URL; the callback is {PublicURL}/auth/callback
	Origin         string    // auth.PublicOrigin(SW_PUBLIC_URL): what a sign-out's Origin header must equal
	Key            []byte    // DeriveLoginKey(secret)
	Tokens         auth.Authenticator
	Principals     auth.PrincipalLookup // used only when SubIsPrincipal is false
	Sessions       *Manager
	Cookies        Cookies
	HTTP           *http.Client
	Logger         *slog.Logger
	Now            func() time.Time
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

func (l *Login) endpoints() Endpoints {
	if l.Endpoints != nil {
		return l.Endpoints
	}
	return HubEndpoints{URL: l.HubURL}
}

func (l *Login) pages() pageSet { return pagesFor(l.Provider) }

// unavailableReason is the log reason when the issuer cannot be reached at sign-in.
func (l *Login) unavailableReason() string {
	if l.SubIsPrincipal {
		return "issuer_unavailable"
	}
	return "hub_unavailable"
}

// Endpoints is the issuer a browser signs in through.
type Endpoints interface {
	// AuthorizeURL is where the browser goes to sign in.
	AuthorizeURL(clientID, redirectURI, state, challenge string) string
	// Exchange trades the code for an access token, server to server, with no Origin header.
	Exchange(ctx context.Context, hc *http.Client, clientID, code, verifier, redirectURI string) (string, error)
}

// HubEndpoints is the AgentPod hub's authorize page and JSON token exchange.
type HubEndpoints struct{ URL string }

func (e HubEndpoints) AuthorizeURL(clientID, redirectURI, state, challenge string) string {
	q := url.Values{}
	q.Set("client", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	return e.URL + "/api/auth/authorize?" + q.Encode()
}

func (e HubEndpoints) Exchange(ctx context.Context, hc *http.Client, _, code, verifier, redirectURI string) (string, error) {
	body, _ := json.Marshal(map[string]string{"code": code, "code_verifier": verifier, "redirect_uri": redirectURI})
	var out struct {
		Token string `json:"token"`
	}
	if err := postForToken(ctx, hc, e.URL+"/api/auth/token/exchange", "application/json", bytes.NewReader(body), &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", errExchangeRefused
	}
	return out.Token, nil
}

// PlaneEndpoints is the organization plane's OAuth 2.1 authorization code flow with PKCE (S256)
// for a first-party public client. Both requests carry resource=<SW_PUBLIC_URL>, so the token's
// aud is this deployment. The refresh token in the answer is not kept: superwitness's own session
// holds the sign-in, and signing in again re-runs authorize.
type PlaneEndpoints struct {
	URL      string // SW_ORG_PLANE_URL
	Resource string // SW_PUBLIC_URL
}

func (e PlaneEndpoints) AuthorizeURL(clientID, redirectURI, state, challenge string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", "openid")
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("resource", e.Resource)
	return e.URL + "/api/auth/oauth2/authorize?" + q.Encode()
}

func (e PlaneEndpoints) Exchange(ctx context.Context, hc *http.Client, clientID, code, verifier, redirectURI string) (string, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI},
		"client_id": {clientID}, "code_verifier": {verifier}, "resource": {e.Resource}}
	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := postForToken(ctx, hc, e.URL+"/api/auth/oauth2/token", "application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()), &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" || !strings.EqualFold(out.TokenType, "Bearer") {
		return "", errExchangeRefused
	}
	return out.AccessToken, nil
}

// postForToken POSTs body and decodes a 200 answer into out. A transport error or 5xx is
// errIssuerUnavailable; any other answer is errExchangeRefused.
func postForToken(ctx context.Context, hc *http.Client, u, contentType string, body io.Reader, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", errIssuerUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w: HTTP %d", errIssuerUnavailable, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(out) != nil {
		return errExchangeRefused
	}
	return nil
}

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
	http.Redirect(w, r, l.endpoints().AuthorizeURL(l.ClientID, l.redirectURI(), lc.State, Challenge(lc.Verifier)), http.StatusFound)
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
		l.refuse(w, http.StatusBadRequest, "login_missing", l.pages().expired)
		return
	}
	lc, err := openLoginCookie(ck.Value, l.Key, l.now())
	switch {
	case errors.Is(err, errLoginExpired):
		l.refuse(w, http.StatusBadRequest, "login_expired", l.pages().expired)
		return
	case err != nil:
		l.refuse(w, http.StatusBadRequest, "login_invalid", l.pages().expired)
		return
	}
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(lc.State)) != 1 {
		l.refuse(w, http.StatusBadRequest, "state_mismatch", l.pages().expired)
		return
	}
	code := q.Get("code")
	if code == "" {
		l.refuse(w, http.StatusBadRequest, "no_code", l.pages().failed)
		return
	}
	tok, err := l.endpoints().Exchange(r.Context(), l.HTTP, l.ClientID, code, lc.Verifier, l.redirectURI())
	if errors.Is(err, errIssuerUnavailable) {
		l.refuse(w, http.StatusServiceUnavailable, l.unavailableReason(), l.pages().down)
		return
	}
	if err != nil {
		l.refuse(w, http.StatusBadRequest, "exchange_refused", l.pages().failed)
		return
	}
	p, err := l.Tokens.Verify(r.Context(), tok)
	var ne *auth.NotEnabledError
	switch {
	case errors.As(err, &ne):
		l.refuse(w, http.StatusForbidden, "product_not_enabled", l.pages().notAuthorised, "org", ne.Org)
		return
	case err != nil:
		l.refuse(w, http.StatusBadRequest, "token_invalid", l.pages().failed)
		return
	}
	if !l.SubIsPrincipal {
		// The hub's browser token names the hub account, not always the principal id: resolve it.
		rec, err := l.Principals.Lookup(r.Context(), p.ID)
		switch {
		case errors.Is(err, auth.ErrPrincipalNotFound):
			l.refuse(w, http.StatusForbidden, "no_principal", l.pages().notAuthorised)
			return
		case err != nil:
			l.refuse(w, http.StatusServiceUnavailable, "hub_unavailable", l.pages().down)
			return
		}
		if rec.Suspended {
			l.refuse(w, http.StatusForbidden, "suspended", l.pages().notAuthorised, "principal", rec.ID)
			return
		}
		if rec.Kind != auth.KindHuman {
			l.refuse(w, http.StatusForbidden, "not_human", l.pages().notAuthorised, "principal", rec.ID)
			return
		}
		p.ID = rec.ID
	}
	// The token's principalKind claim must say human; under the plane it is the only word on it.
	if p.Kind != auth.KindHuman {
		l.refuse(w, http.StatusForbidden, "not_human", l.pages().notAuthorised, "principal", p.ID)
		return
	}
	if !l.Sessions.Allowed[p.ID] {
		l.refuse(w, http.StatusForbidden, "not_allowlisted", l.pages().notAuthorised, "principal", p.ID)
		return
	}
	l.revokePrevious(r)
	token, err := l.Sessions.Issue(r.Context(), p)
	if err != nil {
		l.refuse(w, http.StatusServiceUnavailable, "store_unavailable", l.pages().database, "principal", p.ID)
		return
	}
	l.Cookies.Set(w, l.Cookies.SessionName(), token, AbsoluteTTL)
	l.log().Info("auth.signin", "principal", p.ID)
	http.Redirect(w, r, lc.Next, http.StatusSeeOther)
}

// revokePrevious ends the session the browser already holds, if any, before a new one is
// issued: signing in again must not leave the old cookie's session alive in the store. A store
// failure is logged and does not block the sign-in; the old session still expires on its own.
func (l *Login) revokePrevious(r *http.Request) {
	c, err := r.Cookie(l.Cookies.SessionName())
	if err != nil || c.Value == "" {
		return
	}
	s, err := l.Sessions.Revoke(r.Context(), c.Value)
	switch {
	case err == nil:
		l.log().Info("auth.signout", "principal", s.Principal, "reason", "signed_in_again")
	case errors.Is(err, ErrNotFound):
	default:
		attrs := []any{"reason", "signed_in_again"}
		if s.Principal != "" {
			attrs = append(attrs, "principal", s.Principal)
		}
		l.log().Warn("auth.signout_failed", attrs...)
	}
}

var (
	errIssuerUnavailable = errors.New("issuer unavailable")
	errExchangeRefused   = errors.New("the issuer refused the code")
)

func (l *Login) logout(w http.ResponseWriter, r *http.Request) {
	// Compare against the normalised origin, as the gate does for session writes: the browser
	// sends https://app.example.com for SW_PUBLIC_URL=https://App.Example.com:443. An empty
	// Origin field refuses every sign-out rather than accepting a request with no Origin.
	if o := r.Header.Get("Origin"); l.Origin == "" || o != l.Origin {
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
