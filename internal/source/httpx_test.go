package source

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
)

func TestGetJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(401)
				return
			}
			fmt.Fprint(w, `{"a":"b"}`)
		case "/denied":
			w.WriteHeader(403)
		case "/gone":
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"not_found"}`)
		case "/nowhere":
			w.WriteHeader(404)
			fmt.Fprint(w, `404 page not found`)
		case "/broken":
			fmt.Fprint(w, `{"a":`)
		case "/slow":
			time.Sleep(200 * time.Millisecond)
			fmt.Fprint(w, `{}`)
		default:
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	notFound := func(b []byte) bool { return string(b) == `{"error":"not_found"}` }

	var out map[string]string
	if code, st := GetJSON(context.Background(), srv.Client(), srv.URL+"/ok", "tok", &out, notFound); st != StatusOK || code != 200 || out["a"] != "b" {
		t.Errorf("ok: %d %s %v", code, st, out)
	}
	cases := map[string]SourceStatus{
		"/denied": StatusUnauthorized, "/gone": StatusNotFound, "/nowhere": StatusUnavailable,
		"/broken": StatusUnavailable, "/boom": StatusUnavailable,
	}
	for path, want := range cases {
		if _, st := GetJSON(context.Background(), srv.Client(), srv.URL+path, "tok", &out, notFound); st != want {
			t.Errorf("%s: %s, want %s", path, st, want)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, st := GetJSON(ctx, srv.Client(), srv.URL+"/slow", "", &out, nil); st != StatusTimeout {
		t.Errorf("slow: %s", st)
	}
}

func TestTokenStatus(t *testing.T) {
	if TokenStatus(context.Background(), fmt.Errorf("x: %w", auth.ErrTokenRejected)) != StatusUnauthorized {
		t.Error("rejected token should be unauthorized")
	}
	if TokenStatus(context.Background(), errors.New("dial tcp")) != StatusUnavailable {
		t.Error("transport failure should be unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	if TokenStatus(ctx, ctx.Err()) != StatusTimeout {
		t.Error("deadline should be timeout")
	}
}
