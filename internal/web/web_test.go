package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	h := Handler()
	for _, path := range []string{"/", "/runs/superpipeline/brd_01/run_01"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `__sveltekit`) {
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

// The app installs to a phone's home screen. Nothing on screen breaks when this does: a manifest
// served with the wrong type, or an icon missing or of the wrong size, just means no install offer
// and a blank tile on iOS. So the served manifest, its icons and the iOS tags are checked here.
func TestInstallable(t *testing.T) {
	h := Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/manifest.webmanifest", nil))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/manifest+json" {
		t.Fatalf("/manifest.webmanifest: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var m struct {
		Name     string `json:"name"`
		StartURL string `json:"start_url"`
		Scope    string `json:"scope"`
		Display  string `json:"display"`
		Icons    []struct{ Src, Sizes, Purpose string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "SuperWitness" || m.StartURL != "/" || m.Scope != "/" || m.Display != "standalone" {
		t.Errorf("manifest: %+v", m)
	}

	// Each icon is an opaque PNG at the size it declares (an iOS icon with alpha turns black).
	icons := map[string]string{"/apple-touch-icon.png": "180x180"}
	purposes := map[string]bool{}
	for _, i := range m.Icons {
		icons[i.Src] = i.Sizes
		purposes[i.Sizes+" "+i.Purpose] = true
	}
	for _, want := range []string{"192x192 any", "512x512 any", "192x192 maskable", "512x512 maskable"} {
		if !purposes[want] {
			t.Errorf("manifest has no %s icon", want)
		}
	}
	for src, sizes := range icons {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", src, nil))
		b := rec.Body.Bytes()
		if rec.Code != 200 || len(b) < 26 || string(b[1:4]) != "PNG" {
			t.Errorf("%s: %d, not a PNG", src, rec.Code)
			continue
		}
		got := fmt.Sprintf("%dx%d", binary.BigEndian.Uint32(b[16:20]), binary.BigEndian.Uint32(b[20:24]))
		if got != sizes || b[25] == 4 || b[25] == 6 {
			t.Errorf("%s: %s colour type %d, want %s and opaque", src, got, b[25], sizes)
		}
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	for _, tag := range []string{
		`<link rel="manifest" href="/manifest.webmanifest"`,
		`<link rel="apple-touch-icon" href="/apple-touch-icon.png"`,
		`<meta name="apple-mobile-web-app-capable" content="yes"`,
		`<meta name="apple-mobile-web-app-title" content="SuperWitness"`,
	} {
		if !strings.Contains(rec.Body.String(), tag) {
			t.Errorf("index.html lacks %s", tag)
		}
	}
}

// One mark: the app embeds a copy of landing/public/favicon.svg, as does the docs site. A change
// to the mark is made there and copied to web/static/ and docs-site/; this fails until it is.
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
	for _, p := range []string{"../../web/static/favicon.svg", "../../docs-site/public/favicon.svg", "../../docs-site/src/assets/mark.svg"} {
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

func TestPolicyForHashesInlineScripts(t *testing.T) {
	html := []byte(`<html><head><script src="/a.js"></script></head><body><script>console.log(1)</script></body></html>`)
	sum := sha256.Sum256([]byte("console.log(1)"))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	got := policyFor(html)
	if !strings.Contains(got, "script-src 'self' "+want) {
		t.Fatalf("policy %q lacks %s", got, want)
	}
	directive := got[strings.Index(got, "script-src"):] // script-src is the last directive policyFor appends
	if strings.Contains(directive, "'unsafe-inline'") || strings.Contains(directive, "'unsafe-eval'") {
		t.Fatalf("script-src must stay strict: %q", directive)
	}
}

func TestPolicyForNoInlineScripts(t *testing.T) {
	got := policyFor([]byte(`<script src="/a.js"></script>`))
	if !strings.Contains(got, "script-src 'self'") || strings.Contains(got, "sha256") {
		t.Fatalf("got %q", got)
	}
}

// inlineBodies is the test's own reading of index.html, independent of web.go's.
func inlineBodies(t *testing.T) []string {
	t.Helper()
	index, err := fs.ReadFile(dist, "dist/index.html")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range regexp.MustCompile(`(?s)<script([^>]*)>(.*?)</script>`).FindAllStringSubmatch(string(index), -1) {
		if !strings.Contains(m[1], "src=") {
			out = append(out, m[2])
		}
	}
	return out
}

func TestHandlerServesHashedPolicyOnIndexAndFallback(t *testing.T) {
	h := Handler()
	bodies := inlineBodies(t)
	for _, p := range []string{"/", "/rubrics"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		csp := rec.Header().Get("Content-Security-Policy")
		if !strings.Contains(csp, "script-src 'self'") {
			t.Errorf("%s: CSP %q has no script-src 'self'", p, csp)
		}
		for _, b := range bodies {
			sum := sha256.Sum256([]byte(b))
			if h := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"; !strings.Contains(csp, h) {
				t.Errorf("%s: CSP %q lacks %s", p, csp, h)
			}
		}
	}
}

// A stray inline script would otherwise be hashed and allowed silently: the build has exactly one,
// SvelteKit's bootstrap.
func TestEmbeddedIndexHasExactlyOneInlineScript(t *testing.T) {
	if n := len(inlineBodies(t)); n != 1 {
		t.Fatalf("dist/index.html has %d inline scripts, want 1", n)
	}
}

func TestPolicyForHashesAScriptWithDataSrc(t *testing.T) {
	html := []byte(`<script data-src="/x.js">run()</script><script src="/a.js"></script>`)
	sum := sha256.Sum256([]byte("run()"))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	got := policyFor(html)
	if !strings.Contains(got, want) {
		t.Fatalf("a data-src attribute is not a src: policy %q lacks %s", got, want)
	}
	if strings.Count(got, "sha256-") != 1 {
		t.Fatalf("the external script must not be hashed: %q", got)
	}
}
