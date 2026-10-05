package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testAudience = "https://superwitness.example"

type issuer struct {
	srv     *httptest.Server
	mu      sync.Mutex
	keys    map[string]ed25519.PrivateKey
	fetches atomic.Int32
	down    atomic.Bool
}

func newIssuer(t *testing.T) *issuer {
	t.Helper()
	is := &issuer{keys: map[string]ed25519.PrivateKey{}}
	is.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/jwks" {
			http.NotFound(w, r)
			return
		}
		is.fetches.Add(1)
		if is.down.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		is.mu.Lock()
		defer is.mu.Unlock()
		var keys []map[string]string
		for kid, priv := range is.keys {
			pub := priv.Public().(ed25519.PublicKey)
			keys = append(keys, map[string]string{"kty": "OKP", "crv": "Ed25519", "alg": "EdDSA", "kid": kid,
				"x": base64.RawURLEncoding.EncodeToString(pub)})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	t.Cleanup(is.srv.Close)
	return is
}

func (is *issuer) addKey(t *testing.T, kid string) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	is.mu.Lock()
	is.keys[kid] = priv
	is.mu.Unlock()
}

func (is *issuer) sign(t *testing.T, kid string, now time.Time, edit func(jwt.MapClaims)) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss": is.srv.URL, "aud": testAudience, "sub": "prn_agent01", "principalKind": "agent",
		"tenant": "fleet_01", "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
	}
	if edit != nil {
		edit(claims)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = kid
	is.mu.Lock()
	priv := is.keys[kid]
	is.mu.Unlock()
	if priv == nil {
		_, priv, _ = ed25519.GenerateKey(rand.Reader) // a key the issuer never published
	}
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newTestVerifier(is *issuer, now *time.Time) *Verifier {
	v := NewVerifier(is.srv.URL, testAudience, is.srv.Client())
	v.Now = func() time.Time { return *now }
	return v
}

func TestVerifyAccepts(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := newTestVerifier(is, &now)
	p, err := v.Verify(context.Background(), is.sign(t, "k1", now, func(c jwt.MapClaims) { c["act"] = map[string]string{"sub": "agentpod:bridge"} }))
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "prn_agent01" || p.Kind != KindAgent || p.Tenant != "fleet_01" || p.Actor != "agentpod:bridge" {
		t.Errorf("got %+v", p)
	}
}

func TestVerifyRejects(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := newTestVerifier(is, &now)
	cases := map[string]func(jwt.MapClaims){
		"wrong audience": func(c jwt.MapClaims) { c["aud"] = is.srv.URL },
		"wrong issuer":   func(c jwt.MapClaims) { c["iss"] = "https://evil.example" },
		"expired":        func(c jwt.MapClaims) { c["exp"] = now.Add(-time.Hour).Unix() },
		"no iat":         func(c jwt.MapClaims) { delete(c, "iat") },
		"no exp":         func(c jwt.MapClaims) { delete(c, "exp") },
		"no tenant":      func(c jwt.MapClaims) { delete(c, "tenant") },
		"unknown kind":   func(c jwt.MapClaims) { c["principalKind"] = "robot" },
		"no subject":     func(c jwt.MapClaims) { delete(c, "sub") },
	}
	for name, edit := range cases {
		if _, err := v.Verify(context.Background(), is.sign(t, "k1", now, edit)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"iss": is.srv.URL, "aud": testAudience, "sub": "x"})
	hs.Header["kid"] = "k1"
	hsTok, _ := hs.SignedString([]byte("secret"))
	if _, err := v.Verify(context.Background(), hsTok); err == nil {
		t.Error("HS256 accepted")
	}
	noKid := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{"sub": "x"})
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	noKidTok, _ := noKid.SignedString(priv)
	before := is.fetches.Load()
	if _, err := v.Verify(context.Background(), noKidTok); err == nil {
		t.Error("token without kid accepted")
	}
	if is.fetches.Load() != before {
		t.Error("a token without kid triggered a JWKS fetch")
	}
	if _, err := v.Verify(context.Background(), "not-a-jwt"); err == nil {
		t.Error("garbage accepted")
	}
}

