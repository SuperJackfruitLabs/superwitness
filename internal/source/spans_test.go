package source

import (
	"slices"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/treaty"
)

// Every attribute key SuperWitness queries or reads is one the suite's telemetry conventions
// name, and the spans it joins carry it: run.id on dispatch, attempt.id on attempt and turn.
func TestQueriedAttributesAreConventions(t *testing.T) {
	conv, err := treaty.TelemetryConventions()
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range QueriedAttributes {
		if a, ok := conv.Attributes[k]; !ok || a.Type != "string" {
			t.Errorf("%s: not a string attribute of the conventions", k)
		}
	}
	for span, key := range map[string]string{"dispatch": AttrRunID, "attempt": AttrAttemptID, "turn": AttrAttemptID} {
		if !slices.Contains(conv.Spans[span].Required, key) {
			t.Errorf("span %s does not require %s", span, key)
		}
	}
}
