package source

import (
	"encoding/json"
	"os"
	"testing"
)

func TestVersionedWorkEvidence(t *testing.T) {
	b, err := os.ReadFile("testdata/run-evidence-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var work WorkEvidence
	if err := json.Unmarshal(b, &work); err != nil {
		t.Fatal(err)
	}
	if work.SchemaVersion != "superpipeline-run-evidence/v1" || work.Usage.Status != "known" {
		t.Fatalf("%+v", work)
	}
	for _, mutate := range []func(map[string]any){
		func(m map[string]any) {
			m["provenance"].(map[string]any)["references"].([]any)[0].(map[string]any)["execution_export"].(map[string]any)["verification"] = "verified"
		},
		func(m map[string]any) { m["schema_version"] = "unsupported" },
		func(m map[string]any) { delete(m, "usage") },
		func(m map[string]any) { m["provenance"].(map[string]any)["handoff"] = map[string]any{"raw": "secret"} },
		func(m map[string]any) { m["executor_evidence"].(map[string]any)["external_run_id"] = "foreign" },
		func(m map[string]any) {
			m["provenance"].(map[string]any)["references"].([]any)[0].(map[string]any)["metadata"] = map[string]any{"raw": "secret"}
		},
		func(m map[string]any) {
			m["provenance"].(map[string]any)["references"].([]any)[0].(map[string]any)["runId"] = "foreign"
		},
	} {
		var m map[string]any
		json.Unmarshal(b, &m)
		mutate(m)
		raw, _ := json.Marshal(m)
		if json.Unmarshal(raw, &work) == nil {
			t.Fatal("accepted unknown fields or conflicting work joins")
		}
	}
}
