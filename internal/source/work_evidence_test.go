package source

import (
	"encoding/json"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/treaty"
)

func TestVersionedWorkEvidence(t *testing.T) {
	b := validCase(t, response(t, treaty.BoardReadCases, "readRunEvidenceV1"), "superpipeline's run-evidence-v1 fixture")
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
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		mutate(m)
		raw, _ := json.Marshal(m)
		if json.Unmarshal(raw, &work) == nil {
			t.Fatal("accepted unknown fields or conflicting work joins")
		}
	}
}

// Every evidence-v1 case of treaty's board-read cases reads and every invalid one is refused: an
// unknown schema_version among them is refused, never read as v1.
func TestWorkEvidenceTreatyCases(t *testing.T) {
	cases := response(t, treaty.BoardReadCases, "readRunEvidenceV1")
	for name, body := range cases.Valid {
		t.Run(name, func(t *testing.T) {
			var w WorkEvidence
			if err := json.Unmarshal(body, &w); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, c := range cases.Invalid {
		t.Run(c.Why, func(t *testing.T) {
			var w WorkEvidence
			if json.Unmarshal(c.Body, &w) == nil {
				t.Fatal("accepted")
			}
		})
	}
}

// The legacy run evidence (readRunEvidence) is closed too: every case reads, every invalid one,
// an unknown key among them, is refused.
func TestRunEvidenceTreatyCases(t *testing.T) {
	cases := response(t, treaty.BoardReadCases, "readRunEvidence")
	for name, body := range cases.Valid {
		t.Run(name, func(t *testing.T) {
			var f RunFragment
			if err := json.Unmarshal(body, &f); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, c := range cases.Invalid {
		t.Run(c.Why, func(t *testing.T) {
			var f RunFragment
			if json.Unmarshal(c.Body, &f) == nil {
				t.Fatal("accepted")
			}
		})
	}
}

// The hub's run evidence and attempt reads (agentpod.evidence) take every case and refuse every
// invalid one.
func TestLedgerTreatyCases(t *testing.T) {
	for op, decode := range map[string]func([]byte) error{
		"readRunEvidence": func(b []byte) error { var l LedgerFragment; return json.Unmarshal(b, &l) },
		"readAttemptRun":  func(b []byte) error { var l AttemptLink; return json.Unmarshal(b, &l) },
	} {
		cases := response(t, treaty.EvidenceCases, op)
		for name, body := range cases.Valid {
			t.Run(op+"/"+name, func(t *testing.T) {
				if err := decode(body); err != nil {
					t.Fatal(err)
				}
			})
		}
		for _, c := range cases.Invalid {
			t.Run(op+"/"+c.Why, func(t *testing.T) {
				if decode(c.Body) == nil {
					t.Fatal("accepted")
				}
			})
		}
	}
}

// The one valid case SuperWitness refuses is refused for its rule and nothing else: without its
// second execution of the shared session, the same envelope reads.
func TestStricterThanTreatyIsOnlyTheDuplicateSession(t *testing.T) {
	body := validCase(t, response(t, treaty.EvidenceCases, "readExecutionEvidence"), "every execution of AgentPod's corpus")
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	executions := m["executions"].([]any)
	if len(executions) != 3 {
		t.Fatalf("%d executions", len(executions))
	}
	m["executions"] = executions[:2]
	b, _ := json.Marshal(m)
	var e ExecutionEnvelope
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatalf("without the duplicate session: %v", err)
	}
}

// stricterThanTreaty names the valid cases SuperWitness refuses on purpose, each with its own
// rule. "every execution of AgentPod's corpus" bundles the corpus's three executions into one
// envelope, and two of them are the same task in the same session; SuperWitness reads one
// execution per session, so a second one for a session is a conflicting join, refused rather
// than merged (ExecutionEnvelope.Validate).
var stricterThanTreaty = map[string]string{
	"every execution of AgentPod's corpus": "two executions of one session",
}

// The hub's execution envelope (readExecutionEvidence) and export receipt (readExecutionExport)
// as the route answers them: every case reads, every invalid one is refused, among them an
// execution or receipt whose schema_version SuperWitness does not know.
func TestExecutionRouteTreatyCases(t *testing.T) {
	for op, decode := range map[string]func([]byte) error{
		"readExecutionEvidence": func(b []byte) error { var e ExecutionEnvelope; return json.Unmarshal(b, &e) },
		"readExecutionExport":   func(b []byte) error { var r ExecutionExportReceipt; return json.Unmarshal(b, &r) },
	} {
		cases := response(t, treaty.EvidenceCases, op)
		for name, body := range cases.Valid {
			t.Run(op+"/"+name, func(t *testing.T) {
				if why, stricter := stricterThanTreaty[name]; stricter {
					if decode(body) == nil {
						t.Fatalf("read, though SuperWitness refuses %s", why)
					}
					return
				}
				if err := decode(body); err != nil {
					t.Fatal(err)
				}
			})
		}
		for _, c := range cases.Invalid {
			t.Run(op+"/"+c.Why, func(t *testing.T) {
				if decode(c.Body) == nil {
					t.Fatal("accepted")
				}
			})
		}
	}
}
