package runs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/SuperJackfruitLabs/superwitness/internal/config"
	"github.com/SuperJackfruitLabs/superwitness/internal/contracts"
)

// report builds one report body from a base with edits; a nil value deletes the key.
func report(edits map[string]any) string {
	m := map[string]any{
		"source": "superpipeline", "external_ref": "brd_01/run_01", "status": "running",
		"source_status": "in_progress", "reported_at": "2026-10-06T10:00:00Z",
	}
	for k, v := range edits {
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func batchOf(n int) string {
	items := make([]string, n)
	for i := range items {
		items[i] = report(map[string]any{"external_ref": fmt.Sprintf("brd_01/run_%03d", i)})
	}
	return `{"runs":[` + strings.Join(items, ",") + `]}`
}

type null struct{} // marshals as JSON null, to send a key as null rather than leave it out

func (null) MarshalJSON() ([]byte, error) { return []byte("null"), nil }

// fakePrincipal is a principal id of the hub's shape, built at run time so that no real-looking
// id is written in a tracked file.
var fakePrincipal = "prn_" + strings.Repeat("0a", 10)

var fixtures = []struct {
	name  string
	body  string
	valid bool
}{
	{"minimal", report(nil), true},
	{"full", report(map[string]any{"scope": map[string]any{"id": "brd_01", "name": "Press"}, "title": "Draft the release note",
		"executor": map[string]any{"id": fakePrincipal, "name": "drafter"}, "status": "succeeded", "source_status": "done",
		"started_at": "2026-10-06T09:00:00.123Z", "ended_at": "2026-10-06T09:05:00+05:30"}), true},
	{"null started_at and ended_at", report(map[string]any{"started_at": null{}, "ended_at": null{}}), true},
	{"scope without a name", report(map[string]any{"scope": map[string]any{"id": "brd_01"}}), true},
	{"executor without an id", report(map[string]any{"executor": map[string]any{"name": "drafter"}}), true},
	{"title of 200 two-byte characters", report(map[string]any{"title": strings.Repeat("é", 200)}), true},
	{"external_ref of 256", report(map[string]any{"external_ref": strings.Repeat("r", 256)}), true},
	{"source of one letter", report(map[string]any{"source": "a"}), true},
	{"source of 32", report(map[string]any{"source": "a" + strings.Repeat("b", 31)}), true},
	{"milliseconds", report(map[string]any{"reported_at": "2026-10-06T10:00:00.001Z"}), true},
	{"batch of one", batchOf(1), true},
	{"batch of 100", batchOf(100), true},

	{"no source", report(map[string]any{"source": nil}), false},
	{"no external_ref", report(map[string]any{"external_ref": nil}), false},
	{"no status", report(map[string]any{"status": nil}), false},
	{"no source_status", report(map[string]any{"source_status": nil}), false},
	{"no reported_at", report(map[string]any{"reported_at": nil}), false},
	{"null status", report(map[string]any{"status": null{}}), false},
	{"null title", report(map[string]any{"title": null{}}), false},
	{"null scope", report(map[string]any{"scope": null{}}), false},
	{"null executor", report(map[string]any{"executor": null{}}), false},
	{"null reported_at", report(map[string]any{"reported_at": null{}}), false},
	{"source in capitals", report(map[string]any{"source": "Superpipeline"}), false},
	{"source starting with a digit", report(map[string]any{"source": "9pipeline"}), false},
	{"source of 33", report(map[string]any{"source": "a" + strings.Repeat("b", 32)}), false},
	{"empty external_ref", report(map[string]any{"external_ref": ""}), false},
	{"external_ref of 257", report(map[string]any{"external_ref": strings.Repeat("r", 257)}), false},
	{"unknown field", report(map[string]any{"colour": "green"}), false},
	{"field name in another case", strings.Replace(report(nil), `"source":`, `"Source":`, 1), false},
	{"optional field name in another case", report(map[string]any{"Title": "Draft"}), false},
	{"status not in the list", report(map[string]any{"status": "done"}), false},
	{"empty source_status", report(map[string]any{"source_status": ""}), false},
	{"source_status of 65", report(map[string]any{"source_status": strings.Repeat("s", 65)}), false},
	{"empty title", report(map[string]any{"title": ""}), false},
	{"title of 201", report(map[string]any{"title": strings.Repeat("t", 201)}), false},
	{"title as a number", report(map[string]any{"title": 7}), false},
	{"scope without an id", report(map[string]any{"scope": map[string]any{"name": "Press"}}), false},
	{"scope with a null name", report(map[string]any{"scope": map[string]any{"id": "brd_01", "name": null{}}}), false},
	{"scope with an extra key", report(map[string]any{"scope": map[string]any{"id": "brd_01", "kind": "board"}}), false},
	{"scope as a string", report(map[string]any{"scope": "brd_01"}), false},
	{"executor without a name", report(map[string]any{"executor": map[string]any{"id": fakePrincipal}}), false},
	{"executor id not a principal", report(map[string]any{"executor": map[string]any{"id": "agt_01", "name": "drafter"}}), false},
	{"executor id in capitals", report(map[string]any{"executor": map[string]any{"id": strings.ToUpper(fakePrincipal), "name": "d"}}), false},
	{"executor name of 201", report(map[string]any{"executor": map[string]any{"name": strings.Repeat("n", 201)}}), false},
	{"started_at in words", report(map[string]any{"started_at": "yesterday"}), false},
	{"started_at with a comma fraction", report(map[string]any{"started_at": "2026-10-06T10:00:00,5Z"}), false},
	{"an offset hour of 24", report(map[string]any{"started_at": "2026-10-06T10:00:00+24:00"}), false},
	{"an offset minute of 60", report(map[string]any{"started_at": "2026-10-06T10:00:00+05:60"}), false},
	{"a fraction with no digits", report(map[string]any{"started_at": "2026-10-06T10:00:00.Z"}), false},
	{"ten fraction digits", report(map[string]any{"started_at": "2026-10-06T10:00:00.1234567891Z"}), true},
	{"no offset", report(map[string]any{"started_at": "2026-10-06T10:00:00"}), false},
	{"a date alone", report(map[string]any{"started_at": "2026-10-06"}), false},
	{"hour 24", report(map[string]any{"started_at": "2026-10-06T24:00:00Z"}), false},
	{"lowercase t and z", report(map[string]any{"started_at": "2026-10-06t10:00:00z"}), true},
	{"a negative offset", report(map[string]any{"started_at": "2026-10-06T10:00:00-23:59"}), true},
	{"started_at with a space", report(map[string]any{"started_at": "2026-10-06 10:00:00Z"}), false},
	{"an impossible date", report(map[string]any{"started_at": "2026-02-30T00:00:00Z"}), false},
	{"an offset without a colon", report(map[string]any{"reported_at": "2026-10-06T10:00:00+0530"}), false},
	{"a timestamp of 65 characters", report(map[string]any{"reported_at": "2026-10-06T10:00:00." + strings.Repeat("1", 44) + "Z"}), false},
	{"reported_at as a number", report(map[string]any{"reported_at": 1759744800}), false},
	{"empty batch", `{"runs":[]}`, false},
	{"batch of 101", batchOf(101), false},
	{"batch with another key", `{"runs":[` + report(nil) + `],"source":"superpipeline"}`, false},
	{"batch with one bad item", `{"runs":[` + report(nil) + `,` + report(map[string]any{"status": "done"}) + `]}`, false},
	{"runs not an array", `{"runs":{}}`, false},
	{"top-level array", `[` + report(nil) + `]`, false},
	{"null", `null`, false},
	{"a string", `"run"`, false},
}

const schemaID = "https://docs.superwitness.dev/schemas/run-report.schema.json"

func compiledSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(contracts.RunReportSchema))
	if err != nil {
		t.Fatalf("schema is not JSON: %v", err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource(schemaID, doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile(schemaID)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return s
}

func schemaAccepts(s *jsonschema.Schema, body string) bool {
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(body))
	return err == nil && s.Validate(inst) == nil
}

// The published schema and the handler's validation accept and refuse the same bodies.
func TestSchemaAndGoAgree(t *testing.T) {
	s := compiledSchema(t)
	// Far in the future, so that no fixture trips the clock-skew rule, which the schema cannot hold.
	now := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, f := range fixtures {
		if got := schemaAccepts(s, f.body); got != f.valid {
			t.Errorf("%s: schema says valid=%v, want %v", f.name, got, f.valid)
		}
		if _, _, err := ParseBody([]byte(f.body), now); (err == nil) != f.valid {
			t.Errorf("%s: Go says valid=%v (%v), want %v", f.name, err == nil, err, f.valid)
		}
	}
}

// The rules superwitness keeps beyond the schema: the schema accepts these, Go refuses them.
func TestRulesOutsideTheSchema(t *testing.T) {
	s := compiledSchema(t)
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	for name, body := range map[string]string{
		"reported_at 6 minutes ahead": report(map[string]any{"reported_at": "2026-10-06T10:06:00Z"}),
		"a title holding NUL":         report(map[string]any{"title": "a\u0000b"}),
		"a leap second":               report(map[string]any{"ended_at": "2016-12-31T23:59:60Z"}),
	} {
		if !schemaAccepts(s, body) {
			t.Errorf("%s: the schema refuses it; it should be a superwitness-only rule", name)
		}
		if _, _, err := ParseBody([]byte(body), now); err == nil {
			t.Errorf("%s: Go accepted it", name)
		}
	}
}

// The schema is shared byte for byte with the sources that vendor it; a change is a contract change.
func TestSchemaIsTheSharedCopy(t *testing.T) {
	sum := sha256.Sum256(contracts.RunReportSchema)
	if got := hex.EncodeToString(sum[:]); got != "9b3bebd3c5fc2b827169af29f245e0d4d9ddeedba756b05354dbe24de7d82516" {
		t.Errorf("run-report.schema.json sha256 = %s; a changed schema must be agreed with every source that vendors it", got)
	}
}

func TestSchemaSourcePatternMatchesConfig(t *testing.T) {
	var s struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Pattern string `json:"pattern"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(contracts.RunReportSchema, &s); err != nil {
		t.Fatal(err)
	}
	if got := s.Defs["report"].Properties["source"].Pattern; got != config.SourceNamePattern {
		t.Errorf("schema source pattern %q, config %q", got, config.SourceNamePattern)
	}
}