func TestRotatedKeyAcceptedAfterOneRefetch(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := newTestVerifier(is, &now)
	if _, err := v.Verify(context.Background(), is.sign(t, "k1", now, nil)); err != nil {
		t.Fatal(err)
	}
	is.addKey(t, "k2")
	if _, err := v.Verify(context.Background(), is.sign(t, "k2", now, nil)); err != nil {
		t.Fatalf("rotated key rejected: %v", err)
	}
	if got := is.fetches.Load(); got != 2 {
		t.Errorf("fetches = %d, want 2", got)
	}
}

func TestUnknownKidStormIsRateLimited(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := newTestVerifier(is, &now)
	_, _ = v.Verify(context.Background(), is.sign(t, "k1", now, nil)) // fetch 1
	for i := range 20 {
		_, _ = v.Verify(context.Background(), is.sign(t, "bogus"+string(rune('a'+i)), now, nil))
	}
	if got := is.fetches.Load(); got != 2 {
		t.Errorf("fetches after storm = %d, want 2", got)
	}
	now = now.Add(11 * time.Second)
	_, _ = v.Verify(context.Background(), is.sign(t, "bogus-late", now, nil))
	if got := is.fetches.Load(); got != 3 {
		t.Errorf("fetches after gap = %d, want 3", got)
	}
}

func TestStaleSetServedWhenIssuerDown(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := newTestVerifier(is, &now)
	if _, err := v.Verify(context.Background(), is.sign(t, "k1", now, nil)); err != nil {
		t.Fatal(err)
	}
	now = now.Add(11 * time.Minute)
	is.down.Store(true)
	if _, err := v.Verify(context.Background(), is.sign(t, "k1", now, nil)); err != nil {
		t.Errorf("issuer outage rejected a token signed by a cached key: %v", err)
	}
}

func TestExpiredCacheIssuerDownDoesNotHammer(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := newTestVerifier(is, &now)
	if _, err := v.Verify(context.Background(), is.sign(t, "k1", now, nil)); err != nil {
		t.Fatal(err)
	}
	now = now.Add(11 * time.Minute)
	is.down.Store(true)
	before := is.fetches.Load()
	for i := range 20 {
		_, _ = v.Verify(context.Background(), is.sign(t, "bogus"+string(rune('a'+i)), now, nil))
		if _, err := v.Verify(context.Background(), is.sign(t, "k1", now, nil)); err != nil {
			t.Fatalf("valid token rejected during outage: %v", err)
		}
	}
	if got := is.fetches.Load() - before; got > 2 {
		t.Errorf("fetches during outage = %d, want <= 2", got)
	}
}

func TestConcurrentColdStartCoalesces(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := NewVerifier(is.srv.URL, testAudience, &http.Client{Transport: slowTransport{is.srv.Client().Transport}})
	v.Now = func() time.Time { return now }
	tok := is.sign(t, "k1", now, nil)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := v.Verify(context.Background(), tok)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent cold start rejected a valid token: %v", err)
		}
	}
	if got := is.fetches.Load(); got != 1 {
		t.Errorf("fetches = %d, want 1", got)
	}
}

// slowTransport delays every request so concurrent callers overlap the in-flight fetch.
type slowTransport struct{ next http.RoundTripper }

func (s slowTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	time.Sleep(100 * time.Millisecond)
	return s.next.RoundTrip(r)
}

func TestVerifyReadsScope(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := newTestVerifier(is, &now)

	p, err := v.Verify(context.Background(), is.sign(t, "k1", now, func(c jwt.MapClaims) {
		c["principalKind"] = "service"
		c["scope"] = "evidence:read runs:write"
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !p.HasScope("runs:write") || !p.HasScope("evidence:read") || p.HasScope("runs") || p.HasScope("") {
		t.Errorf("scopes = %q", p.Scopes)
	}

	p, err = v.Verify(context.Background(), is.sign(t, "k1", now, nil))
	if err != nil {
		t.Fatal(err)
	}
	if p.HasScope("runs:write") {
		t.Errorf("a token without a scope claim granted runs:write: %q", p.Scopes)
	}

	// The hub sends one space-delimited string. Anything else is not a token it minted.
	if _, err := v.Verify(context.Background(), is.sign(t, "k1", now, func(c jwt.MapClaims) {
		c["scope"] = []string{"runs:write"}
	})); err == nil {
		t.Error("a scope claim that is an array was accepted")
	}
}
