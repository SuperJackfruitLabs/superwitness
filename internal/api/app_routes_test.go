package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/join"
	"github.com/SuperJackfruitLabs/superwitness/internal/ratelimit"
	"github.com/SuperJackfruitLabs/superwitness/internal/runs"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/fake"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

const appOrigin = "https://superwitness.example"

// sessions is a stand-in session store: cookie value → principal.
type sessions map[string]auth.Principal

func (s sessions) Resolve(_ context.Context, tok string) (auth.Principal, error) {
	if p, ok := s[tok]; ok {
		return p, nil
	}
	return auth.Principal{}, auth.ErrSessionInvalid
}

type appServer struct {
	srv  *httptest.Server
	runs *runs.MemStore
}

func newAppServer(t *testing.T, limits *api.Limits) *appServer {
	t.Helper()
	return newAppServerWith(t, limits, nil)
}

// newAppServerWith is newAppServer with a last say over the Server before it is built.
func newAppServerWith(t *testing.T, limits *api.Limits, mutate func(*api.Server)) *appServer {
	t.Helper()
	d, err := fake.NewDev()
	if err != nil {
		t.Fatal(err)
	}
	store := verdicts.NewMemStore()
	rs := runs.NewMemStore(store)
	ops := &api.Ops{
		Join: &join.Joiner{Superpipeline: d.SP, AgentPod: d.AP, Traces: d.Traces, Logs: d.Logs, Errors: d.Errors,
			Verdicts: store, Principals: d},
		Spans: d, Logs: d, Attempts: d, Transcripts: d,
		Verdicts:   &verdicts.Service{Store: store, Subjects: &join.Subjects{Superpipeline: d.SP, AgentPod: d.AP, Attempts: d}},
		Runs:       rs,
		RunSources: map[string]string{"prn_reporter01": "superpipeline"},
		Rubrics:    store,
		History:    store,
		Now:        func() time.Time { return registryNow },
	}
	login := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	s := &api.Server{Ops: ops, Auth: auth.DevAuthenticator{},
		MCP:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
		Sessions:      sessions{"cookie-human01": {ID: "prn_human01", Kind: auth.KindHuman, Email: "human01@example.com"}},
		SessionCookie: "sw_session", PublicURL: appOrigin, Login: login, Limits: limits}
	if mutate != nil {
		mutate(s)
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return &appServer{srv: srv, runs: rs}
}

// call sends a request with a bearer token, a session cookie and an Origin, each optional.
func (a *appServer) call(t *testing.T, method, path, bearer, cookie, origin, body string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, a.srv.URL+path, strings.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "sw_session", Value: cookie})
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestMe(t *testing.T) {
	a := newAppServer(t, nil)
	if _, b := a.call(t, "GET", "/v1/me", "", "cookie-human01", "", ""); b != `{"email":"human01@example.com","kind":"human","principal":"prn_human01","via":"session"}`+"\n" {
		t.Errorf("session: %s", b)
	}
	if _, b := a.call(t, "GET", "/v1/me", "dev:prn_agent01:agent", "", "", ""); b != `{"email":null,"kind":"agent","principal":"prn_agent01","via":"bearer"}`+"\n" {
		t.Errorf("bearer: %s", b)
	}
	if resp, _ := a.call(t, "GET", "/v1/me", "", "", "", ""); resp.StatusCode != 401 {
		t.Errorf("neither: %d", resp.StatusCode)
	}
}

func TestSessionsCannotReportRunsOrReachMCP(t *testing.T) {
	a := newAppServer(t, nil)
	if resp, b := a.call(t, "POST", "/v1/runs", "", "cookie-human01", appOrigin, runReport); resp.StatusCode != 403 || !strings.Contains(b, "bearer_required") {
		t.Errorf("report with a session: %d %s", resp.StatusCode, b)
	}
	if resp, _ := a.call(t, "POST", "/mcp", "", "cookie-human01", appOrigin, "{}"); resp.StatusCode != 401 {
		t.Errorf("/mcp with a session: %d; want 401", resp.StatusCode)
	}
	if resp, _ := a.call(t, "POST", "/mcp", "dev:prn_agent01:agent", "", "", "{}"); resp.StatusCode != 204 {
		t.Errorf("/mcp with a bearer: %d", resp.StatusCode)
	}
}

