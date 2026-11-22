package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// tokenVectors is treaty's conformance corpus for access-token verifiers, vendored by
// `treaty sync` (treaty.json) and held byte-identical to the pinned release by `treaty verify`.
const tokenVectors = "testdata/treaty/token_vectors.json"

type tokenVectorFile struct {
	RefusalCodes []struct {
		Code string `json:"code"`
	} `json:"refusalCodes"`
	JWKS  json.RawMessage `json:"jwks"`
	Cases []tokenVector   `json:"cases"`
}

type tokenVector struct {
	Name     string `json:"name"`
	Why      string `json:"why"`
	Token    string `json:"token"`
	Verifier struct {
		Issuer, Audience, Product string
		Now                       int64
	} `json:"verifier"`
	Expect struct {
		OK     bool           `json:"ok"`
		Code   string         `json:"code"`
		Claims map[string]any `json:"claims"`
	} `json:"expect"`
}

func readTokenVectors(t *testing.T) tokenVectorFile {
	t.Helper()
	b, err := os.ReadFile(tokenVectors)
	if err != nil {
		t.Fatal(err)
	}
	var f tokenVectorFile
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// vectorCode is the treaty refusal code SuperWitness's error stands for: a NotEnabledError is
// product_not_enabled, a TokenError carries its code, and anything else has none.
func vectorCode(err error) string {
	var ne *NotEnabledError
	var te *TokenError
	switch {
	case errors.As(err, &ne):
		return "product_not_enabled"
	case errors.As(err, &te):
		return te.Code
	default:
		return "uncoded: " + err.Error()
	}
}

// principalFrom is the Principal SuperWitness should map a vector's expected claims into.
func principalFrom(c map[string]any) Principal {
	p := Principal{ID: c["sub"].(string), Kind: PrincipalKind(c["principalKind"].(string)), Tenant: c["org"].(string)}
	p.Email, _ = c["email"].(string)
	if p.Kind == KindAgent || p.Kind == KindService {
		scope, _ := c["scope"].(string)
		p.Scopes = strings.Fields(scope)
	}
	if act, ok := c["act"].(map[string]any); ok {
		p.Actor, _ = act["sub"].(string)
	}
	return p
}

func samePrincipal(a, b Principal) bool {
	return a.ID == b.ID && a.Kind == b.Kind && a.Tenant == b.Tenant && a.Email == b.Email && a.Actor == b.Actor &&
		slices.Equal(a.Scopes, b.Scopes)
}

// Every vector in treaty's corpus produces its expected outcome through the plane verifier, with
// the corpus's JWKS served the way the plane serves it.
func TestPlaneVerifierPassesTreatyTokenVectors(t *testing.T) {
	f := readTokenVectors(t)
	if len(f.Cases) == 0 {
		t.Fatalf("%s holds no cases: the conformance test would pass vacuously", tokenVectors)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(f.JWKS)
	}))
	t.Cleanup(jwks.Close)
	ran := 0
	for _, c := range f.Cases {
		t.Run(c.Name, func(t *testing.T) {
			ran++
			v := NewPlaneVerifier(c.Verifier.Issuer, jwks.URL+"/jwks", c.Verifier.Audience, jwks.Client())
			v.product = c.Verifier.Product
			v.Now = func() time.Time { return time.Unix(c.Verifier.Now, 0) }
			p, err := v.Verify(context.Background(), c.Token)
			switch {
			case c.Expect.OK && err != nil:
				t.Errorf("refused with %s (%v); want accepted. %s", vectorCode(err), err, c.Why)
			case c.Expect.OK:
				if want := principalFrom(c.Expect.Claims); !samePrincipal(p, want) {
					t.Errorf("principal %+v; want %+v", p, want)
				}
			case err == nil:
				t.Errorf("accepted as %+v; want %s. %s", p, c.Expect.Code, c.Why)
			case vectorCode(err) != c.Expect.Code:
				t.Errorf("refused with %s (%v); want %s. %s", vectorCode(err), err, c.Expect.Code, c.Why)
			}
		})
	}
	if ran != len(f.Cases) {
		t.Fatalf("ran %d of %d vectors", ran, len(f.Cases))
	}
}

// The corpus still exercises every refusal code it declares and an accepted token, so a gutted
// or truncated copy cannot pass the test above by leaving outcomes out.
func TestTreatyTokenVectorsCoverEveryOutcome(t *testing.T) {
	f := readTokenVectors(t)
	seen := map[string]bool{}
	for _, c := range f.Cases {
		if c.Expect.OK {
			seen["ok"] = true
		} else {
			seen[c.Expect.Code] = true
		}
	}
	if len(f.RefusalCodes) == 0 || !seen["ok"] {
		t.Fatalf("the corpus declares %d refusal codes and accepted tokens: %v", len(f.RefusalCodes), seen["ok"])
	}
	for _, rc := range f.RefusalCodes {
		if !seen[rc.Code] {
			t.Errorf("no vector expects %s", rc.Code)
		}
	}
}
