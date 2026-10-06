package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	jwksTTL     = 10 * time.Minute
	minMissGap  = 10 * time.Second
	clockLeeway = 30 * time.Second
)

// Product is superwitness's name in the organization plane's ent claim.
const Product = "superwitness"

var planeSubject = regexp.MustCompile(`^prn_[A-Za-z0-9_-]{1,64}$`)

// NotEnabledError is a valid plane token whose workspace has not enabled superwitness. It
// unwraps to ErrUnauthenticated, so a caller that only checks err != nil still refuses it.
type NotEnabledError struct{ Org string }

func (e *NotEnabledError) Error() string { return "superwitness is not enabled for " + e.Org }
func (e *NotEnabledError) Unwrap() error { return ErrUnauthenticated }

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	errUnknownKid      = errors.New("unknown signing key")
)

type Authenticator interface {
	Verify(ctx context.Context, token string) (Principal, error)
}

// call is one in-flight JWKS fetch; ns and err are valid once done is closed.
type call struct {
	done chan struct{}
	ns   *keySet
	err  error
}

type keySet struct {
	keys      map[string]ed25519.PublicKey
	fetchedAt time.Time
}

// Verifier checks JWTs offline against one issuer's JWKS: the hub's, or the organization
// plane's (NewPlaneVerifier). No request is made to the issuer to verify a token.
type Verifier struct {
	issuer   string
	audience string
	jwksURL  string
	plane    bool
	hc       *http.Client
	Now      func() time.Time

	mu       sync.Mutex
	set      *keySet
	lastMiss time.Time // last unknown-kid refetch against a fresh set
	lastTry  time.Time // last periodic refresh attempt (success or failure)
	inflight *call
}

func NewVerifier(issuer, audience string, hc *http.Client) *Verifier {
	issuer = strings.TrimRight(issuer, "/")
	return newVerifier(issuer, issuer+"/api/auth/jwks", audience, false, hc)
}

// NewPlaneVerifier checks organization-plane tokens offline. iss must equal issuer exactly (it
// is never trimmed or prefix-matched), keys come from jwksURL, aud must equal or contain
// audience, sub must be a prn_ id, and org and ent must be present. ent without Product is a
// NotEnabledError. Grant scopes are read only from agent and service tokens.
func NewPlaneVerifier(issuer, jwksURL, audience string, hc *http.Client) *Verifier {
	return newVerifier(issuer, jwksURL, audience, true, hc)
}

func newVerifier(issuer, jwksURL, audience string, plane bool, hc *http.Client) *Verifier {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Verifier{issuer: issuer, jwksURL: jwksURL, audience: audience, plane: plane, hc: hc, Now: time.Now}
}

type hubClaims struct {
	jwt.RegisteredClaims
	PrincipalKind string `json:"principalKind"`
	Tenant        string `json:"tenant"`
	Scope         string `json:"scope"` // OAuth's space-delimited list; absent when the grant holds none
	Email         string `json:"email"` // present only for a principal with a linked account
	Act           *struct {
		Sub string `json:"sub"`
	} `json:"act,omitempty"`
}

type planeClaims struct {
	jwt.RegisteredClaims
	PrincipalKind string   `json:"principalKind"`
	Org           string   `json:"org"`
	Ent           []string `json:"ent"`   // nil when absent; [] when present and empty
	Scope         string   `json:"scope"` // grant scopes on agent and service tokens; OAuth scopes on a person's
	Email         string   `json:"email"`
	Act           *struct {
		Sub string `json:"sub"`
	} `json:"act,omitempty"`
}

func (v *Verifier) Verify(ctx context.Context, raw string) (Principal, error) {
	kid, ok := headerKid(raw)
	if !ok {
		return Principal{}, ErrUnauthenticated
	}
	key, err := v.key(ctx, kid)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	if v.plane {
		return v.verifyPlane(raw, key)
	}
	return v.verifyHub(raw, key)
}