func TestSessionWritesNeedTheOrigin(t *testing.T) {
	a := newAppServer(t, nil)
	body := `{"idempotency_key":"k-origin","subject_kind":"run","subject_ref":"superpipeline:brd_01/run_01","standard":"stage:review","value":{"decision":"pass"}}`
	if resp, b := a.call(t, "POST", "/v1/verdicts", "", "cookie-human01", "", body); resp.StatusCode != 403 || !strings.Contains(b, "origin_mismatch") {
		t.Errorf("no Origin: %d %s", resp.StatusCode, b)
	}
	if resp, b := a.call(t, "POST", "/v1/verdicts", "", "cookie-human01", appOrigin, body); resp.StatusCode != 201 || !strings.Contains(b, `"judge":"prn_human01"`) {
		t.Errorf("right Origin: %d %s", resp.StatusCode, b)
	}
}

func TestVerdictHistory(t *testing.T) {
	a := newAppServer(t, nil)
	post := func(key, value, supersedes string) string {
		body := `{"idempotency_key":"` + key + `","subject_kind":"run","subject_ref":"superpipeline:brd_01/run_01","standard":"stage:review","value":` + value
		if supersedes != "" {
			body += `,"supersedes":"` + supersedes + `"`
		}
		resp, b := a.call(t, "POST", "/v1/verdicts", humanTok, "", "", body+"}")
		if resp.StatusCode != 201 {
			t.Fatalf("%d %s", resp.StatusCode, b)
		}
		var v struct{ ID string }
		_ = json.Unmarshal([]byte(b), &v)
		return v.ID
	}
	first := post("k-h1", `{"decision":"pass"}`, "")
	second := post("k-h2", `{"decision":"fail"}`, first)
	resp, b := a.call(t, "GET", "/v1/verdicts?subject_kind=run&subject_ref=superpipeline:brd_01/run_01", "", "cookie-human01", "", "")
	var h struct {
		Verdicts []struct {
			ID           string  `json:"id"`
			SupersededBy *string `json:"superseded_by"`
		} `json:"verdicts"`
	}
	_ = json.Unmarshal([]byte(b), &h)
	if resp.StatusCode != 200 || len(h.Verdicts) != 2 || h.Verdicts[0].ID != first || h.Verdicts[0].SupersededBy == nil ||
		*h.Verdicts[0].SupersededBy != second || h.Verdicts[1].SupersededBy != nil {
		t.Errorf("history: %d %s", resp.StatusCode, b)
	}
	if _, b := a.call(t, "GET", "/v1/verdicts?subject_kind=run&subject_ref=nope", humanTok, "", "", ""); !strings.Contains(b, "invalid_subject") {
		t.Errorf("bad ref: %s", b)
	}
	if _, b := a.call(t, "GET", "/v1/verdicts?subject_kind=card&subject_ref=x", humanTok, "", "", ""); !strings.Contains(b, "invalid_subject_kind") {
		t.Errorf("bad kind: %s", b)
	}
}

func TestScopes(t *testing.T) {
	a := newAppServer(t, nil)
	if resp, b := a.call(t, "POST", "/v1/runs", reporterTok, "", "", runReport); resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if _, b := a.call(t, "GET", "/v1/scopes", "", "cookie-human01", "", ""); b != `{"scopes":[{"source":"superpipeline","id":"brd_01","name":"Press","runs":1}]}`+"\n" {
		t.Errorf("scopes: %s", b)
	}
}

