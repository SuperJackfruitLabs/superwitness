package docsclaims

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/canary"
	"github.com/SuperJackfruitLabs/superwitness/internal/session"
)

// The public surface, listed from the code: the real routers, the real config loaders, and
// the Go parser over the CLI and the log calls. Each list is what a page must name.

const baselinePath = "docs/undocumented-baseline.json"

// surfaceRoutes walks the API's chi router and the sign-in router mounted at /auth, and adds
// the app's pages from SvelteKit's routes directory. A path served for every method, /mcp,
// is "ANY /mcp".
func surfaceRoutes(t *testing.T) []string {
	methods := map[string][]string{}
	walk := func(h http.Handler, prefix string) {
		r, ok := h.(chi.Routes)
		if !ok {
			t.Fatalf("%T is not a chi router", h)
		}
		err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			if !strings.Contains(route, "*") { // /auth/* is the mount; its routes are walked below
				methods[prefix+route] = append(methods[prefix+route], method)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	walk((&api.Server{MCP: http.NotFoundHandler(), Health: http.NotFoundHandler(), Login: http.NotFoundHandler()}).Handler(), "")
	walk((&session.Login{}).Handler(), "/auth")
	var out []string
	for path, ms := range methods {
		if len(ms) > 4 { // registered with Handle: every method
			out = append(out, "ANY "+path)
			continue
		}
		for _, m := range ms {
			out = append(out, m+" "+path)
		}
	}
	return append(out, appPages(t)...)
}

var svelteParam = regexp.MustCompile(`^\[([a-z_]+)(?:=[a-z_]+)?\]$`)

// appPages turns web/src/routes/**/+page.svelte into "GET /path": [board=id] is {board};
// (group) directories add nothing; the [...rest] catch-all is the not-found page, not a route.
func appPages(t *testing.T) []string {
	root := filepath.Join(repoRoot(t), "web", "src", "routes")
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "+page.svelte" {
			return err
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		var segs []string
		for _, s := range strings.Split(filepath.ToSlash(rel), "/") {
			switch {
			case s == "." || strings.HasPrefix(s, "("):
			case strings.HasPrefix(s, "[..."):
				return nil
			case svelteParam.MatchString(s):
				segs = append(segs, "{"+svelteParam.FindStringSubmatch(s)[1]+"}")
			default:
				segs = append(segs, s)
			}
		}
		out = append(out, "GET /"+strings.Join(segs, "/"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// surfaceEnv records what the service's and the canary's loaders ask the environment for,
// with and without the organization plane, plus the installer's settings.
func surfaceEnv(t *testing.T) []string {
	seen := realEnvVars()
	for _, plane := range []string{"", "https://plane.example"} {
		_, _ = canary.LoadConfig(func(k string) string {
			seen[k] = true
			if k == "SW_ORG_PLANE_URL" {
				return plane
			}
			return ""
		})
	}
	var out []string
	for k := range seen {
		out = append(out, k)
	}
	return append(out, installerEnv(t)...)
}

// installerEnv reads scripts/install.sh's optional settings, the ${NAME:-default} it expands.
// There is no shell parser in the module; the script's own convention is this one form.
var shellDefault = regexp.MustCompile(`\$\{([A-Z][A-Z0-9_]*):-`)

func installerEnv(t *testing.T) []string {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range shellDefault.FindAllStringSubmatch(string(b), -1) {
		out = append(out, m[1])
	}
	return uniq(out)
}

// goFiles parses a directory's non-test Go files.
func goFiles(t *testing.T, dir string) []*ast.File {
	var out []*ast.File
	fset := token.NewFileSet()
	matches, _ := filepath.Glob(filepath.Join(repoRoot(t), dir, "*.go"))
	for _, p := range matches {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, f)
	}
	return out
}

func strLit(e ast.Expr) (string, bool) {
	l, ok := e.(*ast.BasicLit)
	if !ok || l.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(l.Value)
	return s, err == nil
}

// switchCases lists the string cases of every switch in the named function.
func switchCases(files []*ast.File, fn string) []string {
	var out []string
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Name.Name != fn {
				continue
			}
			ast.Inspect(fd, func(n ast.Node) bool {
				if cc, ok := n.(*ast.CaseClause); ok {
					for _, e := range cc.List {
						if s, ok := strLit(e); ok {
							out = append(out, s)
						}
					}
				}
				return true
			})
		}
	}
	return out
}

// flagSets finds each flag.NewFlagSet("<command>", …) and the flags defined on it in the same
// function, as "<command> -<flag>".
func flagSets(files []*ast.File) (commands, flags []string) {
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			sets := map[string]string{} // variable -> command
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				as, ok := n.(*ast.AssignStmt)
				if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
					return true
				}
				call, ok := as.Rhs[0].(*ast.CallExpr)
				if ok && callee(call) == "flag.NewFlagSet" && len(call.Args) > 0 {
					if name, ok := strLit(call.Args[0]); ok {
						if id, ok := as.Lhs[0].(*ast.Ident); ok {
							sets[id.Name] = name
							commands = append(commands, name)
						}
					}
				}
				return true
			})
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				x, ok := sel.X.(*ast.Ident)
				if !ok || sets[x.Name] == "" || !flagDefiners[sel.Sel.Name] {
					return true
				}
				if name, ok := strLit(call.Args[0]); ok {
					flags = append(flags, sets[x.Name]+" -"+name)
				}
				return true
			})
		}
	}
	return commands, flags
}

