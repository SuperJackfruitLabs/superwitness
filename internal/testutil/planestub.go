package testutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// PlaneStub stands in for the organization plane's OAuth 2.1 endpoints in sign-in tests: a JWKS,
// discovery, /api/auth/oauth2/authorize (signs in whoever SignInAs named, at once),
// /api/auth/oauth2/token (form-encoded, PKCE S256, resource required) and /api/auth/oauth2/revoke.
// The code exchange answers a refresh token when the authorize asked for offline_access. Refresh
// tokens rotate: a spent one replays its answer for ReplayWindow, and is refused after that.
type PlaneStub struct {
	URL, Resource, ClientID string
	Down                    atomic.Bool // the token endpoint answers 503
	SawOrigin               atomic.Bool
	AudienceOverride        string        // non-empty: mint for this audience instead (a misconfigured plane)
	RevocationEndpoint      string        // non-empty: discovery names this revocation endpoint instead
	RefreshDelay            time.Duration // a refresh answers this late (to make concurrent refreshes overlap)
	ReplayWindow            time.Duration // zero: 30 seconds
	RefreshSubOverride      string        // non-empty: a refresh mints for this sub (a plane gone wrong)
	Refreshes               atomic.Int32  // refresh_token grants answered 200
	RefreshRequests         atomic.Int32  // refresh_token grants received
	Revocations             atomic.Int32  // revocation requests received

	grants  map[string]*planeGrant // refresh token -> its grant
	revoked []string

	srv           *httptest.Server
	priv          ed25519.PrivateKey
	mu            sync.Mutex
	sub, kind     string
	email         string
	ent           []string
	codes         map[string]planeCode
	LastAuthorize url.Values
	LastToken     url.Values
}

type planeCode struct{ challenge, redirect, sub, kind, email, scope string }

// planeGrant is one refresh token: who it is for, and, once spent, the answer it replays.
type planeGrant struct {
	sub, kind, email string
	ended            bool
	spentAt          time.Time
	answer           []byte
}

func NewPlaneStub(t testing.TB, resource, clientID string) *PlaneStub {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p := &PlaneStub{Resource: resource, ClientID: clientID, priv: priv, codes: map[string]planeCode{},
		grants: map[string]*planeGrant{}, sub: "prn_human01", kind: "human", ent: []string{"superwitness"}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/jwks", p.jwks)
	mux.HandleFunc("GET /api/auth/oauth2/authorize", p.authorize)
	mux.HandleFunc("POST /api/auth/oauth2/token", p.token)
	mux.HandleFunc("POST /api/auth/oauth2/revoke", p.revoke)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", p.discovery)
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	p.URL = p.srv.URL
	return p
}

func (p *PlaneStub) SignInAs(sub, kind, email string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sub, p.kind, p.email = sub, kind, email
}

// SetEnt sets the products enabled for the signed-in person's workspace.
func (p *PlaneStub) SetEnt(ent []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ent = ent
}

func (p *PlaneStub) jwks(w http.ResponseWriter, _ *http.Request) {
	pub := p.priv.Public().(ed25519.PublicKey)
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "OKP", "crv": "Ed25519",
		"alg": "EdDSA", "kid": "plane1", "x": base64.RawURLEncoding.EncodeToString(pub)}}})
}

func (p *PlaneStub) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p.mu.Lock()
	p.LastAuthorize = q
	p.mu.Unlock()
	back, err := url.Parse(q.Get("redirect_uri"))
	if q.Get("response_type") != "code" || q.Get("client_id") != p.ClientID || q.Get("code_challenge_method") != "S256" ||
		len(q.Get("code_challenge")) != 43 || q.Get("state") == "" || q.Get("resource") != p.Resource || err != nil || back.Host == "" {
		http.Error(w, "refused", http.StatusBadRequest)
		return
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	code := base64.RawURLEncoding.EncodeToString(b[:])
	p.mu.Lock()
	p.codes[code] = planeCode{challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri"), sub: p.sub, kind: p.kind,
		email: p.email, scope: q.Get("scope")}
	p.mu.Unlock()
	v := back.Query()
	v.Set("code", code)
	v.Set("state", q.Get("state"))
	back.RawQuery = v.Encode()
	http.Redirect(w, r, back.String(), http.StatusFound)
}

