package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// TokenSource hands out superwitness's short-lived service token. Under the hub, the hub's
// client-credentials exchange mints one token for every source (its aud lists the hub and
// superpipeline). Under the organization plane, each source has its own, for its own audience.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
	// Invalidate drops the cached token after a source answered 401/403 with it.
	Invalidate()
}

// ErrTokenRejected means the issuer (the hub or the organization plane) refused the credential:
// unknown, revoked, or suspended.
var ErrTokenRejected = errors.New("the issuer refused the service credential")

const refreshMargin = 30 * time.Second

// HubTokenSource implements the hub's service-token exchange: POST {hub}/api/auth/service-token with
// Authorization: Bearer <svc_id>:<secret>, answered by {"token","expiresIn"}.
type HubTokenSource struct {
	URL          string
	CredentialID string
	Secret       string
	HC           *http.Client
	Now          func() time.Time

	cache tokenCache
}

func NewHubTokenSource(hubURL, credentialID, secret string, hc *http.Client) *HubTokenSource {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &HubTokenSource{URL: strings.TrimRight(hubURL, "/") + "/api/auth/service-token",
		CredentialID: credentialID, Secret: secret, HC: hc, Now: time.Now}
}

func (s *HubTokenSource) Token(ctx context.Context) (string, error) {
	return s.cache.get(ctx, s.Now, s.exchange)
}

func (s *HubTokenSource) Invalidate() { s.cache.invalidate() }

func (s *HubTokenSource) exchange(ctx context.Context) (string, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+s.CredentialID+":"+s.Secret)
	req.Header.Set("Accept", "application/json")
	resp, err := s.HC.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("service token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", 0, fmt.Errorf("service token: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusBadRequest, resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return "", 0, fmt.Errorf("%w: HTTP %d", ErrTokenRejected, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return "", 0, fmt.Errorf("service token: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Token     string `json:"token"`
		ExpiresIn int64  `json:"expiresIn"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Token == "" || out.ExpiresIn <= 0 {
		return "", 0, errors.New("service token: malformed response")
	}
	return out.Token, time.Duration(out.ExpiresIn) * time.Second, nil
}

// tokenCache holds one short-lived token and coalesces concurrent refreshes into one exchange.
type tokenCache struct {
	mu      sync.Mutex
	value   string
	expires time.Time
	group   singleflight.Group
}

func (c *tokenCache) get(ctx context.Context, now func() time.Time,
	exchange func(context.Context) (string, time.Duration, error)) (string, error) {
	if v, ok := c.cached(now()); ok {
		return v, nil
	}
	r, err, _ := c.group.Do("token", func() (any, error) {
		if v, ok := c.cached(now()); ok { // a caller queued behind a finished exchange
			return v, nil
		}
		tok, ttl, err := exchange(ctx)
		if err != nil {
			return "", err
		}
		c.mu.Lock()
		c.value, c.expires = tok, now().Add(ttl)
		c.mu.Unlock()
		return tok, nil
	})
	if err != nil {
		return "", err
	}
	return r.(string), nil
}

// cached returns the held token while it is more than refreshMargin from expiry.
func (c *tokenCache) cached(now time.Time) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.value != "" && now.Add(refreshMargin).Before(c.expires) {
		return c.value, true
	}
	return "", false
}

func (c *tokenCache) invalidate() {
	c.mu.Lock()
	c.value, c.expires = "", time.Time{}
	c.mu.Unlock()
}

// PlaneTokenSource exchanges superwitness's svc_ credential at the organization plane for a token
// minted for one audience: POST {plane}/api/token/service {"audience": …} with Authorization:
// Bearer svc_…:<secret>, answered by {"access_token","token_type":"Bearer","expires_in"}. Each
// product superwitness calls gets its own source, because a plane token names one audience.
type PlaneTokenSource struct {
	URL        string
	Credential string // svc_<id>:<secret>
	Audience   string
	HC         *http.Client
	Now        func() time.Time

	cache tokenCache
}

func NewPlaneTokenSource(planeURL, credential, audience string, hc *http.Client) *PlaneTokenSource {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &PlaneTokenSource{URL: strings.TrimRight(planeURL, "/") + "/api/token/service",
		Credential: credential, Audience: audience, HC: hc, Now: time.Now}
}

func (s *PlaneTokenSource) Token(ctx context.Context) (string, error) {
	return s.cache.get(ctx, s.Now, s.exchange)
}

func (s *PlaneTokenSource) Invalidate() { s.cache.invalidate() }

func (s *PlaneTokenSource) exchange(ctx context.Context) (string, time.Duration, error) {
	body, _ := json.Marshal(map[string]string{"audience": s.Audience})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+s.Credential)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.HC.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("plane service token: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", 0, fmt.Errorf("plane service token: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusLocked:
		return "", 0, fmt.Errorf("%w: organization plane HTTP %d", ErrTokenRejected, resp.StatusCode)
	default:
		return "", 0, fmt.Errorf("plane service token: HTTP %d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if json.Unmarshal(raw, &out) != nil || out.AccessToken == "" || !strings.EqualFold(out.TokenType, "Bearer") || out.ExpiresIn <= 0 {
		return "", 0, errors.New("plane service token: malformed response")
	}
	return out.AccessToken, time.Duration(out.ExpiresIn) * time.Second, nil
}

var serviceCredential = regexp.MustCompile(`^svc_[A-Za-z0-9]+:\S+$`)

// ParseServiceCredential checks a plane service credential, svc_<id>:<secret>. Its error never
// carries the input, which holds the secret.
func ParseServiceCredential(s string) (string, error) {
	if !serviceCredential.MatchString(s) {
		return "", errors.New("want one line of the form svc_<id>:<secret>")
	}
	return s, nil
}

// ReadServiceCredential reads and checks a plane service credential file (see ReadSecretFile).
func ReadServiceCredential(path string) (string, error) {
	s, err := ReadSecretFile(path)
	if err != nil {
		return "", err
	}
	if _, err := ParseServiceCredential(s); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// ReadSecretFile reads a secret, trimming whitespace, and refuses files others can read.
func ReadSecretFile(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("%s: must not be readable by group or others (mode %v)", path, fi.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", fmt.Errorf("%s: empty", path)
	}
	return s, nil
}