func TestRateLimits(t *testing.T) {
	now := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	limits := &api.Limits{Auth: ratelimit.PerMinute(api.AuthPerMinute, api.AuthBurst),
		SessionWrites: ratelimit.PerMinute(api.SessionWritesPerMinute, api.SessionWriteBurst),
		Reports:       ratelimit.PerMinute(api.ReportsPerMinute, api.ReportBurst),
		ClientIP:      func(r *http.Request) string { return ratelimit.ClientIP(r, nil) }}
	limits.Auth.Now, limits.SessionWrites.Now, limits.Reports.Now = clock, clock, clock
	a := newAppServer(t, limits)

	for i := range api.AuthBurst {
		if resp, _ := a.call(t, "GET", "/auth/login", "", "", "", ""); resp.StatusCode != 204 {
			t.Fatalf("sign-in %d: %d", i+1, resp.StatusCode)
		}
	}
	resp, b := a.call(t, "GET", "/auth/login", "", "", "", "")
	if resp.StatusCode != 429 || resp.Header.Get("Retry-After") != "6" || !strings.Contains(b, "rate_limited") {
		t.Errorf("sign-in over the limit: %d %q %s", resp.StatusCode, resp.Header.Get("Retry-After"), b)
	}

	body := func(i int) string {
		return `{"idempotency_key":"k-rate-` + time.Duration(i).String() + `","subject_kind":"run","subject_ref":"superpipeline:brd_01/run_01","standard":"stage:review","value":{"decision":"pass"}}`
	}
	for i := range api.SessionWriteBurst {
		if resp, b := a.call(t, "POST", "/v1/verdicts", "", "cookie-human01", appOrigin, body(i)); resp.StatusCode != 201 {
			t.Fatalf("session write %d: %d %s", i+1, resp.StatusCode, b)
		}
	}
	if resp, _ := a.call(t, "POST", "/v1/verdicts", "", "cookie-human01", appOrigin, body(99)); resp.StatusCode != 429 {
		t.Errorf("session write over the limit: %d", resp.StatusCode)
	}
	if resp, _ := a.call(t, "GET", "/v1/me", "", "cookie-human01", "", ""); resp.StatusCode != 200 {
		t.Errorf("a session read is not limited: %d", resp.StatusCode)
	}
	if resp, _ := a.call(t, "POST", "/v1/verdicts", humanTok, "", "", body(100)); resp.StatusCode != 201 {
		t.Errorf("a bearer write is not a session write: %d", resp.StatusCode)
	}

	for i := range api.ReportBurst {
		if resp, b := a.call(t, "POST", "/v1/runs", reporterTok, "", "", runReport); resp.StatusCode != 200 {
			t.Fatalf("report %d: %d %s", i+1, resp.StatusCode, b)
		}
	}
	if resp, b := a.call(t, "POST", "/v1/runs", reporterTok, "", "", runReport); resp.StatusCode != 429 || resp.Header.Get("Retry-After") != "1" {
		t.Errorf("report over the limit: %d %q %s", resp.StatusCode, resp.Header.Get("Retry-After"), b)
	}
}

func TestRetryAfterRoundsUp(t *testing.T) {
	// 7 a minute is a token every 8.57 s: Retry-After must say 9, never 8, or a client that waits
	// exactly as told is refused again.
	now := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	l := ratelimit.PerMinute(7, 1)
	l.Now = func() time.Time { return now }
	a := newAppServer(t, &api.Limits{Auth: l, ClientIP: func(r *http.Request) string { return ratelimit.ClientIP(r, nil) }})
	if resp, _ := a.call(t, "GET", "/auth/login", "", "", "", ""); resp.StatusCode != 204 {
		t.Fatalf("first sign-in: %d", resp.StatusCode)
	}
	if resp, b := a.call(t, "GET", "/auth/login", "", "", "", ""); resp.StatusCode != 429 || resp.Header.Get("Retry-After") != "9" {
		t.Errorf("over the limit: %d Retry-After %q %s; want 429 and 9", resp.StatusCode, resp.Header.Get("Retry-After"), b)
	}
}