func (p *PlaneStub) token(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" {
		p.SawOrigin.Store(true)
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}
	f := r.PostForm
	if f.Get("grant_type") == "refresh_token" {
		p.refresh(w, f) // counts the request, then answers 503 when Down
		return
	}
	if p.Down.Load() {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	p.mu.Lock()
	p.LastToken = f
	c, ok := p.codes[f.Get("code")]
	delete(p.codes, f.Get("code"))
	p.mu.Unlock()
	sum := sha256.Sum256([]byte(f.Get("code_verifier")))
	if !ok || f.Get("grant_type") != "authorization_code" || f.Get("client_id") != p.ClientID || c.redirect != f.Get("redirect_uri") ||
		f.Get("resource") != p.Resource || base64.RawURLEncoding.EncodeToString(sum[:]) != c.challenge {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	offline := false
	for _, sc := range strings.Fields(c.scope) {
		offline = offline || sc == "offline_access"
	}
	b, err := p.answer(c.sub, c.kind, c.email, offline)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(b)
}

// answer mints an access token, and with offline a new refresh token, and returns the JSON answer.
func (p *PlaneStub) answer(sub, kind, email string, offline bool) ([]byte, error) {
	p.mu.Lock()
	ent := p.ent
	p.mu.Unlock()
	aud := p.Resource
	if p.AudienceOverride != "" {
		aud = p.AudienceOverride
	}
	now := time.Now()
	claims := jwt.MapClaims{"iss": p.URL, "aud": aud, "sub": sub, "principalKind": kind, "org": "org_01", "ent": ent,
		"mayDispatch": []string{}, "mayGrantReach": false, "scope": "openid", "jti": "jti_" + random(8),
		"client_id": p.ClientID, "azp": p.ClientID, "sid": "sid_01", "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix()}
	if email != "" {
		claims["email"], claims["email_verified"] = email, true
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = "plane1"
	s, err := tok.SignedString(p.priv)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"access_token": s, "token_type": "Bearer", "expires_in": 300, "scope": "openid"}
	if offline {
		rt := "rt_" + random(24)
		out["refresh_token"], out["scope"] = rt, "openid offline_access"
		p.mu.Lock()
		p.grants[rt] = &planeGrant{sub: sub, kind: kind, email: email}
		p.mu.Unlock()
	}
	return json.Marshal(out)
}

func (p *PlaneStub) refresh(w http.ResponseWriter, f url.Values) {
	p.RefreshRequests.Add(1)
	time.Sleep(p.RefreshDelay)
	if p.Down.Load() {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	p.mu.Lock()
	p.LastToken = f
	g, ok := p.grants[f.Get("refresh_token")]
	window := p.ReplayWindow
	if window == 0 {
		window = 30 * time.Second
	}
	refused := !ok || g.ended || f.Get("client_id") != p.ClientID || f.Get("resource") != p.Resource
	if !refused && !g.spentAt.IsZero() {
		if time.Since(g.spentAt) < window { // a retried refresh of the same token gets the same answer
			b := g.answer
			p.mu.Unlock()
			p.Refreshes.Add(1)
			_, _ = w.Write(b)
			return
		}
		refused = true
	}
	p.mu.Unlock()
	if refused {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	sub := g.sub
	if p.RefreshSubOverride != "" {
		sub = p.RefreshSubOverride
	}
	b, err := p.answer(sub, g.kind, g.email, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.mu.Lock()
	g.spentAt, g.answer = time.Now(), b
	p.mu.Unlock()
	p.Refreshes.Add(1)
	_, _ = w.Write(b)
}

func (p *PlaneStub) revoke(w http.ResponseWriter, r *http.Request) {
	p.Revocations.Add(1)
	if err := r.ParseForm(); err != nil || r.PostForm.Get("client_id") != p.ClientID {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}
	rt := r.PostForm.Get("token")
	p.mu.Lock()
	if g, ok := p.grants[rt]; ok {
		g.ended = true
	}
	p.revoked = append(p.revoked, rt)
	p.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (p *PlaneStub) discovery(w http.ResponseWriter, _ *http.Request) {
	rev := p.URL + "/api/auth/oauth2/revoke"
	if p.RevocationEndpoint != "" {
		rev = p.RevocationEndpoint
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"issuer": p.URL, "token_endpoint": p.URL + "/api/auth/oauth2/token",
		"authorization_endpoint": p.URL + "/api/auth/oauth2/authorize", "revocation_endpoint": rev})
}

// EndGrants ends every refresh token issued so far, as signing the app out at the plane does.
func (p *PlaneStub) EndGrants() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, g := range p.grants {
		g.ended = true
	}
}

// LiveRefreshTokens is every refresh token issued that is neither spent nor ended.
func (p *PlaneStub) LiveRefreshTokens() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for rt, g := range p.grants {
		if !g.ended && g.spentAt.IsZero() {
			out = append(out, rt)
		}
	}
	return out
}

// Revoked is every token the revocation endpoint received.
func (p *PlaneStub) Revoked() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.revoked...)
}

func random(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}
