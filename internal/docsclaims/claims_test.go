package docsclaims

import (
	"strings"
	"testing"
)

func testRegistry() Registry {
	return Registry{
		EnvVars: map[string]bool{"SW_LISTEN": true},
		Route:   func(p string) bool { return p == "/v1/verdicts" },
		Tools:   map[string]bool{"get_run": true},
	}
}

const good = "---\ntitle: T\ndescription: D\n---\nSet `SW_LISTEN`, POST `/v1/verdicts`, call `get_run`.\n"

func TestGoodPagePasses(t *testing.T) {
	if v := Check("p.md", good, testRegistry(), true); len(v) != 0 {
		t.Fatalf("want no violations, got %v", v)
	}
}

func TestUnknownEnvVarFails(t *testing.T) {
	v := Check("p.md", good+"Also `SW_NOPE`.\n", testRegistry(), true)
	if len(v) != 1 || !strings.Contains(v[0], "SW_NOPE") {
		t.Fatalf("got %v", v)
	}
}

func TestCanaryVarsAreNotSWVars(t *testing.T) {
	if got := EnvVars("SWC_BOARD_ID and SW_LISTEN"); len(got) != 1 || got[0] != "SW_LISTEN" {
		t.Fatalf("got %v", got)
	}
}

func TestUnknownRouteFails(t *testing.T) {
	v := Check("p.md", good+"GET `/v1/nope/{id}`.\n", testRegistry(), true)
	if len(v) != 1 || !strings.Contains(v[0], "/v1/nope/{id}") {
		t.Fatalf("got %v", v)
	}
}

func TestUnknownToolFails(t *testing.T) {
	v := Check("p.md", good+"Call `list_everything`.\n", testRegistry(), true)
	if len(v) != 1 || !strings.Contains(v[0], "list_everything") {
		t.Fatalf("got %v", v)
	}
}

func TestMissingTitleFails(t *testing.T) {
	v := Check("p.md", "---\ndescription: D\n---\nbody\n", testRegistry(), true)
	if len(v) != 1 || !strings.Contains(v[0], "title") {
		t.Fatalf("got %v", v)
	}
}

func TestMissingDescriptionFails(t *testing.T) {
	v := Check("p.md", "---\ntitle: T\n---\nbody\n", testRegistry(), true)
	if len(v) != 1 || !strings.Contains(v[0], "description") {
		t.Fatalf("got %v", v)
	}
}

func TestRouteExtraction(t *testing.T) {
	got := Routes("curl localhost:8790/v1/runs/superpipeline/brd_01/run_01/spans?limit=50, then `/v1/verdicts`.")
	want := []string{"/v1/runs/superpipeline/brd_01/run_01/spans", "/v1/verdicts"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestRouteExemptAfterPathSegment(t *testing.T) {
	// Another product's route: the collector's VictoriaLogs ingest path.
	if got := Routes("POST http://victorialogs:9428/insert/opentelemetry/v1/logs"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestRouteExemptAfterPlaceholder(t *testing.T) {
	// Superpipeline's route, addressed through a placeholder base URL.
	if got := Routes("GET {SW_SUPERPIPELINE_URL}/v1/boards/{board}/runs/{run}/evidence"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestRouteCountedAfterPort(t *testing.T) {
	for _, s := range []string{"curl localhost:8790/v1/nope", "curl http://127.0.0.1:8790/v1/nope"} {
		if got := Routes(s); len(got) != 1 || got[0] != "/v1/nope" {
			t.Fatalf("%q: got %v", s, got)
		}
	}
}

func TestRouteCountedAfterBacktickQuoteParenBracket(t *testing.T) {
	for _, s := range []string{"`/v1/nope`", "'/v1/nope'", `"/v1/nope"`, "(/v1/nope)", "[/v1/nope]", "/v1/nope", "GET /v1/nope"} {
		if got := Routes(s); len(got) != 1 || got[0] != "/v1/nope" {
			t.Fatalf("%q: got %v", s, got)
		}
	}
}

func TestSQLBlocks(t *testing.T) {
	got := SQLBlocks("<!-- sql:a -->\n```sql\nSELECT 1;\n```\n")
	if got["a"] != "SELECT 1;\n" {
		t.Fatalf("got %q", got["a"])
	}
}