var flagDefiners = map[string]bool{"String": true, "Bool": true, "Int": true, "Int64": true, "Uint": true,
	"Uint64": true, "Duration": true, "Float64": true, "Func": true, "BoolFunc": true}

// callee names a call's function as "pkg.Func", or "" when it is not that form.
func callee(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return x.Name + "." + sel.Sel.Name
}

// surfaceCLI is the binary's commands (the cases main's run dispatches, and canary.Main's)
// and every flag set's command and flags.
func surfaceCLI(t *testing.T) (commands, flags []string) {
	cmdFiles, canaryFiles := goFiles(t, "cmd/superwitness"), goFiles(t, "internal/canary")
	for _, c := range switchCases(cmdFiles, "run") {
		if !strings.HasPrefix(c, "-") { // --version and -version are spellings of version
			commands = append(commands, c)
		}
	}
	for _, c := range switchCases(canaryFiles, "Main") {
		commands = append(commands, "canary "+c)
	}
	setCmds, fl := flagSets(append(cmdFiles, canaryFiles...))
	return uniq(append(commands, setCmds...)), uniq(fl)
}

var logEvent = regexp.MustCompile(`^[a-z]+\.[a-z_]+$`)

var logMethods = map[string]int{"Debug": 0, "Info": 0, "Warn": 0, "Error": 0,
	"DebugContext": 1, "InfoContext": 1, "WarnContext": 1, "ErrorContext": 1}

// surfaceLogs is every structured log event, a "<area>.<event>" message passed to a log call,
// in the binary's packages.
func surfaceLogs(t *testing.T) []string {
	var dirs []string
	for _, top := range []string{"cmd", "internal"} {
		_ = filepath.WalkDir(filepath.Join(repoRoot(t), top), func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return err
			}
			if d.Name() == "testdata" || d.Name() == "dist" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			rel, _ := filepath.Rel(repoRoot(t), p)
			dirs = append(dirs, rel)
			return nil
		})
	}
	var out []string
	for _, dir := range dirs {
		for _, f := range goFiles(t, dir) {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				i, ok := logMethods[sel.Sel.Name]
				if !ok || len(call.Args) <= i {
					return true
				}
				if s, ok := strLit(call.Args[i]); ok && logEvent.MatchString(s) {
					out = append(out, s)
				}
				return true
			})
		}
	}
	return uniq(out)
}

type surface struct {
	items []string
	named func(string) bool
}

