// Package archtest holds the shape of the Go code: size limits that work as ratchets, and the
// direction imports may point between packages. See docs/code-quality.md.
//
// Only shipped code counts: _test.go files and testdata are left out. A test may import whatever
// it needs (except cmd/, which nothing imports).
package archtest

import (
	"bufio"
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	// fileLimit is the most lines a shipped .go file may have without an entry in oversizeFiles.
	fileLimit = 1000
	// packageLimit is the most lines a package (one directory) may have without an entry in
	// oversizePackages.
	packageLimit = 4000
)

// oversizeFiles lists the files over fileLimit and the most lines each may have. Lower these;
// never raise them. Split a file rather than adding it here.
var oversizeFiles = map[string]int{}

// oversizePackages lists the packages over packageLimit and the most lines each may have.
// Lower these; never raise them. Move code out rather than adding a package here.
var oversizePackages = map[string]int{}

// layers orders the shipped packages lowest first. A package imports only from a layer below its
// own, never from its own layer or a higher one. A new package fails the test until it is given a
// place here (or in testSupport).
var layers = [][]string{
	// Foundation: no imports from this module.
	{"internal/auth", "internal/config", "internal/contracts", "internal/ratelimit", "internal/web"},
	// The source contract: what every source of runs, traces, logs and evidence implements.
	{"internal/source"},
	// The sources themselves.
	{"internal/source/agentpod", "internal/source/fake", "internal/source/logs", "internal/source/superpipeline", "internal/source/traces"},
	// A source derived from another one: errors are read out of the logs.
	{"internal/source/errors"},
	// The domain: verdicts and rubrics, telemetry, the canary, sign-in sessions.
	{"internal/verdicts", "internal/telemetry", "internal/canary", "internal/session"},
	// The domain over verdicts: the run registry and joining runs to their evidence.
	{"internal/runs", "internal/join"},
	// The HTTP API, then the MCP server built on it.
	{"internal/api"},
	{"internal/mcp"},
	// The composition root: wires sources, stores and servers together.
	{"internal/app"},
	// The binary.
	{"cmd/superwitness"},
}

// composers may import a concrete source package; everything else goes through the
// internal/source contract. A source package may import another source package below it.
var composers = map[string]bool{"internal/app": true, "cmd/superwitness": true}

// testSupport packages have shipped .go files but serve tests only: no shipped package imports
// them.
var testSupport = map[string]bool{"internal/testutil": true, "internal/docsclaims": true}

func moduleRoot() string { return filepath.Join("..", "..") }

// shippedDirs are the module's trees that hold Go code. web/ is left out: its node_modules can
// hold .go files that are not ours.
var shippedDirs = []string{"cmd", "internal"}

