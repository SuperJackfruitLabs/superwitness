package treaty

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every copy treaty.json vendors under this directory is embedded, parses, and is the file the
// manifest names: a copy added to the manifest but not read here, or the reverse, fails.
func TestEmbedsWhatTheManifestVendors(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "treaty.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Vendored []struct{ Path, Source string }
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	const dir = "internal/treaty/"
	n := 0
	for _, v := range m.Vendored {
		if !strings.HasPrefix(v.Path, dir) {
			continue
		}
		n++
		if v.Path != dir+v.Source {
			t.Errorf("%s: a copy here sits at its treaty path, %s", v.Path, v.Source)
		}
		got, err := File(v.Source)
		if err != nil {
			t.Fatalf("%s is not embedded: %v", v.Source, err)
		}
		if !json.Valid(got) {
			t.Errorf("%s is not JSON", v.Source)
		}
	}
	for _, p := range []string{BoardRead, BoardReadCases, Evidence, EvidenceCases, EvidenceExamples, Telemetry, TelemetryCases} {
		if _, err := File(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if n < 7 {
		t.Fatalf("treaty.json vendors %d files here", n)
	}
}

func TestLoaders(t *testing.T) {
	c, err := Response(BoardReadCases, "readRunEvidence", "200")
	if err != nil || len(c.Valid) == 0 || len(c.Invalid) == 0 {
		t.Fatalf("%v %+v", err, c)
	}
	if _, err := Response(BoardReadCases, "noSuchOperation", "200"); err == nil {
		t.Fatal("an operation the file lacks has cases")
	}
	if _, err := c.Case("no such case"); err == nil {
		t.Fatal("a missing case was found")
	}
	if x, err := Examples("execution-export.v1"); err != nil || len(x.Valid) == 0 {
		t.Fatalf("examples: %v", err)
	}
	ops, err := Operations(Evidence)
	if err != nil || ops["readRunEvidence"].Path != "/api/evidence/runs/{source}/{externalRunId}" || ops["readRunEvidence"].Method != "GET" {
		t.Fatalf("operations: %v %+v", err, ops)
	}
	conv, err := TelemetryConventions()
	if err != nil || conv.Attributes["run.id"].Type != "string" || len(conv.Spans["dispatch"].Required) == 0 {
		t.Fatalf("conventions: %v", err)
	}
	if tc, err := TelemetryExamples(); err != nil || len(tc.Spans["dispatch"].Valid) == 0 {
		t.Fatalf("telemetry cases: %v", err)
	}
}
