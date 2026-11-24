package contracts

import (
	"encoding/json"
	"testing"
)

func TestFixturesAreJSON(t *testing.T) {
	for name, b := range map[string][]byte{
		"jaeger hub": JaegerTraceHub, "jaeger workers": JaegerTraceWorkers,
		"run report schema": RunReportSchema,
	} {
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