func TestSessionsNeedThePublicOrigin(t *testing.T) {
	ss := sessions{"cookie-human01": {ID: "prn_human01", Kind: auth.KindHuman}}
	for _, bad := range []string{"", "superwitness.example", "ftp://superwitness.example"} {
		if _, err := (&api.Server{Auth: auth.DevAuthenticator{}, Sessions: ss, SessionCookie: "sw_session", PublicURL: bad}).Build(); err == nil {
			t.Errorf("sessions with SW_PUBLIC_URL %q: built", bad)
		}
	}
	if _, err := (&api.Server{Auth: auth.DevAuthenticator{}, PublicURL: ""}).Build(); err != nil {
		t.Errorf("bearer only, no public URL: %v", err)
	}
	// SW_PUBLIC_URL as an operator might write it still matches the Origin a browser sends.
	a := newAppServerWith(t, nil, func(s *api.Server) { s.PublicURL = "HTTPS://SuperWitness.Example:443/" })
	body := `{"idempotency_key":"k-norm","subject_kind":"run","subject_ref":"superpipeline:brd_01/run_01","standard":"stage:review","value":{"decision":"pass"}}`
	if resp, b := a.call(t, "POST", "/v1/verdicts", "", "cookie-human01", appOrigin, body); resp.StatusCode != 201 {
		t.Errorf("session write with the browser's Origin: %d %s", resp.StatusCode, b)
	}
	defer func() {
		if recover() == nil {
			t.Error("Handler built a session gate with no origin")
		}
	}()
	(&api.Server{Auth: auth.DevAuthenticator{}, Sessions: ss, SessionCookie: "sw_session"}).Handler()
}

// edgeServer serves /mcp with a handler that counts the calls that reach it.
func edgeServer(t *testing.T, trusted []netip.Prefix) (http.Handler, *int) {
	t.Helper()
	calls := new(int)
	mcp := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { *calls++; w.WriteHeader(http.StatusNoContent) })
	web := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h, err := (&api.Server{Auth: auth.DevAuthenticator{}, MCP: mcp, Web: web, TrustedProxies: trusted}).Build()
	if err != nil {
		t.Fatal(err)
	}
	return h, calls
}

