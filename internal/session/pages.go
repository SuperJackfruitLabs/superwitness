package session

import (
	"html/template"
	"net/http"

	"github.com/SuperJackfruitLabs/superwitness/internal/web"
)

// The pages a browser lands on when sign-in cannot finish. They are served before the app has
// a session, so they are plain HTML from the binary, not the app.
var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} · superwitness</title>
<style>
body{margin:0;font:16px/1.55 system-ui,-apple-system,sans-serif;background:#0e1116;color:#e8ecf2}
main{max-width:34rem;margin:0 auto;padding:3rem 1rem}h1{font:600 1.5rem Georgia,serif;margin:0 0 .75rem}
a{color:#a6ecd6}p{color:#bcc4d0}
@media (prefers-color-scheme: light){body{background:#f7f8f6;color:#12161c}a{color:#1b7f63}p{color:#353d48}}
</style></head>
<body><main><h1>{{.Title}}</h1><p>{{.Body}}</p>{{if .Link}}<p><a href="{{.Link}}">{{.LinkText}}</a></p>{{end}}</main></body></html>
`))

type pageData struct{ Title, Body, Link, LinkText string }

func render(w http.ResponseWriter, status int, d pageData) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", web.CSP)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = page.Execute(w, d)
}

type pageSet struct{ notAuthorised, down, expired, failed, database pageData }

// pagesFor names the sign-in service on each page: "AgentPod" under the hub, the plane's host
// under the organization plane.
func pagesFor(provider string) pageSet {
	if provider == "" {
		provider = "AgentPod"
	}
	signIn := "Sign in with " + provider
	return pageSet{
		notAuthorised: pageData{"Not authorised", "This " + provider + " account is not allowed to use this superwitness. Ask its operator to add you.", "/", "Back"},
		down:          pageData{provider + " sign-in is unavailable", provider + " did not answer, so you could not be signed in. Sessions that are already open keep working.", "/auth/login", "Try again"},
		expired:       pageData{"Sign-in expired", "This sign-in took too long or was started in another tab. Start again.", "/auth/login", signIn},
		failed:        pageData{"Sign-in failed", provider + " did not confirm this sign-in. Start again.", "/auth/login", signIn},
		database:      pageData{"Can't reach the database", "superwitness could not record your session. Try again in a minute.", "/auth/login", signIn},
	}
}

var pageOff = pageData{"Sign-in is off", "This superwitness has no one allowed to sign in, so its app is off. Its API still answers bearer tokens.", "", ""}

// Off answers /auth/* when sign-in is off.
func Off() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { render(w, http.StatusNotFound, pageOff) })
}
