package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// TokenSource hands out superwitness's short-lived service token, minted by the hub's
// client-credentials exchange. One token
// serves every source: its aud lists the hub and superpipeline.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
	// Invalidate drops the cached token after a source answered 401/403 with it.
	Invalidate()
}

// ErrTokenRejected means the hub refused the credential: unknown, revoked, or suspended.
var ErrTokenRejected = errors.New("hub refused the service credential")

const refreshMargin = 30 * time.Second

// HubTokenSource implements the hub's service-token exchange: POST {hub}/api/auth/service-token with
// Authorization: Bearer <svc_id>:<secret>, answered by {"token","expiresIn"}.
type HubTokenSource struct {
	URL          string
	CredentialID string
	Secret       string
	HC           *http.Client
	Now          func() time.Time

	mu      sync.Mutex
	value   string
	expires time.Time
	group   singleflight.Group
}

func NewHubTokenSource(hubURL, credentialID, secret string, hc *http.Client) *HubTokenSource {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &HubTokenSource{URL: strings.TrimRight(hubURL, "/") + "/api/auth/service-token",
		CredentialID: credentialID, Secret: secret, HC: hc, Now: time.Now}
}

func (s *HubTokenSource) Token(ctx context.Context) (string, error) {
	if v, ok := s.cached(); ok {
		return v, nil
	}
	r, err, _ := s.group.Do("token", func() (any, error) {
		// A caller that queued behind a finished exchange finds the fresh token here.
		if v, ok := s.cached(); ok {
			return v, nil
		}
		return s.exchange(ctx)
	})
	if err != nil {
		return "", err
	}
	return r.(string), nil
}

// cached returns the held token while it is more than refreshMargin from expiry.
func (s *HubTokenSource) cached() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.value != "" && s.Now().Add(refreshMargin).Before(s.expires) {
		return s.value, true
	}
	return "", false
}

func (s *HubTokenSource) Invalidate() {
	s.mu.Lock()
	s.value, s.expires = "", time.Time{}
	s.mu.Unlock()
}

func (s *HubTokenSource) exchange(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.CredentialID+":"+s.Secret)
	req.Header.Set("Accept", "application/json")
	resp, err := s.HC.Do(req)
	if err != nil {
		return "", fmt.Errorf("service token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", fmt.Errorf("service token: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusBadRequest, resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return "", fmt.Errorf("%w: HTTP %d", ErrTokenRejected, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("service token: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Token     string `json:"token"`
		ExpiresIn int64  `json:"expiresIn"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Token == "" || out.ExpiresIn <= 0 {
		return "", errors.New("service token: malformed response")
	}
	s.mu.Lock()
	s.value, s.expires = out.Token, s.Now().Add(time.Duration(out.ExpiresIn)*time.Second)
	s.mu.Unlock()
	return out.Token, nil
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