// raw sends one request straight to h, from peer, with the path exactly as written.
func raw(h http.Handler, method, target, peer, cfIP, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader("{}"))
	req.RemoteAddr = peer
	if cfIP != "" {
		req.Header.Set("CF-Connecting-IP", cfIP)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var mcpVariants = []string{"//mcp", "/MCP", "/Mcp", "/./mcp", "/%2Fmcp", "/%2fmcp", "/%6dcp", "/mcp/", "/mcp/x", "/x/../mcp", "///mcp/"}

const agentTok = "dev:prn_agent01:agent"

func TestMCPIsHiddenFromThePublicEdge(t *testing.T) {
	h, calls := edgeServer(t, []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	const proxy, visitor = "127.0.0.1:40000", "198.51.100.7"
	for _, m := range []string{"GET", "POST"} {
		for _, p := range append([]string{"/mcp"}, mcpVariants...) {
			// 404 before auth: with a valid bearer token, and with none (not 401, which would say /mcp exists).
			for _, tok := range []string{agentTok, ""} {
				if rec := raw(h, m, p, proxy, visitor, tok); rec.Code != 404 || !strings.Contains(rec.Body.String(), "not_found") {
					t.Errorf("edge %s %s (bearer %v): %d %s; want 404", m, p, tok != "", rec.Code, rec.Body.String())
				}
			}
		}
	}
	if *calls != 0 {
		t.Errorf("the MCP handler ran %d times for edge requests", *calls)
	}
	// The edge keeps the rest of the API.
	if rec := raw(h, "GET", "/v1/nope", proxy, visitor, agentTok); rec.Code != 404 {
		t.Errorf("edge /v1/nope: %d", rec.Code)
	}
	if rec := raw(h, "GET", "/runs", proxy, visitor, ""); rec.Code != 200 {
		t.Errorf("edge app page: %d", rec.Code)
	}
}

func TestMCPStaysOnTheTailnet(t *testing.T) {
	h, calls := edgeServer(t, []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	// No CF-Connecting-IP from the proxy, or a header from a peer that is not a trusted proxy:
	// a tailnet caller, which reaches /mcp as before.
	for name, c := range map[string]struct{ peer, cf string }{
		"proxy, no header":          {"127.0.0.1:40000", ""},
		"untrusted peer, header":    {"192.0.2.9:40000", "198.51.100.7"},
		"untrusted peer, no header": {"192.0.2.9:40000", ""},
	} {
		before := *calls
		if rec := raw(h, "POST", "/mcp", c.peer, c.cf, agentTok); rec.Code != 204 || *calls != before+1 {
			t.Errorf("%s: /mcp with a bearer: %d (calls %d → %d)", name, rec.Code, before, *calls)
		}
		if rec := raw(h, "POST", "/mcp", c.peer, c.cf, ""); rec.Code != 401 {
			t.Errorf("%s: /mcp without a bearer: %d; want 401", name, rec.Code)
		}
		// Only exactly /mcp is MCP: no variant is cleaned, redirected or matched onto it.
		before = *calls
		for _, m := range []string{"GET", "POST"} {
			for _, p := range mcpVariants {
				rec := raw(h, m, p, c.peer, c.cf, agentTok)
				if *calls != before || rec.Code == 204 || (rec.Code >= 300 && rec.Code < 400) {
					t.Errorf("%s: %s %s reached MCP or redirected: %d %v (calls %d → %d)", name, m, p, rec.Code, rec.Header(), before, *calls)
				}
			}
		}
	}
}

func TestMCPEdgeTrustDefaultsToThisHost(t *testing.T) {
	// SW_TRUSTED_PROXIES unset: a tunnel connector on this host (loopback) is the edge.
	h, calls := edgeServer(t, nil)
	if rec := raw(h, "POST", "/mcp", "127.0.0.1:40000", "198.51.100.7", agentTok); rec.Code != 404 || *calls != 0 {
		t.Errorf("edge via loopback, default trust: %d (calls %d)", rec.Code, *calls)
	}
	if rec := raw(h, "POST", "/mcp", "127.0.0.1:40000", "", agentTok); rec.Code != 204 {
		t.Errorf("loopback without the header: %d", rec.Code)
	}
}

func TestEmptyTrustedProxiesIsTheHostDefault(t *testing.T) {
	// A non-nil empty list must not switch the edge guard off: it means this host's own addresses.
	h, calls := edgeServer(t, []netip.Prefix{})
	if rec := raw(h, "POST", "/mcp", "127.0.0.1:40000", "198.51.100.7", agentTok); rec.Code != 404 || *calls != 0 {
		t.Errorf("edge request with an empty TrustedProxies: %d (calls %d); want 404", rec.Code, *calls)
	}
}

func TestAuthLimitAlwaysKeysOnAClientIP(t *testing.T) {
	l := ratelimit.PerMinute(1, 1)
	a := newAppServer(t, &api.Limits{Auth: l}) // no ClientIP configured
	if resp, _ := a.call(t, "GET", "/auth/login", "", "", "", ""); resp.StatusCode != 204 {
		t.Fatalf("first sign-in: %d", resp.StatusCode)
	}
	if resp, _ := a.call(t, "GET", "/auth/login", "", "", "", ""); resp.StatusCode != 429 {
		t.Errorf("second sign-in with a limit but no ClientIP: %d; want 429", resp.StatusCode)
	}
}
