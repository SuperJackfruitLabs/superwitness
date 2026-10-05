package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type lookupMap map[string]PrincipalRecord

func (m lookupMap) Lookup(_ context.Context, key string) (PrincipalRecord, error) {
	if key == "hubuser_down" {
		return PrincipalRecord{}, ErrLookupUnavailable
	}
	r, ok := m[key]
	if !ok {
		return PrincipalRecord{}, ErrPrincipalNotFound
	}
	return r, nil
}

var people = lookupMap{
	"hubuser_01":   {ID: "prn_human01", Kind: KindHuman},
	"hubuser_gone": {ID: "prn_human09", Kind: KindHuman, Suspended: true},
	"hubuser_bot":  {ID: "prn_agent01", Kind: KindAgent},
}

func TestResolvingNamesEveryCallerByPrincipalID(t *testing.T) {
	a := Resolving{Inner: DevAuthenticator{}, Principals: people}
	p, err := a.Verify(context.Background(), "dev:hubuser_01:human")
	if err != nil || p.ID != "prn_human01" || p.Kind != KindHuman {
		t.Errorf("a hub account id: %+v %v; want prn_human01", p, err)
	}
	p, err = a.Verify(context.Background(), "dev:prn_agent01:agent")
	if err != nil || p.ID != "prn_agent01" {
		t.Errorf("a principal id passes through: %+v %v", p, err)
	}
	for tok, want := range map[string]error{
		"dev:hubuser_none:human": ErrPrincipalNotFound,
		"dev:hubuser_gone:human": ErrPrincipalSuspended,
		"dev:hubuser_bot:human":  ErrUnauthenticated, // the record says agent
		"dev:hubuser_down:human": ErrLookupUnavailable,
	} {
		if _, err := a.Verify(context.Background(), tok); !errors.Is(err, want) {
			t.Errorf("%s: %v, want %v", tok, err, want)
		}
	}
}

func TestMiddlewareAnswers503WhenTheHubCannotResolve(t *testing.T) {
	h := Middleware(Resolving{Inner: DevAuthenticator{}, Principals: people})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(204)
	}))
	for tok, code := range map[string]int{"dev:hubuser_down:human": 503, "dev:hubuser_gone:human": 401, "dev:hubuser_01:human": 204} {
		req := httptest.NewRequest("GET", "/v1/x", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != code {
			t.Errorf("%s: %d, want %d", tok, rec.Code, code)
		}
		if code == 503 && rec.Header().Get("Retry-After") != "5" {
			t.Errorf("503 without Retry-After: 5")
		}
	}
}
