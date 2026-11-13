package docsclaims

// Completeness, the code-to-docs direction. Check above fails when a page names something
// the code does not define; the ratchet here fails when the code defines something no page
// names. Items no page names yet are listed in docs/undocumented-baseline.json, which can
// only shrink: an item that a page now names, or that the code no longer defines, must leave
// it. A new route, setting, command, flag, tool or log event must arrive with its docs.

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Surfaces, in the order the baseline file lists them.
const (
	SurfaceRoutes   = "http_routes"
	SurfaceEnv      = "env_vars"
	SurfaceCommands = "cli_commands"
	SurfaceFlags    = "cli_flags"
	SurfaceTools    = "mcp_tools"
	SurfaceLogs     = "log_events"
)

var Surfaces = []string{SurfaceRoutes, SurfaceEnv, SurfaceCommands, SurfaceFlags, SurfaceTools, SurfaceLogs}

// Baseline is the committed list of items no page names yet, per surface.
type Baseline map[string][]string

func LoadBaseline(path string) (Baseline, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bl Baseline
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&bl); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for k := range bl {
		if !knownSurface(k) {
			return nil, fmt.Errorf("%s: unknown surface %q", path, k)
		}
	}
	return bl, nil
}

func knownSurface(s string) bool {
	for _, k := range Surfaces {
		if k == s {
			return true
		}
	}
	return false
}

// Ratchet compares one surface's items, as the code defines them, with the baseline. It
// reports an item no page names that the baseline does not hold, a baseline item a page now
// names, and a baseline item the code no longer defines.
func Ratchet(surface string, items []string, named func(string) bool, baseline []string) []string {
	inBase := map[string]bool{}
	for _, b := range baseline {
		inBase[b] = true
	}
	inCode := map[string]bool{}
	var v []string
	for _, it := range items {
		inCode[it] = true
		switch doc := named(it); {
		case !doc && !inBase[it]:
			v = append(v, fmt.Sprintf("%s: %q is on no docs page; document it (docs-site/src/content/docs)", surface, it))
		case doc && inBase[it]:
			v = append(v, fmt.Sprintf("%s: %q is documented now; remove it from docs/undocumented-baseline.json", surface, it))
		}
	}
	for _, b := range baseline {
		if !inCode[b] {
			v = append(v, fmt.Sprintf("%s: %q is in docs/undocumented-baseline.json but the code no longer defines it; remove it", surface, b))
		}
	}
	sort.Strings(v)
	return v
}

// Undocumented lists the items no page names, sorted: what the baseline should hold.
func Undocumented(items []string, named func(string) bool) []string {
	out := []string{}
	for _, it := range uniq(items) {
		if !named(it) {
			out = append(out, it)
		}
	}
	sort.Strings(out)
	return out
}

// Naming rules. Each takes the docs as one text: every page joined.

// prose is the docs with backticks and table bars made spaces, so "`GET` | `/v1/runs`" in a
// table and "`GET /v1/runs`" in a sentence read the same.
func prose(text string) string {
	return strings.NewReplacer("`", " ", "|", " ").Replace(text)
}

const pathChar = `A-Za-z0-9_{}/.\-`

var placeholder = regexp.MustCompile(`\\\{[^}]*\\\}`)

// pathPattern matches path p with any placeholder names: chi's {boardId} is the docs' {board}.
func pathPattern(p string) string {
	return placeholder.ReplaceAllString(regexp.QuoteMeta(p), `\{[^}\s]+\}`)
}

// RouteNamed reports whether the docs name a route, "METHOD /path". A route of method ANY
// is named by its path alone.
func RouteNamed(text, route string) bool {
	method, path, ok := strings.Cut(route, " ")
	if !ok {
		return false
	}
	pre := `(?:^|[^` + pathChar + `])`
	if method != "ANY" {
		pre = `(?:^|\s)` + regexp.QuoteMeta(method) + `\s+`
	}
	re := regexp.MustCompile(pre + pathPattern(path) + `(?:$|[^` + pathChar + `])`)
	return re.MatchString(prose(text))
}

// WordNamed reports whether the docs name w as a whole token: not inside a longer name.
func WordNamed(text, w string) bool {
	return regexp.MustCompile(`(?:^|[^A-Za-z0-9_.\-])` + regexp.QuoteMeta(w) + `(?:$|[^A-Za-z0-9_\-])`).MatchString(text)
}

// EnvNamed reports whether the docs name a variable. A name with no underscore, such as the
// installer's VERSION, is an ordinary word too, so it counts only backticked or as NAME=.
func EnvNamed(text, name string) bool {
	if strings.Contains(name, "_") {
		return WordNamed(text, name)
	}
	q := regexp.QuoteMeta(name)
	return regexp.MustCompile("`" + q + "`|(?:^|[^A-Za-z0-9_])" + q + "=").MatchString(text)
}

// CommandNamed reports whether the docs name a command as typed: "superwitness <cmd>" or
// "suwi <cmd>".
func CommandNamed(text, cmd string) bool {
	q := strings.ReplaceAll(regexp.QuoteMeta(cmd), " ", `\s+`)
	return regexp.MustCompile(`(?:superwitness|suwi)\s+` + q + `(?:$|[^A-Za-z0-9_\-])`).MatchString(prose(text))
}

// FlagNamed reports whether one page names both a command and its flag, "-name" or "--name".
// item is "<command> -<name>".
func FlagNamed(pages []string, item string) bool {
	i := strings.LastIndex(item, " -")
	if i < 0 {
		return false
	}
	cmd, flag := item[:i], strings.TrimLeft(item[i+1:], "-")
	re := regexp.MustCompile(`(?:^|[\s\x60(\[])--?` + regexp.QuoteMeta(flag) + `(?:$|[^A-Za-z0-9_\-])`)
	for _, p := range pages {
		if CommandNamed(p, cmd) && re.MatchString(p) {
			return true
		}
	}
	return false
}