func (v *Verifier) parse(raw string, key ed25519.PublicKey, c jwt.Claims) error {
	_, err := jwt.ParseWithClaims(raw, c, func(*jwt.Token) (any, error) { return key, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(clockLeeway),
		jwt.WithTimeFunc(v.Now),
	)
	return err
}

func validKind(k PrincipalKind) bool { return k == KindHuman || k == KindAgent || k == KindService }

func (v *Verifier) verifyHub(raw string, key ed25519.PublicKey) (Principal, error) {
	var c hubClaims
	if err := v.parse(raw, key, &c); err != nil || c.IssuedAt == nil || c.Subject == "" || c.Tenant == "" {
		return Principal{}, ErrUnauthenticated
	}
	kind := PrincipalKind(c.PrincipalKind)
	if !validKind(kind) {
		return Principal{}, ErrUnauthenticated
	}
	p := Principal{ID: c.Subject, Kind: kind, Tenant: c.Tenant, Scopes: strings.Fields(c.Scope), Email: c.Email}
	if c.Act != nil {
		p.Actor = c.Act.Sub
	}
	return p, nil
}

func (v *Verifier) verifyPlane(raw string, key ed25519.PublicKey) (Principal, error) {
	var c planeClaims
	if err := v.parse(raw, key, &c); err != nil || c.IssuedAt == nil || !planeSubject.MatchString(c.Subject) ||
		!strings.HasPrefix(c.Org, "org_") || len(c.Org) == len("org_") || c.Ent == nil {
		return Principal{}, ErrUnauthenticated
	}
	kind := PrincipalKind(c.PrincipalKind)
	if !validKind(kind) {
		return Principal{}, ErrUnauthenticated
	}
	if !slices.Contains(c.Ent, Product) {
		return Principal{}, &NotEnabledError{Org: c.Org}
	}
	p := Principal{ID: c.Subject, Kind: kind, Tenant: c.Org, Email: c.Email}
	if kind == KindAgent || kind == KindService { // a person's scope claim is OAuth's, not a grant
		p.Scopes = strings.Fields(c.Scope)
	}
	if c.Act != nil {
		p.Actor = c.Act.Sub
	}
	return p, nil
}

// headerKid rejects a non-EdDSA alg or a missing kid before any key lookup, so junk
// headers never reach the network.
func headerKid(raw string) (string, bool) {
	h, _, ok := strings.Cut(raw, ".")
	if !ok {
		return "", false
	}
	b, err := base64.RawURLEncoding.DecodeString(h)
	if err != nil {
		return "", false
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid any    `json:"kid"`
	}
	if json.Unmarshal(b, &hdr) != nil || hdr.Alg != "EdDSA" {
		return "", false
	}
	kid, ok := hdr.Kid.(string)
	return kid, ok && kid != ""
}

func (v *Verifier) key(ctx context.Context, kid string) (ed25519.PublicKey, error) {
	now := v.Now()
	v.mu.Lock()
	set := v.set
	fresh := set != nil && now.Sub(set.fetchedAt) < jwksTTL
	if fresh {
		if k, ok := set.keys[kid]; ok {
			v.mu.Unlock()
			return k, nil
		}
	}
	// A fetch already in flight is joined, never failed against: its result answers this
	// caller too. Otherwise: either the cache is missing or expired (periodic refresh), or
	// the set is fresh and the kid unseen (rotation or junk). Both are one fetch per
	// minMissGap; the compare and the stamp share this critical section, and a failed
	// attempt counts, so a dead hub is not hammered. Only a completed attempt inside the gap
	// blocks: known kids are then served from the last good set, unknown ones fail closed.
	c := v.inflight
	if c == nil {
		last := &v.lastTry
		if fresh {
			last = &v.lastMiss
		}
		if !last.IsZero() && now.Sub(*last) < minMissGap {
			v.mu.Unlock()
			return staleKey(set, kid)
		}
		*last = now
		c = &call{done: make(chan struct{})}
		v.inflight = c
		v.mu.Unlock()
		v.fetchInto(c)
	} else {
		v.mu.Unlock()
	}
	select {
	case <-c.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if c.err != nil {
		if fresh {
			return nil, c.err
		}
		if k, serr := staleKey(set, kid); serr == nil { // issuer outage: last good set
			return k, nil
		}
		return nil, c.err
	}
	if k, ok := c.ns.keys[kid]; ok {
		return k, nil
	}
	return nil, errUnknownKid
}

func staleKey(set *keySet, kid string) (ed25519.PublicKey, error) {
	if set != nil {
		if k, ok := set.keys[kid]; ok {
			return k, nil
		}
	}
	return nil, errUnknownKid
}

// fetchInto runs the one JWKS fetch for c and publishes the result to every waiter. It is
// bounded by its own 5 s timeout, so a caller disconnecting cannot fail the others.
func (v *Verifier) fetchInto(c *call) {
	c.ns, c.err = v.fetch()
	v.mu.Lock()
	if c.err == nil {
		v.set = c.ns
	}
	v.inflight = nil
	v.mu.Unlock()
	close(c.done)
}

func (v *Verifier) fetch() (*keySet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := v.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks: HTTP %d", resp.StatusCode)
	}
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Kid string `json:"kid"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	keys := map[string]ed25519.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "OKP" || k.Crv != "Ed25519" || k.Kid == "" {
			continue
		}
		x, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil || len(x) != ed25519.PublicKeySize {
			continue
		}
		keys[k.Kid] = ed25519.PublicKey(x)
	}
	if len(keys) == 0 {
		return nil, errors.New("jwks: no Ed25519 keys")
	}
	return &keySet{keys: keys, fetchedAt: v.Now()}, nil
}
