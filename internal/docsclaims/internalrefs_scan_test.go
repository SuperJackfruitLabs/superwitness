package docsclaims

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This guard keeps internal identifiers of private infrastructure out of the
// tracked tree (the repo is public). Pointer families are Go RE2 regexes.
// Private host and repository names are checked separately: each line is split
// into [A-Za-z0-9]+ tokens, lowercased and hashed, so the match is whole-token
// and case-insensitive, and this file only holds digests of the names.
//
// Deliberately NOT forbidden:
//   - /etc/superwitness: it is the generic install path in the README, docs
//     and deploy units, so a path ban would need a huge allowlist. Naming a
//     specific host is already caught by the private-name check.
//   - hub.agentpod.dev, "tailnet address", "Tailnet-first", ntfy wording.
//   - Obviously fake fixture ids such as prn_grader01 or brd_01. Only ids
//     that look machine-generated (16+ alphanumerics after the prefix, e.g.
//     ULIDs) are flagged by the real-looking-id pattern.

type internalPattern struct {
	name string
	re   *regexp.Regexp
}

var internalPatterns = []internalPattern{
	{"tailnet-ip", regexp.MustCompile(`\b100\.\d+\.\d+\.\d+\b`)},
	{"ts-net", regexp.MustCompile(`\.ts\.net\b`)},
	{"ruling", regexp.MustCompile(`[Rr]uling R?\d`)},
	{"per-ruling-id", regexp.MustCompile(`Per R?\d+\b|PerR\d+\b`)},
	{"task-n", regexp.MustCompile(`\bTask \d`)},
	{"workstream", regexp.MustCompile(`\bws[1-6]\b`)},
	{"milestone", regexp.MustCompile(`[Mm]ilestone \d`)},
	{"spec-section", regexp.MustCompile(`spec §`)},
	{"section-sign", regexp.MustCompile(`§\d`)},
	{"plan-index", regexp.MustCompile(`plan index`)},
	{"contract-c", regexp.MustCompile(`contract C\d`)},
	{"index-c", regexp.MustCompile(`\bindex C\d`)},
	{"gap-g", regexp.MustCompile(`\bgap G\d`)},
	{"review-focus", regexp.MustCompile(`[Rr]eview [Ff]ocus`)},
	{"real-looking-id", regexp.MustCompile(`\b(?:prn|brd|svc)_[0-9A-Za-z]{16,}\b`)},
}

// privateNameDigests is sha256 of lowercase private host and repository
// names; kept as digests so this file does not publish them.
var privateNameDigests = map[string]bool{
	"dfb316701857783dac69a14d1fe3fd60cff21d56e830baf7f0e3871bd73eee39": true,
	"eb0de251ac21fb6753f04bc96527b0ff0686b585de75f924aedd7ba90502d6b4": true,
	"5a20fe2f1b70d1150712b032c84f5998ac797d1bd9c84ed2ada77866ea030287": true,
	"61bf1411aace686f0e1fe80cde6f609bf40651896c2afd3a50ba6ce67ab46faf": true,
	"e62377847bbe2117fbea3a09ac72e59a12572c20ca18ee024fedeaa94ba8525a": true,
	"8399e57405627368722830c9ff3db81fba0afa6120a439c7a2568a5be31b8295": true,
	"2837c4003284e58aef92ab6a3127d3f78e6bd241479f58b65acc124752231af2": true,
}

const privateNamePattern = "private-name"

var tokenRE = regexp.MustCompile(`[A-Za-z0-9]+`)

// tokenDigest is the hex sha256 of the lowercased token.
func tokenDigest(tok string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(tok)))
	return hex.EncodeToString(sum[:])
}

type internalRef struct {
	pattern string // pattern name
	match   string // the text that matched
}

// scanLine returns every forbidden match on line, checking whole tokens
// against the given private-name digest set.
func scanLine(line string, digests map[string]bool) []internalRef {
	var out []internalRef
	for _, tok := range tokenRE.FindAllString(line, -1) {
		if digests[tokenDigest(tok)] {
			out = append(out, internalRef{privateNamePattern, tok})
		}
	}
	for _, p := range internalPatterns {
		if loc := p.re.FindStringIndex(line); loc != nil {
			out = append(out, internalRef{p.name, line[loc[0]:loc[1]]})
		}
	}
	return out
}

// findInternalRefs returns the names of every pattern that matches line.
func findInternalRefs(line string) []string {
	var out []string
	for _, r := range scanLine(line, privateNameDigests) {
		out = append(out, r.pattern)
	}
	return out
}

type allowEntry struct {
	glob    string // slash-separated path.Match glob against the repo-relative path
	pattern string // pattern name, or "*" for all
	reason  string
}

