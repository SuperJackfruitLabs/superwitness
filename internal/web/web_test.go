package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	h := Handler()
	for _, path := range []string{"/", "/runs/superpipeline/brd_01/run_01"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `<div id="root">`) {
			t.Errorf("%s: %d", path, rec.Code)
		}
		for _, want := range []string{"default-src 'self'", "frame-ancestors 'none'"} {
			if !strings.Contains(rec.Header().Get("Content-Security-Policy"), want) {
				t.Errorf("%s: CSP = %q, want %s", path, rec.Header().Get("Content-Security-Policy"), want)
			}
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/assets/missing.js", nil))
	if rec.Code != 404 {
		t.Errorf("missing asset: %d", rec.Code)
	}
}
