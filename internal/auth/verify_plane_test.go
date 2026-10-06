package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// signPlane signs the plane's claim set: no tenant; org, ent, jti and the grant claims present.
func (is *issuer) signPlane(t *testing.T, now time.Time, edit func(jwt.MapClaims)) string {
	t.Helper()
	return is.sign(t, "k1", now, func(c jwt.MapClaims) {
		delete(c, "tenant")
		c["org"], c["ent"], c["jti"] = "org_01", []string{"superpipeline", "superwitness"}, "jti_01"
		c["mayDispatch"], c["mayGrantReach"] = []string{}, false
		if edit != nil {
			edit(c)
		}
	})
}

func newPlaneVerifier(is *issuer, now *time.Time) *Verifier {
	v := NewPlaneVerifier(is.srv.URL, is.srv.URL+"/api/auth/jwks", testAudience, is.srv.Client())
	v.Now = func() time.Time { return *now }
	return v
}

func TestPlaneVerifyAccepts(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	p, err := newPlaneVerifier(is, &now).Verify(context.Background(), is.signPlane(t, now, func(c jwt.MapClaims) {
		c["act"] = map[string]string{"sub": "prn_hub01"}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "prn_agent01" || p.Kind != KindAgent || p.Tenant != "org_01" || p.Actor != "prn_hub01" {
		t.Errorf("got %+v", p)
	}
}

func TestPlaneVerifyAudienceStringOrArray(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := newPlaneVerifier(is, &now)
	for name, aud := range map[string]any{
		"string": testAudience, "array of one": []string{testAudience}, "array containing it": []string{"https://hub.example", testAudience},
	} {
		if _, err := v.Verify(context.Background(), is.signPlane(t, now, func(c jwt.MapClaims) { c["aud"] = aud })); err != nil {
			t.Errorf("aud %s: %v", name, err)
		}
	}
	for name, aud := range map[string]any{
		"the hub's": "https://hub.example", "superpipeline's": []string{"https://app.superpipeline.example"},
		"a prefix of ours": "https://superwitness.example.evil", "empty array": []string{},
	} {
		if _, err := v.Verify(context.Background(), is.signPlane(t, now, func(c jwt.MapClaims) { c["aud"] = aud })); err == nil {
			t.Errorf("aud %s accepted", name)
		}
	}
}

func TestPlaneVerifyRejects(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := newPlaneVerifier(is, &now)
	cases := map[string]func(jwt.MapClaims){
		"issuer with a trailing slash": func(c jwt.MapClaims) { c["iss"] = is.srv.URL + "/" },
		"issuer as a prefix":           func(c jwt.MapClaims) { c["iss"] = is.srv.URL + ".evil" },
		"the hub's sub, an account id": func(c jwt.MapClaims) { c["sub"] = "hubuser_01" },
		"no org":                       func(c jwt.MapClaims) { delete(c, "org") },
		"org without its prefix":       func(c jwt.MapClaims) { c["org"] = "tenant_01" },
		"bare org prefix":              func(c jwt.MapClaims) { c["org"] = "org_" },
		"no ent":                       func(c jwt.MapClaims) { delete(c, "ent") },
		"ent as a string":              func(c jwt.MapClaims) { c["ent"] = "superwitness" },
		"unknown kind":                 func(c jwt.MapClaims) { c["principalKind"] = "robot" },
		"expired":                      func(c jwt.MapClaims) { c["exp"] = now.Add(-time.Hour).Unix() },
		"no iat":                       func(c jwt.MapClaims) { delete(c, "iat") },
		"scope as an array":            func(c jwt.MapClaims) { c["scope"] = []string{"runs:write"} },
	}
	for name, edit := range cases {
		if _, err := v.Verify(context.Background(), is.signPlane(t, now, edit)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// A hub token (tenant, no org/ent) is not a plane token, even from the same keys.
	if _, err := v.Verify(context.Background(), is.sign(t, "k1", now, nil)); err == nil {
		t.Error("a hub-shaped token was accepted under the plane")
	}
}

func TestPlaneVerifyEntitlement(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := newPlaneVerifier(is, &now)
	for name, ent := range map[string][]string{"other products": {"agentpod", "superpipeline"}, "none": {}} {
		_, err := v.Verify(context.Background(), is.signPlane(t, now, func(c jwt.MapClaims) { c["ent"] = ent }))
		var ne *NotEnabledError
		if !errors.As(err, &ne) || ne.Org != "org_01" || !errors.Is(err, ErrUnauthenticated) {
			t.Errorf("ent %s: %v; want NotEnabledError{org_01} wrapping ErrUnauthenticated", name, err)
		}
	}
}

func TestPlaneVerifyReadsGrantScopesOnlyFromAgentsAndServices(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := newPlaneVerifier(is, &now)
	human, err := v.Verify(context.Background(), is.signPlane(t, now, func(c jwt.MapClaims) {
		c["sub"], c["principalKind"], c["scope"] = "prn_human01", "human", "openid runs:write"
	}))
	if err != nil || human.HasScope("runs:write") || len(human.Scopes) != 0 {
		t.Errorf("a person's OAuth scope string became grant scopes: %+v %v", human, err)
	}
	for _, kind := range []string{"service", "agent"} {
		p, err := v.Verify(context.Background(), is.signPlane(t, now, func(c jwt.MapClaims) {
			c["principalKind"], c["scope"] = kind, "runs:write"
		}))
		if err != nil || !p.HasScope("runs:write") {
			t.Errorf("%s: %+v %v", kind, p, err)
		}
	}
}

func TestPlaneVerifyFetchesTheConfiguredJWKSURL(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	// The issuer string serves nothing; only the configured JWKS URL does.
	v := NewPlaneVerifier("https://accounts.example", is.srv.URL+"/api/auth/jwks", testAudience, is.srv.Client())
	v.Now = func() time.Time { return now }
	if _, err := v.Verify(context.Background(), is.signPlane(t, now, func(c jwt.MapClaims) { c["iss"] = "https://accounts.example" })); err != nil {
		t.Fatalf("key from SW_ORG_PLANE_JWKS_URL not used: %v", err)
	}
	if is.fetches.Load() != 1 {
		t.Errorf("fetches = %d", is.fetches.Load())
	}
}

func TestPlaneVerifyServesTheLastGoodKeySet(t *testing.T) {
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := newPlaneVerifier(is, &now)
	if _, err := v.Verify(context.Background(), is.signPlane(t, now, nil)); err != nil {
		t.Fatal(err)
	}
	now = now.Add(11 * time.Minute)
	is.down.Store(true)
	if _, err := v.Verify(context.Background(), is.signPlane(t, now, nil)); err != nil {
		t.Errorf("plane outage rejected a token signed by a cached key: %v", err)
	}
	is.down.Store(false)
	now = now.Add(minMissGap + time.Second) // the failed attempt above holds off refetches for minMissGap
	is.addKey(t, "k2")                      // rotation: one refetch on the unknown kid
	tok := is.sign(t, "k2", now, func(c jwt.MapClaims) {
		delete(c, "tenant")
		c["org"], c["ent"] = "org_01", []string{"superwitness"}
	})
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Errorf("rotated key: %v", err)
	}
}

func TestPlaneVerifyAcceptsTheContractFixture(t *testing.T) {
	b, err := os.ReadFile("testdata/org-plane-tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string         `json:"name"`
		Claims map[string]any `json:"claims"`
		Want   struct {
			ID, Kind, Org, Email, Actor string
			Scopes                      []string
		} `json:"want"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	is := newIssuer(t)
	is.addKey(t, "k1")
	now := time.Now()
	v := newPlaneVerifier(is, &now)
	for _, c := range cases {
		claims := jwt.MapClaims(c.Claims)
		claims["iss"], claims["iat"], claims["exp"] = is.srv.URL, now.Unix(), now.Add(5*time.Minute).Unix()
		tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		tok.Header["kid"] = "k1"
		is.mu.Lock()
		s, err := tok.SignedString(is.keys["k1"])
		is.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		p, err := v.Verify(context.Background(), s)
		if err != nil {
			t.Errorf("%s: %v", c.Name, err)
			continue
		}
		if p.ID != c.Want.ID || string(p.Kind) != c.Want.Kind || p.Tenant != c.Want.Org || p.Email != c.Want.Email ||
			p.Actor != c.Want.Actor || !slices.Equal(p.Scopes, c.Want.Scopes) && !(len(p.Scopes) == 0 && len(c.Want.Scopes) == 0) {
			t.Errorf("%s: got %+v", c.Name, p)
		}
	}
}
