package docsclaims

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// docs/code-quality.md states the pinned linter versions, the size limits, the linters shipped
// code must stay clean of and the package layers. Each claim is read from the page and compared
// with the file that enforces it, so the page cannot drift from the checks it describes.

const qualityPage = "docs/code-quality.md"

func readRepo(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// find returns the first submatch of re in text, failing the test when there is none.
func find(t *testing.T, where, text, re string) string {
	t.Helper()
	m := regexp.MustCompile(re).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("%s: no match for %s; the extractor is broken", where, re)
	}
	return m[1]
}

func TestQualityPageNamesThePinnedLinters(t *testing.T) {
	page := readRepo(t, qualityPage)
	makefile := readRepo(t, "Makefile")
	script := readRepo(t, "scripts/lint-go.mjs")

	doc := find(t, qualityPage, page, "golangci-lint\\]\\([^)]*\\) `v([0-9.]+)`")
	if got := find(t, "Makefile", makefile, `(?m)^GOLANGCI_LINT_VERSION = ([0-9.]+)$`); got != doc {
		t.Errorf("Makefile pins golangci-lint %s; %s says %s", got, qualityPage, doc)
	}
	if got := find(t, "scripts/lint-go.mjs", script, `const VERSION = '([0-9.]+)'`); got != doc {
		t.Errorf("scripts/lint-go.mjs pins golangci-lint %s; %s says %s", got, qualityPage, doc)
	}

	var pkg struct {
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(readRepo(t, "web/package.json")), &pkg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"oxlint", "oxlint-tsgolint"} {
		claim := regexp.QuoteMeta(name)
		if name == "oxlint" {
			claim = `oxlint\]\([^)]*\)`
		} else {
			claim = "`" + claim + "`"
		}
		doc := find(t, qualityPage, page, claim+" `([0-9.]+)`")
		if got := pkg.DevDependencies[name]; got != doc {
			t.Errorf("web/package.json pins %s %q; %s says %s (pin it exactly)", name, got, qualityPage, doc)
		}
	}
}

func TestQualityPageStatesTheSizeLimits(t *testing.T) {
	page := readRepo(t, qualityPage)
	arch := readRepo(t, "internal/archtest/archtest_test.go")
	m := regexp.MustCompile(`([0-9,]+) lines per file and ([0-9,]+) lines per package`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("%s states no size limits; the extractor is broken", qualityPage)
	}
	for i, name := range []string{"fileLimit", "packageLimit"} {
		want := find(t, "internal/archtest", arch, `(?m)^\s*`+name+` = ([0-9]+)$`)
		if got := strings.ReplaceAll(m[i+1], ",", ""); got != want {
			t.Errorf("%s says %s is %s; internal/archtest has %s", qualityPage, name, got, want)
		}
	}
}

func TestQualityPageListsTheShippedCleanLinters(t *testing.T) {
	page := readRepo(t, qualityPage)
	script := readRepo(t, "scripts/lint-go.mjs")
	sentence := find(t, qualityPage, page, `(?s)\*\*Shipped code stays clean\*\* of (.*?)\(`+"`SHIPPED_CLEAN`")
	set := find(t, "scripts/lint-go.mjs", script, `SHIPPED_CLEAN = new Set\(\[([^\]]*)\]`)
	doc := quoted(sentence, "`")
	code := quoted(set, "'")
	if strings.Join(doc, ",") != strings.Join(code, ",") {
		t.Errorf("%s lists %v as clean in shipped code; scripts/lint-go.mjs has %v", qualityPage, doc, code)
	}
}

// quoted returns the strings between pairs of q in text, sorted.
func quoted(text, q string) []string {
	var out []string
	for _, m := range regexp.MustCompile(regexp.QuoteMeta(q)+`([^`+q+`]+)`+regexp.QuoteMeta(q)).FindAllStringSubmatch(text, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

func TestQualityPageLayersMatchTheArchitectureTest(t *testing.T) {
	page := readRepo(t, qualityPage)
	arch := readRepo(t, "internal/archtest/archtest_test.go")

	// The layers in the test: one {"…", "…"} line per layer inside `var layers = [][]string{`.
	body := find(t, "internal/archtest", arch, `(?s)var layers = \[\]\[\]string\{\n(.*?)\n\}`)
	var code []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var pkgs []string
		for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(line, -1) {
			pkgs = append(pkgs, strings.TrimPrefix(m[1], "internal/"))
		}
		code = append(code, strings.Join(pkgs, ","))
	}

	// The layers on the page: the second column of the table under "Import direction".
	table := find(t, qualityPage, page, `(?s)\| Layer \| Packages under `+"`internal/`"+` \|\n\|---\|---\|\n(.*?)\n\n`)
	var doc []string
	for _, row := range strings.Split(table, "\n") {
		cells := strings.Split(row, "|")
		if len(cells) < 3 {
			t.Fatalf("%s: malformed layer row %q", qualityPage, row)
		}
		var pkgs []string
		for _, m := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(cells[2], -1) {
			if m[1] != "internal/" {
				pkgs = append(pkgs, m[1])
			}
		}
		doc = append(doc, strings.Join(pkgs, ","))
	}
	if len(code) == 0 {
		t.Fatal("found no layers in internal/archtest; the extractor is broken")
	}
	if strings.Join(doc, "\n") != strings.Join(code, "\n") {
		t.Errorf("%s layers:\n%s\ninternal/archtest layers:\n%s", qualityPage, numbered(doc), numbered(code))
	}
}

func numbered(xs []string) string {
	var b strings.Builder
	for i, x := range xs {
		b.WriteString("  " + strconv.Itoa(i) + ": " + x + "\n")
	}
	return b.String()
}
