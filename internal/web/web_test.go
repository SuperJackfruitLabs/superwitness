package web

import (
	"bytes"
	"io/fs"
	"net/http/httptest"
	"os"
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

// The app's tab shows the product mark: served at /favicon.svg, linked from index.html, and
// /favicon.ico (which browsers ask for unprompted) sends them there rather than a 404.
func TestFavicon(t *testing.T) {
	h := Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/favicon.svg", nil))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("/favicon.svg: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "<svg") {
		t.Errorf("/favicon.svg is not an SVG")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec.Body.String(), `<link rel="icon" type="image/svg+xml" href="/favicon.svg"`) {
		t.Errorf("index.html does not link /favicon.svg")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/favicon.ico", nil))
	if rec.Code != 301 || rec.Header().Get("Location") != "/favicon.svg" {
		t.Errorf("/favicon.ico: %d Location %q, want 301 to /favicon.svg", rec.Code, rec.Header().Get("Location"))
	}
}

// One mark: the app embeds a copy of landing/public/favicon.svg, as does the docs site. A change
// to the mark is made there and copied to web/public/ and docs-site/; this fails until it is.
func TestFaviconMatchesTheMark(t *testing.T) {
	mark, err := os.ReadFile("../../landing/public/favicon.svg")
	if err != nil {
		t.Fatal(err)
	}
	embedded, err := fs.ReadFile(dist, "dist/favicon.svg")
	if err != nil {
		t.Fatal(err)
	}
	copies := map[string][]byte{"embedded dist/favicon.svg": embedded}
	for _, p := range []string{"../../web/public/favicon.svg", "../../docs-site/public/favicon.svg", "../../docs-site/src/assets/mark.svg"} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		copies[p] = b
	}
	for name, b := range copies {
		if !bytes.Equal(b, mark) {
			t.Errorf("%s differs from landing/public/favicon.svg; copy it again", name)
		}
	}
}
