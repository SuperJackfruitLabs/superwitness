package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tokenServer mimics the hub's service-token exchange: POST /api/auth/service-token, Authorization: Bearer <svc_id>:<secret>.
func tokenServer(t *testing.T, hits *atomic.Int32, status int, expiresIn int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/auth/service-token" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer svc_0123:s3cret" {
			t.Errorf("Authorization = %q", got)
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"tok-` + strconv.Itoa(int(n)) + `","expiresIn":` + strconv.Itoa(expiresIn) + `}`))
	}))
}

func TestHubTokenSourceCaches(t *testing.T) {
	var hits atomic.Int32
	srv := tokenServer(t, &hits, http.StatusOK, 300)
	defer srv.Close()
	ts := NewHubTokenSource(srv.URL, "svc_0123", "s3cret", srv.Client())
	a, err := ts.Token(context.Background())
	if err != nil || a != "tok-1" {
		t.Fatalf("token = %q, %v", a, err)
	}
	b, _ := ts.Token(context.Background())
	if b != "tok-1" || hits.Load() != 1 {
		t.Errorf("second call = %q, hits = %d", b, hits.Load())
	}
}

func TestHubTokenSourceRefreshesBeforeExpiry(t *testing.T) {
	var hits atomic.Int32
	srv := tokenServer(t, &hits, http.StatusOK, 120)
	defer srv.Close()
	now := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	ts := NewHubTokenSource(srv.URL, "svc_0123", "s3cret", srv.Client())
	ts.Now = func() time.Time { return now }

	_, _ = ts.Token(context.Background())
	now = now.Add(89 * time.Second) // 31 s left: still cached
	_, _ = ts.Token(context.Background())
	now = now.Add(2 * time.Second) // 29 s left: inside the 30 s margin
	_, _ = ts.Token(context.Background())
	if hits.Load() != 2 {
		t.Errorf("hits = %d, want 2", hits.Load())
	}
}

func TestHubTokenSourceRejected(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		var hits atomic.Int32
		srv := tokenServer(t, &hits, code, 0)
		ts := NewHubTokenSource(srv.URL, "svc_0123", "s3cret", srv.Client())
		if _, err := ts.Token(context.Background()); !errors.Is(err, ErrTokenRejected) {
			t.Errorf("HTTP %d: err = %v, want ErrTokenRejected", code, err)
		}
		srv.Close()
	}
	var hits atomic.Int32
	srv := tokenServer(t, &hits, http.StatusBadGateway, 0)
	defer srv.Close()
	ts := NewHubTokenSource(srv.URL, "svc_0123", "s3cret", srv.Client())
	if _, err := ts.Token(context.Background()); err == nil || errors.Is(err, ErrTokenRejected) {
		t.Errorf("502: err = %v, want a transient error", err)
	}
}

func TestHubTokenSourceCoalescesConcurrentCalls(t *testing.T) {
	var hits atomic.Int32
	srv := tokenServer(t, &hits, http.StatusOK, 300)
	defer srv.Close()
	ts := NewHubTokenSource(srv.URL, "svc_0123", "s3cret", srv.Client())
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { _, _ = ts.Token(context.Background()) })
	}
	wg.Wait()
	if hits.Load() != 1 {
		t.Errorf("hits = %d, want 1", hits.Load())
	}
}

func TestHubTokenSourceInvalidate(t *testing.T) {
	var hits atomic.Int32
	srv := tokenServer(t, &hits, http.StatusOK, 300)
	defer srv.Close()
	ts := NewHubTokenSource(srv.URL, "svc_0123", "s3cret", srv.Client())
	_, _ = ts.Token(context.Background())
	ts.Invalidate()
	if tok, _ := ts.Token(context.Background()); tok != "tok-2" {
		t.Errorf("after invalidate = %q", tok)
	}
}

func TestReadSecretFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good")
	if err := os.WriteFile(good, []byte("  s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s, err := ReadSecretFile(good); err != nil || s != "s3cret" {
		t.Errorf("got %q, %v", s, err)
	}
	open := filepath.Join(dir, "open")
	_ = os.WriteFile(open, []byte("s3cret"), 0o644)
	if _, err := ReadSecretFile(open); err == nil {
		t.Error("want refusal of a group/world-readable secret")
	}
	empty := filepath.Join(dir, "empty")
	_ = os.WriteFile(empty, []byte("\n"), 0o600)
	if _, err := ReadSecretFile(empty); err == nil {
		t.Error("want refusal of an empty secret")
	}
}
