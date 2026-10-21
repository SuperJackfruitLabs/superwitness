package source

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func executionFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/execution-evidence-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Success   []struct{ Value json.RawMessage }
		Rejection []struct{ Value json.RawMessage }
	}
	if err := json.Unmarshal(b, &corpus); err != nil {
		t.Fatal(err)
	}
	return corpus.Success[0].Value
}
func wrapExecution(value []byte) []byte {
	return []byte(`{"schema_version":"agentpod-execution-evidence/v1","external_source":"superpipeline","external_run_id":"run_abcdef","board_id":"brd_abcdef","card_id":"card_abcdef","executions":[` + string(value) + `],"as_of":"2026-10-21T00:00:00Z"}`)
}
func TestExecutionSharedCorpus(t *testing.T) {
	b, err := os.ReadFile("testdata/execution-evidence-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Success []struct {
			Name  string
			Value json.RawMessage
		}
		Rejection []struct {
			Name  string
			Value json.RawMessage
		}
	}
	if err := json.Unmarshal(b, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Success) != 3 || len(corpus.Rejection) != 7 {
		t.Fatal("canonical success/rejection corpus changed")
	}
	for _, c := range corpus.Success {
		t.Run(c.Name, func(t *testing.T) {
			var e ExecutionEnvelope
			if err := json.Unmarshal(wrapExecution(c.Value), &e); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, c := range corpus.Rejection {
		t.Run(c.Name, func(t *testing.T) {
			var e ExecutionEnvelope
			if err := json.Unmarshal(wrapExecution(c.Value), &e); err == nil {
				t.Fatal("accepted rejected fixture")
			}
		})
	}
}
func TestExecutionRejectsJoinAndMissingFields(t *testing.T) {
	for _, mutate := range []func(map[string]any){
		func(x map[string]any) { delete(x, "usage") },
		func(x map[string]any) { x["unknown"] = true },
		func(x map[string]any) {
			x["usage"].(map[string]any)["session_id"] = "22222222-2222-4222-8222-222222222222"
		},
		func(x map[string]any) { x["binding"].(map[string]any)["run"].(map[string]any)["id"] = "run_other" },
	} {
		var x map[string]any
		json.Unmarshal(executionFixture(t), &x)
		mutate(x)
		b, _ := json.Marshal(x)
		var e ExecutionEnvelope
		if json.Unmarshal(wrapExecution(b), &e) == nil {
			t.Fatal("accepted conflicting evidence")
		}
	}
	var e ExecutionEnvelope
	if json.Unmarshal(append(wrapExecution(executionFixture(t)), []byte(` {}`)...), &e) == nil {
		t.Fatal("accepted trailing JSON")
	}
}

func TestExecutionContributorClosureAndScopedReferences(t *testing.T) {
	var e ExecutionEnvelope
	if err := json.Unmarshal(wrapExecution(executionFixture(t)), &e); err != nil {
		t.Fatal(err)
	}
	e.Executions[0].Contributors.Closure = "complete"
	ids, complete := e.ContributorClosure()
	if !complete || len(ids) != 1 {
		t.Fatalf("closure %v %v", ids, complete)
	}
	child := "prn_" + strings.Repeat("b", 20)
	e.Executions[0].Children = []ExecutionChild{{ContributorPrincipalID: &child}}
	ids, complete = e.ContributorClosure()
	if !complete || ids[len(ids)-1] != child {
		t.Fatal("child contributor omitted")
	}
	e.Executions[0].Children[0].ContributorPrincipalID = nil
	if _, complete = e.ContributorClosure(); complete {
		t.Fatal("unresolved child accepted")
	}
	receipt := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	e.Executions[0].Tools = []ExecutionTool{{ID: "receipt_1", Source: "managed-http", ReceiptDigest: &receipt}}
	e.Executions[0].Artifacts = []ExecutionArtifact{{ID: "artifact_1"}}
	if !e.ContainsTypedReference("tool_receipt", "receipt_1", "managed-http") || !e.ContainsReference("artifact", "artifact_1") || e.ContainsReference("artifact", "other") {
		t.Fatal("reference scope")
	}
	e.Executions[0].Tools[0].ReceiptDigest = nil
	if e.ContainsTypedReference("tool_receipt", "receipt_1", "managed-http") {
		t.Fatal("model/tool state is not a receipt")
	}
}

func TestReadinessFingerprintRemainsSourcePinnedEvidence(t *testing.T) {
	var evidence map[string]any
	json.Unmarshal(executionFixture(t), &evidence)
	hash := strings.Repeat("a", 64)
	evidence["fingerprints"] = []any{map[string]any{"session_id": evidence["task"].(map[string]any)["session_id"], "worker_epoch": 1, "version": 1, "harness": "salaam", "harness_version": "dev", "pi_version": "dev", "bundle_digest": hash, "policy_digest": hash, "provider": "provider", "model": "model", "context_digest": hash, "context_admission_digest": nil, "verification": "pinned-readiness-handshake", "digest": hash}}
	fpMap := evidence["fingerprints"].([]any)[0].(map[string]any)
	fpRaw, _ := json.Marshal(fpMap)
	var fp ExecutionFingerprint
	json.Unmarshal(fpRaw, &fp)
	fpMap["digest"] = readinessDigest(fp)
	b, _ := json.Marshal(evidence)
	var decoded ExecutionEnvelope
	if err := json.Unmarshal(wrapExecution(b), &decoded); err != nil {
		t.Fatal(err)
	}
	evidence["fingerprints"].([]any)[0].(map[string]any)["verification"] = "model-claimed"
	b, _ = json.Marshal(evidence)
	if json.Unmarshal(wrapExecution(b), &decoded) == nil {
		t.Fatal("model claim became readiness evidence")
	}
}

func TestReadinessDigestBindsSessionAndWorkerEpoch(t *testing.T) {
	hash := strings.Repeat("a", 64)
	fp := ExecutionFingerprint{SessionID: "11111111-1111-4111-8111-111111111111", WorkerEpoch: 1, Version: 1, Harness: "salaam", HarnessVersion: "dev", PiVersion: "dev", BundleDigest: hash, PolicyDigest: hash, Provider: "provider", Model: "model", ContextDigest: hash, Verification: "pinned-readiness-handshake"}
	// Independently generated with protocol.canonical's sorted JSON keys.
	if got := readinessDigest(fp); got != "796c2c325b9e86ab7b65d4192bc72173aaf6a0b83fd86f80c09062ed1b30bad7" {
		t.Fatalf("canonical fingerprint digest %s", got)
	}
	fp.Digest = readinessDigest(fp)
	var raw map[string]any
	json.Unmarshal(executionFixture(t), &raw)
	raw["fingerprints"] = []ExecutionFingerprint{fp}
	b, _ := json.Marshal(raw)
	var e ExecutionEnvelope
	if err := json.Unmarshal(wrapExecution(b), &e); err != nil {
		t.Fatal(err)
	}
	fp.WorkerEpoch++
	raw["fingerprints"] = []ExecutionFingerprint{fp}
	b, _ = json.Marshal(raw)
	if json.Unmarshal(wrapExecution(b), &e) == nil {
		t.Fatal("changed epoch retained trusted fingerprint")
	}
	fp.WorkerEpoch--
	fp.SessionID = "22222222-2222-4222-8222-222222222222"
	raw["fingerprints"] = []ExecutionFingerprint{fp}
	b, _ = json.Marshal(raw)
	if json.Unmarshal(wrapExecution(b), &e) == nil {
		t.Fatal("foreign session retained trusted fingerprint")
	}
}

func TestUnmappedContributorProofFailsClosed(t *testing.T) {
	for _, invalid := range []string{"agt_unmapped", "prn_not_a_canonical_principal"} {
		var e ExecutionEnvelope
		if err := json.Unmarshal(wrapExecution(executionFixture(t)), &e); err != nil {
			t.Fatal(err)
		}
		e.Executions[0].Contributors.Closure = "complete"
		e.Executions[0].Contributors.PrincipalIDs = []string{invalid}
		if _, complete := e.ContributorClosure(); complete {
			t.Fatal("unmapped declared contributor established closure")
		}
		e.Executions[0].Contributors.PrincipalIDs = nil
		e.Executions[0].Children = []ExecutionChild{{ContributorPrincipalID: &invalid}}
		if _, complete := e.ContributorClosure(); complete {
			t.Fatal("unmapped child contributor established closure")
		}
	}
}

func TestOperatorAttestationDoesNotReplaceNativeReceipt(t *testing.T) {
	b, err := os.ReadFile("testdata/execution-evidence-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Success []struct{ Value json.RawMessage }
	}
	json.Unmarshal(b, &corpus)
	var e ExecutionEnvelope
	if err := json.Unmarshal(wrapExecution(corpus.Success[2].Value), &e); err != nil {
		t.Fatal(err)
	}
	found := false
	for i, tool := range e.Executions[0].Tools {
		if tool.ResolutionEvidence == nil {
			continue
		}
		found = true
		if tool.State != "outcome_unknown" || tool.ResolutionEvidence.ProductEffectVerified || tool.ResolutionEvidence.Authority {
			t.Fatal("operator attestation promoted native effect")
		}
		e.Executions[0].Tools[i].ReceiptDigest = nil
		if e.ContainsTypedReference("tool_receipt", tool.ID, "managed-toolkit") {
			t.Fatal("operator evidence substituted for native tool receipt")
		}
	}
	if !found {
		t.Fatal("missing canonical attestation proof")
	}
}
