package session

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
)

// TokenSet is an issuer's answer to a token request. Refresh is "" when none was issued (always,
// from the hub). ExpiresIn is the access token's life in seconds, 0 when not given.
type TokenSet struct {
	Access    string
	Refresh   string
	ExpiresIn int
}

// Grants is the organization plane's refresh-token grant, as a session uses it.
type Grants interface {
	// Refresh spends refreshToken and returns the rotated set and the principal its access token
	// names. A refusal (the grant has ended, or the token no longer admits this product) is
	// ErrGrantRefused; anything else is the plane being unreachable.
	Refresh(ctx context.Context, refreshToken string) (auth.Principal, TokenSet, error)
	// Revoke ends the grant behind refreshToken at the plane (RFC 7009).
	Revoke(ctx context.Context, refreshToken string) error
}

// ErrGrantRefused is the plane refusing a refresh: the session ends.
var ErrGrantRefused = errors.New("the issuer refused the refresh token")

// PlaneGrants refreshes and revokes against the organization plane as a public client. Every
// refresh carries resource=<SW_PUBLIC_URL>, like the code exchange.
type PlaneGrants struct {
	Endpoints PlaneEndpoints
	ClientID  string
	Tokens    auth.Authenticator // checks each refreshed access token
	HTTP      *http.Client

	mu        sync.Mutex
	revokeURL string // from discovery, once found
}

func (g *PlaneGrants) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return http.DefaultClient
}

func (g *PlaneGrants) Refresh(ctx context.Context, refreshToken string) (auth.Principal, TokenSet, error) {
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken},
		"client_id": {g.ClientID}, "resource": {g.Endpoints.Resource}}
	status, body, err := postForm(ctx, g.client(), g.Endpoints.URL+"/api/auth/oauth2/token", form)
	switch {
	case err != nil:
		return auth.Principal{}, TokenSet{}, err
	case status == http.StatusBadRequest || status == http.StatusUnauthorized:
		return auth.Principal{}, TokenSet{}, ErrGrantRefused
	case status != http.StatusOK:
		return auth.Principal{}, TokenSet{}, fmt.Errorf("%w: HTTP %d", errIssuerUnavailable, status)
	}
	set, err := decodeTokenSet(body)
	if err != nil {
		return auth.Principal{}, TokenSet{}, fmt.Errorf("%w: %v", errIssuerUnavailable, err)
	}
	p, err := g.Tokens.Verify(ctx, set.Access)
	var ne *auth.NotEnabledError
	switch {
	case errors.As(err, &ne):
		return auth.Principal{}, TokenSet{}, ErrGrantRefused
	case err != nil:
		// The answer came from the plane over its own connection; a token that does not verify
		// is most likely a key that could not be fetched, so it is not taken as a refusal.
		return auth.Principal{}, TokenSet{}, fmt.Errorf("%w: the refreshed token did not verify", errIssuerUnavailable)
	}
	return p, set, nil
}

func (g *PlaneGrants) Revoke(ctx context.Context, refreshToken string) error {
	u, err := g.revocationEndpoint(ctx)
	if err != nil {
		return err
	}
	form := url.Values{"token": {refreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {g.ClientID}}
	status, _, err := postForm(ctx, g.client(), u, form)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("revocation answered HTTP %d", status)
	}
	return nil
}

// revocationEndpoint is the plane's RFC 8414 metadata's revocation_endpoint, which must be on the
// plane's own origin: the refresh token is never sent anywhere else.
func (g *PlaneGrants) revocationEndpoint(ctx context.Context) (string, error) {
	g.mu.Lock()
	cached := g.revokeURL
	g.mu.Unlock()
	if cached != "" {
		return cached, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.Endpoints.URL+"/.well-known/oauth-authorization-server", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := g.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("discovery: %w", err)
	}
	defer resp.Body.Close()
	var meta struct {
		RevocationEndpoint string `json:"revocation_endpoint"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&meta) != nil {
		return "", fmt.Errorf("discovery answered HTTP %d", resp.StatusCode)
	}
	want, err1 := auth.PublicOrigin(g.Endpoints.URL)
	got, err2 := auth.PublicOrigin(meta.RevocationEndpoint)
	if meta.RevocationEndpoint == "" || err1 != nil || err2 != nil || got != want {
		return "", errors.New("discovery names no revocation endpoint on the plane's origin")
	}
	g.mu.Lock()
	g.revokeURL = meta.RevocationEndpoint
	g.mu.Unlock()
	return meta.RevocationEndpoint, nil
}

// postForm POSTs a form and returns the status and up to 64 KiB of the body. A transport error is
// errIssuerUnavailable.
func postForm(ctx context.Context, hc *http.Client, u string, form url.Values) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", errIssuerUnavailable, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", errIssuerUnavailable, err)
	}
	return resp.StatusCode, b, nil
}

// decodeTokenSet reads an OAuth token answer: a Bearer access token, and optionally a refresh
// token and expires_in.
func decodeTokenSet(b []byte) (TokenSet, error) {
	var out struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return TokenSet{}, err
	}
	if out.AccessToken == "" || !strings.EqualFold(out.TokenType, "Bearer") {
		return TokenSet{}, errors.New("no bearer access token")
	}
	return TokenSet{Access: out.AccessToken, Refresh: out.RefreshToken, ExpiresIn: out.ExpiresIn}, nil
}

// accessLife is how long an access token from the plane is taken to last: expires_in, at most
// AccessLife.
func accessLife(expiresIn int) time.Duration {
	d := time.Duration(expiresIn) * time.Second
	if d <= 0 || d > AccessLife {
		return AccessLife
	}
	return d
}

// refreshKey derives the key that seals a session's refresh token from the session cookie's 32
// random bytes. The store holds only the cookie's sha256, so a copy of the store cannot open it.
func refreshKey(token string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("not a session token")
	}
	return hkdf.Key(sha256.New, raw, nil, "superwitness refresh token v1", 32)
}

func refreshAEAD(token string) (cipher.AEAD, error) {
	key, err := refreshKey(token)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// sealRefresh encrypts refreshToken with AES-256-GCM under the cookie's key, bound to the row's
// id: nonce || ciphertext.
func sealRefresh(token string, id []byte, refreshToken string) ([]byte, error) {
	aead, err := refreshAEAD(token)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, []byte(refreshToken), id), nil
}

func openRefresh(token string, id, sealed []byte) (string, error) {
	aead, err := refreshAEAD(token)
	if err != nil {
		return "", err
	}
	if len(sealed) < aead.NonceSize() {
		return "", errors.New("no sealed refresh token")
	}
	b, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], id)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
