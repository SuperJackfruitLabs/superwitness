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

// Admits gives the conventions' cases their verdicts: every value of a valid span case is of its
// key's type, and every invalid case lacks a key its span requires or has a value Admits refuses.
func TestAdmitsAgreesWithTheCases(t *testing.T) {
	conv, err := TelemetryConventions()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := TelemetryExamples()
	if err != nil {
		t.Fatal(err)
	}
	refused := func(span string, body json.RawMessage) []string {
		var attrs map[string]any
		dec := json.NewDecoder(strings.NewReader(string(body)))
		dec.UseNumber()
		if err := dec.Decode(&attrs); err != nil {
			t.Fatal(err)
		}
		var why []string
		for _, k := range conv.Spans[span].Required {
			if _, ok := attrs[k]; !ok {
				why = append(why, "no "+k)
			}
		}
		for k, v := range attrs {
			if a, ok := conv.Attributes[k]; ok && !a.Admits(v) {
				why = append(why, k)
			}
		}
		return why
	}
	for span, c := range cases.Spans {
		for name, body := range c.Valid {
			if why := refused(span, body); len(why) > 0 {
				t.Errorf("%s %q is valid, but refused for %v", span, name, why)
			}
		}
		for _, inv := range c.Invalid {
			if len(refused(span, inv.Body)) == 0 {
				t.Errorf("%s %q is invalid, but admitted", span, inv.Why)
			}
		}
	}
}

func TestCoerce(t *testing.T) {
	for _, c := range []struct {
		typ  string
		in   any
		want any
	}{
		{"int", "3", int64(3)}, {"int", json.Number("3"), int64(3)}, {"int", "-1", int64(-1)},
		{"int", "1.5", "1.5"}, {"int", "three", "three"}, {"int", "", ""}, {"int", true, true},
		{"double", "0.25", 0.25}, {"double", "NaN", "NaN"}, {"double", "1e999", "1e999"},
		{"boolean", "true", true}, {"boolean", "false", false}, {"boolean", "1", "1"}, {"boolean", "TRUE", "TRUE"},
		{"string", json.Number("42"), "42"}, {"string", true, "true"}, {"string", "x", "x"},
		{"string[]", "x", "x"},
	} {
		if got := CoerceTo(c.typ, c.in); got != c.want {
			t.Errorf("CoerceTo(%s, %#v) = %#v, want %#v", c.typ, c.in, got, c.want)
		}
	}
}
