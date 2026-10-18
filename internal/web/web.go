// Package web serves the embedded run page built from /web (make web).
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// CSP is the Content-Security-Policy every page superwitness serves carries; session's sign-in
// pages reuse it.
const CSP = "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'"

var inlineScript = regexp.MustCompile(`(?s)<script(\s[^>]*)?>(.*?)</script>`)

// policyFor is CSP plus a script-src that allows same-origin files and, by SHA-256 hash, exactly
// the inline scripts in index (SvelteKit's bootstrap). Scripts stay strict: no 'unsafe-inline'.
func policyFor(index []byte) string {
	p := CSP + "; script-src 'self'"
	for _, m := range inlineScript.FindAllSubmatch(index, -1) {
		if strings.Contains(string(m[1]), "src=") {
			continue
		}
		sum := sha256.Sum256(m[2])
		p += " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	}
	return p
}

func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // the embed directive guarantees dist exists
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		panic(err) // make web always writes index.html
	}
	policy := policyFor(index)
	files := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", policy)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "favicon.ico" {
			// Browsers ask for /favicon.ico unprompted; the mark exists only as SVG.
			http.Redirect(w, r, "/favicon.svg", http.StatusMovedPermanently)
			return
		}
		if p == "" || p == "." {
			files.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(sub, p); err != nil {
			if path.Ext(p) != "" {
				http.NotFound(w, r)
				return
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/" // client-side route: serve index.html
			files.ServeHTTP(w, r2)
			return
		}
		files.ServeHTTP(w, r)
	})
}
