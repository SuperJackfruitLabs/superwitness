package contracts

import (
	"encoding/json"
	"testing"
)

func TestFixturesAreJSON(t *testing.T) {
	for name, b := range map[string][]byte{
		"superpipeline": SuperpipelineEvidence, "hub run": HubEvidenceRun, "hub attempt": HubEvidenceAttempt,
		"principal": HubPrincipal, "jaeger hub": JaegerTraceHub, "jaeger workers": JaegerTraceWorkers,
	} {
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