var internalAllowlist = []allowEntry{
	{"internal/docsclaims/internalrefs*_test.go", "*", "the guard itself spells out the forbidden patterns"},
}

func allowed(file, pattern string) bool {
	for _, a := range internalAllowlist {
		if ok, _ := path.Match(a.glob, file); ok && (a.pattern == "*" || a.pattern == pattern) {
			return true
		}
	}
	return false
}

func skipFile(file string) bool {
	base := path.Base(file)
	return base == "package-lock.json" || strings.HasPrefix(file, "internal/web/dist/")
}

// TestNoInternalIdentifiersInTrackedTree scans every tracked text file.
func TestNoInternalIdentifiersInTrackedTree(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files failed (git is required for this guard): %v", err)
	}
	var problems []string
	for _, file := range strings.Split(string(out), "\x00") {
		if file == "" || skipFile(file) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			continue // tracked but deleted in the worktree
		}
		if bytes.IndexByte(data, 0) >= 0 {
			continue // binary
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, r := range scanLine(line, privateNameDigests) {
				if !allowed(file, r.pattern) && !syntheticExecutionRef(file, data, r) {
					problems = append(problems, fmt.Sprintf("%s:%d: [%s] %q", file, i+1, r.pattern, r.match))
				}
			}
		}
	}
	if len(problems) > 0 {
		t.Errorf("internal identifiers in tracked files:\n%s", strings.Join(problems, "\n"))
	}
}

// The canonical cross-language evidence corpus requires principal syntax. Only
// these reviewed synthetic principals in these exact bytes get this exception.
const executionFixturePath = "internal/source/testdata/execution-evidence-v1.json"
const executionFixtureDigest = "0c29ac94d21fcef7bc854a2fbf0b7d81d8261d77162ae530f51de0799120e96f"

func syntheticExecutionRef(file string, data []byte, r internalRef) bool {
	expected := map[string]string{
		executionFixturePath: executionFixtureDigest,
		"internal/source/testdata/execution-export-v1.json": "6af4eebb36ecd99258ed5e9d7049a86163b6c1e7cbb3bd4500491902e7ff0de1",
	}[file]
	if expected == "" || r.pattern != "real-looking-id" {
		return false
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != expected {
		return false
	}
	return r.match == "prn_"+strings.Repeat("a", 20) || r.match == "prn_"+strings.Repeat("b", 20)
}
func TestExecutionFixtureExceptionIsExact(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), executionFixturePath))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != executionFixtureDigest {
		t.Fatal("shared execution corpus bytes changed; review canonical mirror and synthetic identity exception")
	}
	refs := 0
	for _, line := range strings.Split(string(b), "\n") {
		for _, r := range scanLine(line, privateNameDigests) {
			refs++
			if !syntheticExecutionRef(executionFixturePath, b, r) {
				t.Fatalf("non-synthetic reference: %s", r.match)
			}
		}
	}
	if refs == 0 {
		t.Fatal("fixture no longer exercises synthetic exception")
	}
	for _, tc := range []struct {
		file string
		data []byte
		r    internalRef
	}{
		{"other.json", b, internalRef{"real-looking-id", "prn_" + strings.Repeat("a", 20)}},
		{executionFixturePath, append(append([]byte{}, b...), ' '), internalRef{"real-looking-id", "prn_" + strings.Repeat("a", 20)}},
		{executionFixturePath, b, internalRef{"real-looking-id", "prn_0123456789abcdef0123"}},
		{executionFixturePath, b, internalRef{"tailnet-ip", "100.1.2.3"}},
	} {
		if syntheticExecutionRef(tc.file, tc.data, tc.r) {
			t.Fatal("exception accepted non-reviewed identity, bytes or path")
		}
	}
}

func TestExportFixtureExceptionIsExact(t *testing.T) {
	path := "internal/source/testdata/execution-export-v1.json"
	b, err := os.ReadFile(filepath.Join(repoRoot(t), path))
	if err != nil {
		t.Fatal(err)
	}
	refs := 0
	for _, line := range strings.Split(string(b), "\n") {
		for _, r := range scanLine(line, privateNameDigests) {
			refs++
			if !syntheticExecutionRef(path, b, r) {
				t.Fatal("export corpus exception requires reviewed exact bytes and synthetic identity")
			}
		}
	}
	if refs == 0 {
		t.Fatal("missing synthetic export identity")
	}
	if syntheticExecutionRef(path, append(append([]byte{}, b...), ' '), internalRef{"real-looking-id", "prn_" + strings.Repeat("a", 20)}) {
		t.Fatal("changed corpus bytes accepted")
	}
	if syntheticExecutionRef(path, b, internalRef{"real-looking-id", "prn_0123456789abcdef0123"}) {
		t.Fatal("non-synthetic identity accepted")
	}
}
