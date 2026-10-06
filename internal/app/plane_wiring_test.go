package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/config"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

type seen struct {
	mu   sync.Mutex
	auth map[string][]string // path → Authorization headers
}

func (s *seen) record(r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auth[r.URL.Path] = append(s.auth[r.URL.Path], r.Header.Get("Authorization"))
}

func planeWiringRig(t *testing.T) (config.Config, *seen) {
	t.Helper()
	s := &seen{auth: map[string][]string{}}
	plane := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.record(r)
		var body struct {
			Audience string `json:"audience"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok:" + body.Audience, "token_type": "Bearer", "expires_in": 300})
	}))
	t.Cleanup(plane.Close)
	product := func() *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.record(r)
			w.WriteHeader(http.StatusUnauthorized)
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	hub, sp := product(), product()
	cred := filepath.Join(t.TempDir(), "cred")
	if err := os.WriteFile(cred, []byte("svc_01:s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return config.Config{PublicURL: "https://superwitness.example", HubURL: hub.URL, SuperpipelineURL: sp.URL,
		TracesURL: "http://127.0.0.1:1", LogsURL: "http://127.0.0.1:1",
		OrgPlaneIssuer: "https://accounts.example", OrgPlaneJWKSURL: plane.URL + "/api/auth/jwks", OrgPlaneURL: plane.URL,
		OrgPlaneCredentialFile: cred}, s
}

func TestPlaneWiringAsksForOneTokenPerAudience(t *testing.T) {
	cfg, s := planeWiringRig(t)
	w, err := realWiring(cfg, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	w.attempts.ResolveAttempt(ctx, "attempt_01")
	w.sp.Fetch(ctx, source.RunRef{Source: "superpipeline", BoardID: "brd_01", RunID: "run_01"})

	s.mu.Lock()
	defer s.mu.Unlock()
	if got := s.auth["/api/evidence/attempts/attempt_01"]; len(got) != 1 || got[0] != "Bearer tok:"+cfg.HubURL {
		t.Errorf("hub saw %q; want its own audience's token", got)
	}
	if got := s.auth["/v1/boards/brd_01/runs/run_01/evidence"]; len(got) != 1 || got[0] != "Bearer tok:"+cfg.SuperpipelineURL {
		t.Errorf("superpipeline saw %q; want its own audience's token", got)
	}
	if got := s.auth["/api/token/service"]; len(got) != 2 || got[0] != "Bearer svc_01:s3cret" {
		t.Errorf("plane exchanges %q; want two, with the svc_ credential", got)
	}
	if _, ok := s.auth["/api/auth/service-token"]; ok {
		t.Error("the hub's own service-token route was called under the plane")
	}
	if _, ok := w.authn.(*auth.Verifier); !ok {
		t.Errorf("authn = %T; want the plane verifier", w.authn)
	}
}

func TestPlaneWiringRefusesABadCredentialWithoutShowingIt(t *testing.T) {
	cfg, _ := planeWiringRig(t)
	_ = os.WriteFile(cfg.OrgPlaneCredentialFile, []byte("s3cret-without-an-id\n"), 0o600)
	_, err := realWiring(cfg, http.DefaultClient)
	if err == nil || !strings.Contains(err.Error(), "SW_ORG_PLANE_SERVICE_CREDENTIAL_FILE") || strings.Contains(err.Error(), "s3cret") {
		t.Errorf("err = %v", err)
	}
}
