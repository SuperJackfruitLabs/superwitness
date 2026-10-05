package docsclaims

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/config"
	swmcp "github.com/SuperJackfruitLabs/superwitness/internal/mcp"
)

func repoRoot(t *testing.T) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// realEnvVars records every name config.Load and config.LoadMigrate ask the environment for.
func realEnvVars() map[string]bool {
	seen := map[string]bool{}
	rec := func(k string) string { seen[k] = true; return "" }
	_, _ = config.Load(rec)
	_, _ = config.LoadMigrate(rec)
	return seen
}

// realRoute matches a path against the API's own chi router, GET or POST.
func realRoute(t *testing.T) func(string) bool {
	h := (&api.Server{MCP: http.NotFoundHandler(), Health: http.NotFoundHandler()}).Handler()
	mux, ok := h.(*chi.Mux)
	if !ok {
		t.Fatalf("api.Server.Handler is %T, not *chi.Mux", h)
	}
	return func(p string) bool {
		for _, m := range []string{http.MethodGet, http.MethodPost} {
			if mux.Match(chi.NewRouteContext(), m, p) {
				return true
			}
		}
		return false
	}
}

// realTools lists the tools the MCP server registers, over an in-memory session.
func realTools(t *testing.T) map[string]bool {
	ctx := context.Background()
	srv := swmcp.NewServer(nil, auth.Principal{}, "docsclaims")
	ct, st := sdk.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "docsclaims"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, tool := range res.Tools {
		out[tool.Name] = true
	}
	return out
}

func TestRegistriesAreNotEmpty(t *testing.T) {
	// A guard on the guard: an empty registry fails every page; a broken extractor passes them all.
	if env := realEnvVars(); !env["SW_DATABASE_URL"] || !env["SW_MIGRATE_DATABASE_URL"] || len(env) < 10 {
		t.Fatalf("env registry looks wrong: %v", env)
	}
	if r := realRoute(t); !r("/v1/verdicts") || !r("/v1/runs/by-attempt/x") || r("/v1/nope") {
		t.Fatal("route registry looks wrong")
	}
	if tools := realTools(t); len(tools) != 4 || !tools["record_verdict"] {
		t.Fatalf("tool registry looks wrong: %v", tools)
	}
}

func TestRouteRuleAcceptsConcreteAndPlaceholderPaths(t *testing.T) {
	r := realRoute(t)
	for _, p := range []string{"/v1/runs/superpipeline/{board}/{run}", "/v1/runs/superpipeline/brd_01/run_01/spans",
		"/v1/runs/superpipeline/{board}/{run}/logs", "/v1/runs/by-attempt/{attempt}"} {
		if !r(p) {
			t.Errorf("%s should match a registered route", p)
		}
	}
}

type page struct{ rel, text string }

func pages(t *testing.T, dir string, exts ...string) []page {
	var out []page
	root := filepath.Join(repoRoot(t), dir)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "node_modules" {
			return filepath.SkipDir
		}
		for _, e := range exts {
			if !d.IsDir() && strings.HasSuffix(p, e) {
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				rel, _ := filepath.Rel(root, p)
				out = append(out, page{rel, string(b)})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDocsClaims(t *testing.T) {
	reg := Registry{EnvVars: realEnvVars(), Route: realRoute(t), Tools: realTools(t)}
	docs := pages(t, "docs-site/src/content/docs", ".md", ".mdx")
	if len(docs) < 14 {
		t.Fatalf("found %d docs pages; the walk is broken or pages are missing", len(docs))
	}
	for _, p := range docs {
		for _, v := range Check(p.rel, p.text, reg, true) {
			t.Error(v)
		}
	}
	landing := pages(t, "landing/src", ".astro")
	if len(landing) < 1 {
		t.Fatal("found no landing pages; the walk is broken")
	}
	for _, p := range landing {
		for _, v := range Check("landing/"+p.rel, p.text, reg, false) {
			t.Error(v)
		}
	}
	// The README and the deploy env examples name variables and routes too.
	root := repoRoot(t)
	files := []string{"README.md", "deploy/env.example", "deploy/canary/canary.env.example"}
	for _, rel := range files {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if len(EnvVars(string(b)))+len(Routes(string(b))) == 0 {
			t.Errorf("%s: no SW_* variable or /v1 route found; the extractor is broken", rel)
		}
		r := reg
		if rel == "deploy/canary/canary.env.example" {
			// SWC_OTLP_LOGS_URL points at the collector's OTLP/HTTP port, not at superwitness.
			r.Route = func(p string) bool { return p == "/v1/logs" || p == "/v1/traces" || reg.Route(p) }
		}
		for _, v := range Check(rel, string(b), r, false) {
			t.Error(v)
		}
	}
}

func TestInstallSQLMatchesREADME(t *testing.T) {
	// internal/testutil runs README's SQL blocks against Postgres; the install page must say the same.
	readme, err := os.ReadFile(filepath.Join(repoRoot(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	install, err := os.ReadFile(filepath.Join(repoRoot(t), "docs-site/src/content/docs/install.md"))
	if err != nil {
		t.Fatal(err)
	}
	want, got := SQLBlocks(string(readme)), SQLBlocks(string(install))
	for _, name := range []string{"provision", "runtime-grants"} {
		if want[name] == "" || got[name] != want[name] {
			t.Errorf("install.md sql:%s differs from README.md (or is missing)", name)
		}
	}
}
