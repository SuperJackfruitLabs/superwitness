package auth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The suite's access-token rules, as treaty states them (fixtures/ecosystem-identity/
// token_vectors.json, vendored in testdata/treaty/): EdDSA over Ed25519 only, a kid required,
// 30 s of leeway on exp, nbf and iat, a lifetime of at most 300 s, and every required claim
// present and in its grammar.
const (
	planeLeeway      = 30  // seconds, on exp, nbf and iat
	planeMaxLifetime = 300 // seconds, exp - iat
)

var (
	prnGrammar = regexp.MustCompile(`^prn_[0-9a-f]{20}$`)
	orgGrammar = regexp.MustCompile(`^org_[0-9a-f]{20}$`)
)

// TokenError is a plane token refused under treaty's token rules. Code is one of treaty's
// refusal codes (malformed, bad_algorithm, missing_kid, unknown_key, bad_signature,
// wrong_issuer, wrong_audience, expired, not_yet_valid, issued_in_future, lifetime_too_long,
// missing_claim, bad_claim); product_not_enabled is a NotEnabledError instead. It unwraps to
// ErrUnauthenticated.
type TokenError struct{ Code, Detail string }

func (e *TokenError) Error() string {
	if e.Detail == "" {
		return "token refused: " + e.Code
	}
	return "token refused: " + e.Code + ": " + e.Detail
}
func (e *TokenError) Unwrap() error { return ErrUnauthenticated }

func refuse(code, detail string) error { return &TokenError{Code: code, Detail: detail} }

// verifyPlaneToken checks raw under treaty's token rules and maps its claims into a Principal.
// The checks run in the reference verifier's order; every refusal is a TokenError, except a
// valid token whose ent lacks the product (NotEnabledError) and a key that could not be fetched.
func (v *Verifier) verifyPlaneToken(ctx context.Context, raw string) (Principal, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Principal{}, refuse("malformed", "not a compact JWS")
	}
	var hdr map[string]any
	if !decodeObject(parts[0], &hdr) {
		return Principal{}, refuse("malformed", "the header is not a JSON object")
	}
	if hdr["alg"] != "EdDSA" {
		return Principal{}, refuse("bad_algorithm", "only EdDSA is accepted")
	}
	kid, _ := hdr["kid"].(string)
	if kid == "" {
		return Principal{}, refuse("missing_kid", "")
	}
	key, err := v.key(ctx, kid)
	if errors.Is(err, errUnknownKid) {
		return Principal{}, refuse("unknown_key", "no Ed25519 key has kid "+kid)
	}
	if err != nil {
		return Principal{}, fmt.Errorf("%w: %w", ErrUnauthenticated, err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(key, []byte(parts[0]+"."+parts[1]), sig) {
		return Principal{}, refuse("bad_signature", "")
	}
	var c map[string]any
	if !decodeObject(parts[1], &c) {
		return Principal{}, refuse("malformed", "the payload is not a JSON object")
	}
	return v.planeClaims(c)
}

// decodeObject reads one base64url JSON object.
func decodeObject(part string, out *map[string]any) bool {
	b, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(b))
	return d.Decode(out) == nil && *out != nil && !d.More()
}

type claimCheck func(any) bool

func isString(x any) bool { _, ok := x.(string); return ok }
func isNumber(x any) bool { _, ok := x.(float64); return ok }
func isBool(x any) bool   { _, ok := x.(bool); return ok }
func isPrn(x any) bool    { s, ok := x.(string); return ok && prnGrammar.MatchString(s) }
func isOrg(x any) bool    { s, ok := x.(string); return ok && orgGrammar.MatchString(s) }
func isKind(x any) bool   { s, ok := x.(string); return ok && validKind(PrincipalKind(s)) }
func isJTI(x any) bool    { s, ok := x.(string); return ok && s != "" }

func every(x any, each claimCheck) bool {
	a, ok := x.([]any)
	return ok && !slices.ContainsFunc(a, func(e any) bool { return !each(e) })
}
func isStrings(x any) bool  { return every(x, isString) }
func isPrns(x any) bool     { return every(x, isPrn) }
func isAudience(x any) bool { return isString(x) || isStrings(x) }
func isAct(x any) bool {
	m, ok := x.(map[string]any)
	return ok && isString(m["sub"])
}

// planeConditional are the claims a token may carry, and the type each must have when present.
var planeConditional = map[string]claimCheck{
	"act": isAct, "email": isString, "email_verified": isBool, "scope": isString, "amr": isStrings,
	"orgRole": isString, "client_id": isString, "azp": isString, "sid": isString,
}

// required reads a required claim: absent is missing_claim, present and failing is bad_claim.
func required(c map[string]any, name string, ok claimCheck) (any, error) {
	x, present := c[name]
	if !present {
		return nil, refuse("missing_claim", name)
	}
	if !ok(x) {
		return nil, refuse("bad_claim", name)
	}
	return x, nil
}

func (v *Verifier) planeClaims(c map[string]any) (Principal, error) {
	now := float64(v.Now().Unix())
	iss, err := required(c, "iss", isString)
	if err != nil {
		return Principal{}, err
	}
	if iss != v.issuer {
		return Principal{}, refuse("wrong_issuer", "")
	}
	aud, err := required(c, "aud", isAudience)
	if err != nil {
		return Principal{}, err
	}
	if s, ok := aud.(string); ok && s != v.audience || !ok && !slices.Contains(aud.([]any), any(v.audience)) {
		return Principal{}, refuse("wrong_audience", "")
	}
	exp, err := required(c, "exp", isNumber)
	if err != nil {
		return Principal{}, err
	}
	iat, err := required(c, "iat", isNumber)
	if err != nil {
		return Principal{}, err
	}
	if now >= exp.(float64)+planeLeeway {
		return Principal{}, refuse("expired", "")
	}
	if nbf, present := c["nbf"]; present {
		if !isNumber(nbf) {
			return Principal{}, refuse("bad_claim", "nbf")
		}
		if nbf.(float64) > now+planeLeeway {
			return Principal{}, refuse("not_yet_valid", "")
		}
	}
	if iat.(float64) > now+planeLeeway {
		return Principal{}, refuse("issued_in_future", "")
	}
	if exp.(float64)-iat.(float64) > planeMaxLifetime {
		return Principal{}, refuse("lifetime_too_long", "")
	}
	got := map[string]any{}
	for _, r := range []struct {
		name string
		ok   claimCheck
	}{
		{"jti", isJTI}, {"sub", isPrn}, {"principalKind", isKind}, {"org", isOrg}, {"ent", isStrings},
		{"mayDispatch", isPrns}, {"mayGrantReach", isBool},
	} {
		if got[r.name], err = required(c, r.name, r.ok); err != nil {
			return Principal{}, err
		}
	}
	for name, ok := range planeConditional {
		if x, present := c[name]; present && !ok(x) {
			return Principal{}, refuse("bad_claim", name)
		}
	}
	org := got["org"].(string)
	if !slices.Contains(got["ent"].([]any), any(v.product)) {
		return Principal{}, &NotEnabledError{Org: org}
	}
	kind := PrincipalKind(got["principalKind"].(string))
	p := Principal{ID: got["sub"].(string), Kind: kind, Tenant: org}
	p.Email, _ = c["email"].(string)
	if kind == KindAgent || kind == KindService { // a person's scope claim is OAuth's, not a grant
		scope, _ := c["scope"].(string)
		p.Scopes = strings.Fields(scope)
	}
	if act, ok := c["act"].(map[string]any); ok {
		p.Actor = act["sub"].(string)
	}
	return p, nil
}