func surfaces(t *testing.T, docs []page) map[string]surface {
	texts := make([]string, len(docs))
	for i, p := range docs {
		texts[i] = p.text
	}
	all := strings.Join(texts, "\n")
	commands, flags := surfaceCLI(t)
	var tools []string
	for k := range realTools(t) {
		tools = append(tools, k)
	}
	return map[string]surface{
		SurfaceRoutes:   {surfaceRoutes(t), func(s string) bool { return RouteNamed(all, s) }},
		SurfaceEnv:      {surfaceEnv(t), func(s string) bool { return EnvNamed(all, s) }},
		SurfaceCommands: {commands, func(s string) bool { return CommandNamed(all, s) }},
		SurfaceFlags:    {flags, func(s string) bool { return FlagNamed(texts, s) }},
		SurfaceTools:    {tools, func(s string) bool { return WordNamed(all, s) }},
		SurfaceLogs:     {surfaceLogs(t), func(s string) bool { return WordNamed(all, s) }},
	}
}

func TestSurfaceRegistriesAreNotEmpty(t *testing.T) {
	// A guard on the guard: an extractor that finds nothing passes every page.
	s := surfaces(t, nil)
	want := map[string][]string{
		SurfaceRoutes:   {"GET /v1/runs", "POST /v1/runs", "GET /health", "ANY /mcp", "POST /auth/logout", "GET /", "GET /rubrics/{id}/{version}"},
		SurfaceEnv:      {"SW_DATABASE_URL", "SW_ORG_PLANE_SERVICE_CREDENTIAL_FILE", "SWC_BOARD_ID", "SWC_SUPERWITNESS_AUDIENCE", "SWC_HUB_CLIENT_ID", "BIN_DIR"},
		SurfaceCommands: {"serve", "migrate", "canary", "canary run", "skill install"},
		SurfaceFlags:    {"rubric-add -id", "skill install -dir", "canary run -trigger", "canary status -json"},
		SurfaceTools:    {"record_verdict", "list_runs"},
		SurfaceLogs:     {"runs.report_rejected", "auth.signin", "transcript.read"},
	}
	for name, items := range want {
		have := map[string]bool{}
		for _, it := range s[name].items {
			have[it] = true
		}
		for _, it := range items {
			if !have[it] {
				t.Errorf("%s: extractor did not find %q; found %v", name, it, s[name].items)
			}
		}
	}
	if n := len(s[SurfaceRoutes].items); n < 20 {
		t.Errorf("routes: only %d found", n)
	}
	if n := len(s[SurfaceEnv].items); n < 40 {
		t.Errorf("env: only %d found", n)
	}
}

// TestDocsCompleteness is the ratchet: everything the code defines is named on a docs page,
// or listed in docs/undocumented-baseline.json, which only shrinks. Set
// DOCSCLAIMS_WRITE_BASELINE=1 to print what the baseline should hold.
func TestDocsCompleteness(t *testing.T) {
	docs := pages(t, "docs-site/src/content/docs", ".md", ".mdx")
	all := surfaces(t, docs)
	if os.Getenv("DOCSCLAIMS_WRITE_BASELINE") == "1" {
		want := Baseline{}
		for _, name := range Surfaces {
			want[name] = Undocumented(all[name].items, all[name].named)
		}
		b, _ := json.MarshalIndent(want, "", "  ")
		t.Logf("%s:\n%s", baselinePath, b)
		return
	}
	bl, err := LoadBaseline(filepath.Join(repoRoot(t), baselinePath))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range Surfaces {
		s := all[name]
		for _, v := range Ratchet(name, s.items, s.named, bl[name]) {
			t.Error(v)
		}
	}
}

func TestSurfaceItemsAreSorted(t *testing.T) {
	// The baseline is reviewed by hand: keep each list sorted so a diff shows one line per item.
	bl, err := LoadBaseline(filepath.Join(repoRoot(t), baselinePath))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range Surfaces {
		if !sort.StringsAreSorted(bl[name]) {
			t.Errorf("%s: %s is not sorted", baselinePath, name)
		}
	}
}
