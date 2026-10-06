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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// PlaneStub stands in for the organization plane's OAuth 2.1 endpoints in sign-in tests: a JWKS,
// /api/auth/oauth2/authorize (signs in whoever SignInAs named, at once) and /api/auth/oauth2/token
// (form-encoded, PKCE S256, resource required), answering access_token plus a refresh token that a
// correct client never keeps.
type PlaneStub struct {
	URL, Resource, ClientID string
	Down                    atomic.Bool // the token endpoint answers 503
	SawOrigin               atomic.Bool
	AudienceOverride        string // non-empty: mint for this audience instead (a misconfigured plane)

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

type planeCode struct{ challenge, redirect, sub, kind, email string }

func NewPlaneStub(t testing.TB, resource, clientID string) *PlaneStub {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p := &PlaneStub{Resource: resource, ClientID: clientID, priv: priv, codes: map[string]planeCode{},
		sub: "prn_human01", kind: "human", ent: []string{"superwitness"}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/jwks", p.jwks)
	mux.HandleFunc("GET /api/auth/oauth2/authorize", p.authorize)
	mux.HandleFunc("POST /api/auth/oauth2/token", p.token)
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
	p.codes[code] = planeCode{challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri"), sub: p.sub, kind: p.kind, email: p.email}
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
	if p.Down.Load() {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}
	f := r.PostForm
	p.mu.Lock()
	p.LastToken = f
	c, ok := p.codes[f.Get("code")]
	delete(p.codes, f.Get("code"))
	ent := p.ent
	p.mu.Unlock()
	sum := sha256.Sum256([]byte(f.Get("code_verifier")))
	if !ok || f.Get("grant_type") != "authorization_code" || f.Get("client_id") != p.ClientID || c.redirect != f.Get("redirect_uri") ||
		f.Get("resource") != p.Resource || base64.RawURLEncoding.EncodeToString(sum[:]) != c.challenge {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	aud := p.Resource
	if p.AudienceOverride != "" {
		aud = p.AudienceOverride
	}
	now := time.Now()
	claims := jwt.MapClaims{"iss": p.URL, "aud": aud, "sub": c.sub, "principalKind": c.kind, "org": "org_01", "ent": ent,
		"mayDispatch": []string{}, "mayGrantReach": false, "scope": "openid", "jti": "jti_" + f.Get("code")[:8],
		"client_id": p.ClientID, "azp": p.ClientID, "sid": "sid_01", "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix()}
	if c.email != "" {
		claims["email"], claims["email_verified"] = c.email, true
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = "plane1"
	s, err := tok.SignedString(p.priv)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": s, "token_type": "Bearer", "expires_in": 300,
		"refresh_token": "rt_never_kept", "scope": "openid"})
}
