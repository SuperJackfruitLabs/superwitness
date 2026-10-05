package runs

import (
	"context"
	"errors"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

func TestMemStoreContract(t *testing.T) {
	vs := verdicts.NewMemStore()
	storeContract(t, NewMemStore(vs), func(v verdicts.Verdict) {
		if _, _, err := vs.Insert(context.Background(), v); err != nil {
			t.Fatal(err)
		}
	})
}

func TestMemStoreFail(t *testing.T) {
	s := NewMemStore(nil)
	s.Fail = ErrUnavailable
	if _, err := s.Upsert(context.Background(), []Run{mk("a", "running", 0, t0)}, t0); !errors.Is(err, ErrUnavailable) {
		t.Errorf("upsert: %v", err)
	}
	if _, err := s.List(context.Background(), Filter{Limit: 1}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("list: %v", err)
	}
}
