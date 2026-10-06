package app

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/config"
)

type countingLookup struct{ n atomic.Int32 }

func (c *countingLookup) Lookup(_ context.Context, key string) (auth.PrincipalRecord, error) {
	c.n.Add(1)
	if key == "hubuser_01" {
		return auth.PrincipalRecord{ID: "prn_human01", Kind: auth.KindHuman}, nil
	}
	return auth.PrincipalRecord{}, auth.ErrPrincipalNotFound
}

func TestBearerAuthResolvesHubSubsOnlyWithoutThePlane(t *testing.T) {
	ctx := context.Background()
	lookups := &countingLookup{}
	w := wiring{authn: auth.DevAuthenticator{}, principals: lookups}

	p, err := bearerAuth(config.Config{}, w).Verify(ctx, "dev:hubuser_01:human")
	if err != nil || p.ID != "prn_human01" {
		t.Fatalf("hub mode: %+v %v; want the account resolved to prn_human01", p, err)
	}

	lookups.n.Store(0)
	planeCfg := config.Config{OrgPlaneIssuer: "https://accounts.example"}
	p, err = bearerAuth(planeCfg, w).Verify(ctx, "dev:prn_human01:human")
	if err != nil || p.ID != "prn_human01" || lookups.n.Load() != 0 {
		t.Errorf("plane mode: %+v %v, %d lookups; want sub used as is, no hub call", p, err, lookups.n.Load())
	}
	// The plane's verifier only admits prn_ subs, but nothing under the plane may ask the hub
	// what a sub means, whatever it looks like.
	p, err = bearerAuth(planeCfg, w).Verify(ctx, "dev:hubuser_01:human")
	if err != nil || p.ID != "hubuser_01" || lookups.n.Load() != 0 {
		t.Errorf("plane mode, non-prn sub: %+v %v, %d lookups; want no hub call", p, err, lookups.n.Load())
	}
}
