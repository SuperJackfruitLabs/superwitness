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

// Verifier checks hub-issued JWTs offline against the hub's published JWKS. The hub is the
// one issuer; no request is made to it to verify a token.
type Verifier struct {
	issuer   string
	audience string
	hc       *http.Client
	Now      func() time.Time

	mu       sync.Mutex
	set      *keySet
	lastMiss time.Time // last unknown-kid refetch against a fresh set
	lastTry  time.Time // last periodic refresh attempt (success or failure)
	inflight *call
}

func NewVerifier(issuer, audience string, hc *http.Client) *Verifier {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Verifier{issuer: strings.TrimRight(issuer, "/"), audience: audience, hc: hc, Now: time.Now}
}

type hubClaims struct {
	jwt.RegisteredClaims
	PrincipalKind string `json:"principalKind"`
	Tenant        string `json:"tenant"`
	Scope         string `json:"scope"` // OAuth's space-delimited list; absent when the grant holds none
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
	var c hubClaims
	_, err = jwt.ParseWithClaims(raw, &c, func(*jwt.Token) (any, error) { return key, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(clockLeeway),
		jwt.WithTimeFunc(v.Now),
	)
	if err != nil || c.IssuedAt == nil || c.Subject == "" || c.Tenant == "" {
		return Principal{}, ErrUnauthenticated
	}
	kind := PrincipalKind(c.PrincipalKind)
	if kind != KindHuman && kind != KindAgent && kind != KindService {
		return Principal{}, ErrUnauthenticated
	}
	p := Principal{ID: c.Subject, Kind: kind, Tenant: c.Tenant, Scopes: strings.Fields(c.Scope)}
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.issuer+"/api/auth/jwks", nil)
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
