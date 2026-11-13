package docsclaims

import (
	"strings"
	"testing"
)

func TestRatchetDirections(t *testing.T) {
	named := func(s string) bool { return s == "documented" || s == "documented-and-listed" }
	got := Ratchet("x", []string{"documented", "listed", "new", "documented-and-listed"}, named,
		[]string{"listed", "documented-and-listed", "gone"})
	want := []string{
		`x: "documented-and-listed" is documented now; remove it`,
		`x: "gone" is in docs/undocumented-baseline.json but the code no longer defines it`,
		`x: "new" is on no docs page`,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d violations, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if !strings.HasPrefix(got[i], w) {
			t.Errorf("violation %d = %q, want prefix %q", i, got[i], w)
		}
	}
}

func TestRouteNamed(t *testing.T) {
	doc := "| `GET` | `/v1/runs/superpipeline/{board}/{run}` | x |\n`POST /v1/runs` takes one report. The `/mcp` endpoint. `GET /` (home)"
	for route, want := range map[string]bool{
		"GET /v1/runs/superpipeline/{boardId}/{runId}": true, // placeholder names may differ
		"POST /v1/runs":                  true,
		"GET /v1/runs":                   false, // POST is named, GET is not
		"GET /v1/runs/superpipeline/{b}": false, // a prefix of a named path is not named
		"ANY /mcp":                       true,
		"GET /":                          true,
		"GET /rubrics":                   false,
	} {
		if got := RouteNamed(doc, route); got != want {
			t.Errorf("RouteNamed(%q) = %v, want %v", route, got, want)
		}
	}
}

func TestEnvAndWordNamed(t *testing.T) {
	doc := "Set `SW_LOGS_URL`; `BIN_DIR=` moves it, `VERSION=` pins it. Logged as `auth.signin_refused`."
	for name, want := range map[string]bool{
		"SW_LOGS_URL": true, "SW_LOGS": false, "BIN_DIR": true, "VERSION": true, "LOGS": false,
	} {
		if got := EnvNamed(doc, name); got != want {
			t.Errorf("EnvNamed(%q) = %v, want %v", name, got, want)
		}
	}
	if WordNamed(doc, "auth.signin") || !WordNamed(doc, "auth.signin_refused") {
		t.Error("WordNamed must match a log event whole, not as the start of a longer one")
	}
}

func TestCommandAndFlagNamed(t *testing.T) {
	pages := []string{
		"Run `superwitness canary run --trigger nightly`.",
		"`superwitness canary status` prints. With `-json` it is JSON.",
		"`--results` is somewhere else.",
	}
	if !CommandNamed(pages[0], "canary run") || !CommandNamed(pages[0], "canary") || CommandNamed(pages[0], "canary status") {
		t.Error("CommandNamed is wrong")
	}
	for item, want := range map[string]bool{
		"canary run -trigger": true, "canary status -json": true,
		"canary run -results":    false, // named, but not on a page that names the command
		"canary run -force-fail": false,
	} {
		if got := FlagNamed(pages, item); got != want {
			t.Errorf("FlagNamed(%q) = %v, want %v", item, got, want)
		}
	}
}
