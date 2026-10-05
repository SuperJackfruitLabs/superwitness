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

// HubStub stands in for the AgentPod hub in sign-in tests. It publishes a JWKS, answers the
// authorize redirect at once for whoever SignInAs named, and exchanges each code once for a
// signed token, checking the PKCE verifier as the hub does.
type HubStub struct {
	URL       string
	Audience  string
	Down      atomic.Bool  // the exchange answers 503
	SawOrigin atomic.Bool  // an exchange arrived carrying an Origin header
	Exchanges atomic.Int32 // exchanges attempted

	srv   *httptest.Server
	priv  ed25519.PrivateKey
	mu    sync.Mutex
	sub   string
	kind  string
	email string
	codes map[string]stubCode
}

type stubCode struct{ challenge, redirect, sub, kind, email string }

func NewHubStub(t testing.TB, audience string) *HubStub {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	h := &HubStub{Audience: audience, priv: priv, codes: map[string]stubCode{}, sub: "hubuser_01", kind: "human"}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/jwks", h.jwks)
	mux.HandleFunc("GET /api/auth/authorize", h.authorize)
	mux.HandleFunc("POST /api/auth/token/exchange", h.exchange)
	h.srv = httptest.NewServer(mux)
	t.Cleanup(h.srv.Close)
	h.URL = h.srv.URL
	return h
}

// Stop takes the hub away entirely, as an outage would.
func (h *HubStub) Stop() { h.srv.Close() }

// SignInAs sets who the next authorize signs in: a hub account id (or prn_ id), a principal
// kind and an email ("" for none).
func (h *HubStub) SignInAs(sub, kind, email string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sub, h.kind, h.email = sub, kind, email
}

func (h *HubStub) jwks(w http.ResponseWriter, _ *http.Request) {
	pub := h.priv.Public().(ed25519.PublicKey)
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "OKP", "crv": "Ed25519",
		"alg": "EdDSA", "kid": "stub1", "x": base64.RawURLEncoding.EncodeToString(pub)}}})
}

func (h *HubStub) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	back, err := url.Parse(q.Get("redirect_uri"))
	if q.Get("client") == "" || q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 ||
		q.Get("state") == "" || err != nil || back.Host == "" {
		http.Error(w, "refused", http.StatusBadRequest) // the hub's refusals redirect nowhere
		return
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	code := base64.RawURLEncoding.EncodeToString(b[:])
	h.mu.Lock()
	h.codes[code] = stubCode{challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri"), sub: h.sub, kind: h.kind, email: h.email}
	h.mu.Unlock()
	v := back.Query()
	v.Set("code", code)
	v.Set("state", q.Get("state"))
	back.RawQuery = v.Encode()
	http.Redirect(w, r, back.String(), http.StatusFound)
}

func (h *HubStub) exchange(w http.ResponseWriter, r *http.Request) {
	h.Exchanges.Add(1)
	if r.Header.Get("Origin") != "" {
		h.SawOrigin.Store(true)
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}
	if h.Down.Load() {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	var body map[string]string
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}
	h.mu.Lock()
	c, ok := h.codes[body["code"]]
	delete(h.codes, body["code"]) // spent before the verifier is checked, as the hub does
	h.mu.Unlock()
	sum := sha256.Sum256([]byte(body["code_verifier"]))
	if !ok || c.redirect != body["redirect_uri"] || base64.RawURLEncoding.EncodeToString(sum[:]) != c.challenge {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	now := time.Now()
	claims := jwt.MapClaims{"iss": h.URL, "aud": []string{h.Audience}, "sub": c.sub, "principalKind": c.kind,
		"tenant": "tenant_01", "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix()}
	if c.email != "" {
		claims["email"], claims["email_verified"] = c.email, true
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = "stub1"
	s, err := tok.SignedString(h.priv)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"token": s, "expiresIn": 300})
}
