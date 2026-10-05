// Package docsclaims checks what the published docs claim against what the code defines.
//
// The docs site (docs-site/) and the landing page (landing/) are prose about this
// binary. Prose drifts silently: a renamed variable or route leaves a page that is
// wrong and nothing fails. The tests here read the pages and fail when they name an
// SW_* variable internal/config does not read, a /v1 route the API does not register,
// or an MCP tool the server does not register, or when a docs page lacks a title or
// description.
package docsclaims

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	envRe = regexp.MustCompile(`\bSW_[A-Z0-9_]+\b`)
	// A /v1/ token is a superwitness route claim only when it starts a token:
	// after start of line, whitespace, a backtick, a quote, "(", "[", or a ":<port>".
	// A /v1/ after another path segment or a "{placeholder}" belongs to another
	// product (the collector's /insert/opentelemetry/v1/traces, superpipeline's
	// {SW_SUPERPIPELINE_URL}/v1/boards/...), so it is not matched.
	routeRe = regexp.MustCompile("(?m)(?:^|[\\s`'\"(\\[]|:[0-9]+)(/v1(?:/[A-Za-z0-9_{}.:\\-]+)+)")
	tickRe  = regexp.MustCompile("`([^`\n]+)`")
	toolRe  = regexp.MustCompile(`^(?:get|list|record|create|update|delete|set|search|find|add|remove)_[a-z0-9_]+$`)
	sqlRe   = regexp.MustCompile("(?s)<!-- sql:([a-z0-9-]+) -->\\s*```sql\\n(.*?)```")
	fmRe    = regexp.MustCompile(`(?s)\A---\n(.*?)\n---\n`)
)

type Registry struct {
	EnvVars map[string]bool
	Route   func(path string) bool
	Tools   map[string]bool
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func EnvVars(text string) []string { return uniq(envRe.FindAllString(text, -1)) }

// Routes returns the superwitness /v1 route claims in text, query stripped and
// trailing punctuation trimmed. See routeRe for what counts as a claim.
func Routes(text string) []string {
	var out []string
	for _, m := range routeRe.FindAllStringSubmatch(text, -1) {
		out = append(out, strings.TrimRight(m[1], ".,:;"))
	}
	return uniq(out)
}

func ToolNames(text string) []string {
	var out []string
	for _, m := range tickRe.FindAllStringSubmatch(text, -1) {
		if toolRe.MatchString(m[1]) {
			out = append(out, m[1])
		}
	}
	return uniq(out)
}

func Frontmatter(text string) (title, description string, ok bool) {
	m := fmRe.FindStringSubmatch(text)
	if m == nil {
		return "", "", false
	}
	for _, line := range strings.Split(m[1], "\n") {
		k, v, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `'"`)
		switch strings.TrimSpace(k) {
		case "title":
			title = v
		case "description":
			description = v
		}
	}
	return title, description, true
}

func SQLBlocks(text string) map[string]string {
	out := map[string]string{}
	for _, m := range sqlRe.FindAllStringSubmatch(text, -1) {
		out[m[1]] = m[2]
	}
	return out
}

func Check(page, text string, r Registry, needFrontmatter bool) []string {
	var v []string
	if needFrontmatter {
		title, desc, _ := Frontmatter(text)
		if title == "" {
			v = append(v, fmt.Sprintf("%s: no title in frontmatter", page))
		}
		if desc == "" {
			v = append(v, fmt.Sprintf("%s: no description in frontmatter", page))
		}
	}
	for _, e := range EnvVars(text) {
		if !r.EnvVars[e] {
			v = append(v, fmt.Sprintf("%s: %s is not read by internal/config", page, e))
		}
	}
	for _, p := range Routes(text) {
		if !r.Route(p) {
			v = append(v, fmt.Sprintf("%s: %s is not a route the API registers", page, p))
		}
	}
	for _, t := range ToolNames(text) {
		if !r.Tools[t] {
			v = append(v, fmt.Sprintf("%s: %s is not an MCP tool the server registers", page, t))
		}
	}
	sort.Strings(v)
	return v
}
