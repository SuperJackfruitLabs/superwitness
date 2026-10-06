package canary

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
)

type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// ServiceToken mints the canary's short-lived hub token. It is a thin adapter
// over the service's auth.HubTokenSource, which owns the exchange, the caching and the
// singleflight: it reads the secret with auth.ReadSecretFile and builds the source
// lazily, once. The secret is read on the first Token call and not re-read; a failed
// read is retried on the next call. Errors never carry the secret or a response body.
type ServiceToken struct {
	HubURL     string
	ClientID   string
	SecretFile string
	HTTP       *http.Client
	Now        func() time.Time

	mu  sync.Mutex
	src *auth.HubTokenSource
}

func (c *ServiceToken) source() (*auth.HubTokenSource, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.src != nil {
		return c.src, nil
	}
	secret, err := auth.ReadSecretFile(c.SecretFile)
	if err != nil {
		return nil, fmt.Errorf("reading the service secret file: %w", err)
	}
	src := auth.NewHubTokenSource(c.HubURL, c.ClientID, secret, c.HTTP)
	if c.Now != nil {
		src.Now = c.Now
	}
	c.src = src
	return src, nil
}

func (c *ServiceToken) Token(ctx context.Context) (string, error) {
	src, err := c.source()
	if err != nil {
		return "", err
	}
	return src.Token(ctx)
}

// PrincipalFromJWT reads the sub claim without verifying the token. It only tells the
// canary who it is, for check (e); the servers do the verifying. The service's internal/auth has
// no unverified-claims reader, so this stays here.
func PrincipalFromJWT(token string) (string, error) {
	parts := splitJWT(token)
	if parts == nil {
		return "", errors.New("token is not a JWT")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("token payload: %w", err)
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return "", fmt.Errorf("token payload: %w", err)
	}
	if claims.Sub == "" {
		return "", errors.New("token has no sub claim")
	}
	return claims.Sub, nil
}

func splitJWT(token string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	parts = append(parts, token[start:])
	if len(parts) != 3 {
		return nil
	}
	return parts
}

// PlaneServiceToken mints the canary's token for one audience at the organization plane. It
// reads the credential file on first use, as ServiceToken does, and builds the source once.
type PlaneServiceToken struct {
	PlaneURL       string
	CredentialFile string
	Audience       string
	HTTP           *http.Client
	Now            func() time.Time

	mu  sync.Mutex
	src *auth.PlaneTokenSource
}

func (c *PlaneServiceToken) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.src == nil {
		cred, err := auth.ReadServiceCredential(c.CredentialFile)
		if err != nil {
			c.mu.Unlock()
			return "", fmt.Errorf("reading the plane service credential: %w", err)
		}
		c.src = auth.NewPlaneTokenSource(c.PlaneURL, cred, c.Audience, c.HTTP)
		if c.Now != nil {
			c.src.Now = c.Now
		}
	}
	src := c.src
	c.mu.Unlock()
	return src.Token(ctx)
}
