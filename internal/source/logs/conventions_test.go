package logs

import (
	"regexp"
	"slices"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/treaty"
)

// The log query uses only attribute keys the suite's telemetry conventions name. The other
// fields it reads are the log store's own record fields, not attributes a producer sets.
func TestLogQueryUsesConventionKeys(t *testing.T) {
	conv, err := treaty.TelemetryConventions()
	if err != nil {
		t.Fatal(err)
	}
	ref, err := source.NewSuperpipelineRef("brd_01", "run_01")
	if err != nil {
		t.Fatal(err)
	}
	ref.TraceIDs = []string{"4bf92f3577b34da6a3ce929d0e0e4736"}
	q, err := BuildListQuery(ref, source.LogQuery{Level: "error", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	record := []string{"_time", "trace_id", "severity_number", "severity_text"}
	fields := regexp.MustCompile(`"([^"]+)":=|([A-Za-z_][A-Za-z0-9_.]*):(?:in\(|range\[|i\(|[0-9])`).FindAllStringSubmatch(q, -1)
	if len(fields) == 0 {
		t.Fatalf("no fields in %s", q)
	}
	attrs := 0
	for _, m := range fields {
		key := m[1] + m[2]
		if slices.Contains(record, key) {
			continue
		}
		attrs++
		if _, ok := conv.Attributes[key]; !ok {
			t.Errorf("the log query filters on %q, a key the conventions do not name", key)
		}
		if !slices.Contains(source.QueriedAttributes, key) {
			t.Errorf("the log query filters on %q, which source.QueriedAttributes does not list", key)
		}
	}
	if attrs == 0 {
		t.Fatalf("the query filters on no attribute: %s", q)
	}
	for name, l := range conv.Logs {
		for _, k := range l.Required {
			if k == source.AttrRunID {
				return
			}
		}
		t.Errorf("log record %s does not carry %s, the key the query finds a run's lines by", name, source.AttrRunID)
	}
}