// goFiles calls fn for every .go file under the shipped trees, with its slash-separated path
// relative to root. testdata is skipped.
func goFiles(t *testing.T, root string, fn func(rel string, body []byte)) {
	t.Helper()
	fsys := os.DirFS(root)
	for _, top := range shippedDirs {
		err := fs.WalkDir(fsys, top, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") {
				return nil
			}
			body, err := fs.ReadFile(fsys, p)
			if err != nil {
				return err
			}
			fn(p, body)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func isTest(file string) bool { return strings.HasSuffix(file, "_test.go") }

// shippedSizes returns the line count of every shipped .go file.
func shippedSizes(t *testing.T, root string) map[string]int {
	t.Helper()
	sizes := map[string]int{}
	goFiles(t, root, func(rel string, body []byte) {
		if !isTest(rel) {
			sizes[rel] = bytes.Count(body, []byte("\n"))
		}
	})
	if len(sizes) == 0 {
		t.Fatal("found no shipped .go files; the walk is broken")
	}
	return sizes
}

// allowed is the most a name may have: its recorded size, or the limit.
func allowed(name string, limit int, recorded map[string]int) int {
	if n, ok := recorded[name]; ok {
		return n
	}
	return limit
}

// ratchet reports sizes over their limit and recorded sizes that have fallen.
func ratchet(sizes map[string]int, limit int, recorded map[string]int) (grown, stale []string) {
	for name, n := range sizes {
		if n > allowed(name, limit, recorded) {
			grown = append(grown, name)
		}
	}
	for name, size := range recorded {
		if sizes[name] < size {
			stale = append(stale, name)
		}
	}
	sort.Strings(grown)
	sort.Strings(stale)
	return grown, stale
}

func TestNoShippedFileGrowsPastItsLimit(t *testing.T) {
	sizes := shippedSizes(t, moduleRoot())
	grown, stale := ratchet(sizes, fileLimit, oversizeFiles)
	for _, f := range grown {
		t.Errorf("%s has %d lines; the limit is %d. Split it rather than raising the limit.", f, sizes[f], allowed(f, fileLimit, oversizeFiles))
	}
	for _, f := range stale {
		t.Errorf("%s shrank to %d lines (recorded %d); lower or remove its oversizeFiles entry", f, sizes[f], oversizeFiles[f])
	}
}

func TestNoPackageGrowsPastItsLimit(t *testing.T) {
	packages := map[string]int{}
	for file, n := range shippedSizes(t, moduleRoot()) {
		packages[path.Dir(file)] += n
	}
	grown, stale := ratchet(packages, packageLimit, oversizePackages)
	for _, p := range grown {
		t.Errorf("package %s has %d lines; the limit is %d. Move code out rather than raising the limit.", p, packages[p], allowed(p, packageLimit, oversizePackages))
	}
	for _, p := range stale {
		t.Errorf("package %s shrank to %d lines (recorded %d); lower or remove its oversizePackages entry", p, packages[p], oversizePackages[p])
	}
}

func TestRatchetSeesGrowthAndShrinkage(t *testing.T) {
	grown, stale := ratchet(
		map[string]int{"small": 10, "new-big": 30, "held": 50, "shrunk": 40},
		20,
		map[string]int{"held": 50, "shrunk": 45, "gone": 25},
	)
	if strings.Join(grown, ",") != "new-big" {
		t.Errorf("grown = %v, want [new-big]", grown)
	}
	if strings.Join(stale, ",") != "gone,shrunk" {
		t.Errorf("stale = %v, want [gone shrunk]", stale)
	}
}

// modulePath reads the module path from go.mod.
func modulePath(t *testing.T, root string) string {
	t.Helper()
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }() // read-only; a close error changes nothing
	s := bufio.NewScanner(f)
	for s.Scan() {
		if rest, ok := strings.CutPrefix(s.Text(), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatal("go.mod names no module")
	return ""
}

// graph holds the module-internal imports of each package, keyed by the importing package's
// directory, for shipped files and for test files separately.
type graph struct {
	shipped map[string]map[string]bool
	tests   map[string]map[string]bool
}

func add(m map[string]map[string]bool, from, to string) {
	if m[from] == nil {
		m[from] = map[string]bool{}
	}
	m[from][to] = true
}

func importGraph(t *testing.T, root string) graph {
	t.Helper()
	prefix := modulePath(t, root) + "/"
	g := graph{shipped: map[string]map[string]bool{}, tests: map[string]map[string]bool{}}
	fset := token.NewFileSet()
	goFiles(t, root, func(rel string, body []byte) {
		f, err := parser.ParseFile(fset, rel, body, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		pkg := path.Dir(rel)
		m := g.shipped
		if isTest(rel) {
			m = g.tests
		}
		if m[pkg] == nil {
			m[pkg] = map[string]bool{}
		}
		for _, spec := range f.Imports {
			p, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if to, ok := strings.CutPrefix(p, prefix); ok {
				add(m, pkg, to)
			}
		}
	})
	return g
}

func isSource(pkg string) bool { return strings.HasPrefix(pkg, "internal/source/") }

// violations checks a shipped import graph against the layers, the composers and the test
// support packages, and checks that no file at all imports cmd/.
func violations(g graph, layers [][]string, composers, testSupport map[string]bool) []string {
	rank := map[string]int{}
	for i, layer := range layers {
		for _, p := range layer {
			rank[p] = i
		}
	}
	var out []string
	for from, imports := range g.shipped {
		if testSupport[from] {
			continue
		}
		fromRank, placed := rank[from]
		if !placed {
			out = append(out, fmt.Sprintf("%s has no layer; give it a place in layers", from))
			continue
		}
		for to := range imports {
			switch {
			case testSupport[to]:
				out = append(out, fmt.Sprintf("%s imports %s, which serves tests only", from, to))
			case strings.HasPrefix(to, "cmd/"):
				out = append(out, fmt.Sprintf("%s imports %s; nothing imports cmd/", from, to))
			case !hasRank(rank, to):
				out = append(out, fmt.Sprintf("%s imports %s, which has no layer", from, to))
			case rank[to] >= fromRank:
				out = append(out, fmt.Sprintf("%s imports %s, which is not below it in layers", from, to))
			case isSource(to) && !isSource(from) && !composers[from]:
				out = append(out, fmt.Sprintf("%s imports the source %s; only the composition root may, the rest use internal/source", from, to))
			}
		}
	}
	for from, imports := range g.tests {
		for to := range imports {
			if strings.HasPrefix(to, "cmd/") && from != to {
				out = append(out, fmt.Sprintf("a test in %s imports %s; nothing imports cmd/", from, to))
			}
		}
	}
	sort.Strings(out)
	return out
}

func hasRank(rank map[string]int, p string) bool {
	_, ok := rank[p]
	return ok
}

func TestImportsPointDownTheLayers(t *testing.T) {
	g := importGraph(t, moduleRoot())
	if len(g.shipped) < len(layers) {
		t.Fatalf("found %d packages; the walk is broken", len(g.shipped))
	}
	for _, v := range violations(g, layers, composers, testSupport) {
		t.Error(v)
	}
}

func TestEveryLayeredPackageExists(t *testing.T) {
	g := importGraph(t, moduleRoot())
	for _, layer := range layers {
		for _, p := range layer {
			if _, ok := g.shipped[p]; !ok {
				t.Errorf("layers names %s, which has no shipped .go file; remove it", p)
			}
		}
	}
	for p := range testSupport {
		if _, ok := g.shipped[p]; !ok {
			t.Errorf("testSupport names %s, which has no shipped .go file; remove it", p)
		}
	}
}

func TestViolationsSeesEveryKindOfBadImport(t *testing.T) {
	ls := [][]string{{"internal/low"}, {"internal/source"}, {"internal/source/a", "internal/source/b"}, {"internal/mid"}, {"internal/app"}, {"cmd/bin"}}
	shipped := map[string]map[string]bool{
		"internal/low":      {"internal/mid": true},                             // upward
		"internal/source/a": {"internal/source/b": true},                        // same layer
		"internal/mid":      {"internal/source/a": true, "internal/util": true}, // a source; a test helper
		"internal/app":      {"internal/source/a": true, "cmd/bin": true},       // allowed; cmd/
		"internal/stray":    {},                                                 // no layer
		"internal/util":     {"internal/app": true},                             // test support is not checked
		"cmd/bin":           {"internal/app": true, "internal/ghost": true},     // allowed; unknown
	}
	tests := map[string]map[string]bool{"internal/low": {"cmd/bin": true}, "cmd/bin": {"cmd/bin": true}}
	got := violations(graph{shipped: shipped, tests: tests}, ls, map[string]bool{"internal/app": true}, map[string]bool{"internal/util": true})
	want := []string{
		"a test in internal/low imports cmd/bin; nothing imports cmd/",
		"cmd/bin imports internal/ghost, which has no layer",
		"internal/app imports cmd/bin; nothing imports cmd/",
		"internal/low imports internal/mid, which is not below it in layers",
		"internal/mid imports internal/util, which serves tests only",
		"internal/mid imports the source internal/source/a; only the composition root may, the rest use internal/source",
		"internal/source/a imports internal/source/b, which is not below it in layers",
		"internal/stray has no layer; give it a place in layers",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("violations:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
