package source

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestMetadataExportReceiptStrictness(t *testing.T) {
	r := ExecutionExportReceipt{ArtifactFiles: []ExecutionExportFile{}, SchemaVersion: ExecutionExportSchemaV1, ExportID: "00000000-0000-4000-8000-000000000001", ExternalSource: "superpipeline", ExternalRunID: "run_01", BoardID: "brd_01", CardID: "card_01", StageKey: "draft", PrincipalID: "prn_agent01", RunEpoch: 1, ManifestSHA256: strings.Repeat("a", 64), ManifestBytes: 100, ArtifactKind: "execution-metadata-manifest", Verification: "download-sha256-verified", Library: ExecutionExportLibrary{ItemID: "item_01", Version: 1, SHA256: strings.Repeat("a", 64), Bytes: 100, Entry: "execution-evidence.json"}}
	b, _ := json.Marshal(r)
	var receipt ExecutionExportReceipt
	if err := json.Unmarshal(b, &receipt); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(map[string]any){
		func(m map[string]any) { m["verification"] = "claimed" },
		func(m map[string]any) { m["library"].(map[string]any)["sha256"] = strings.Repeat("b", 64) },
		func(m map[string]any) { m["library"].(map[string]any)["bytes"] = 101 },
		func(m map[string]any) { m["source_bytes"] = "raw" },
		func(m map[string]any) { delete(m, "principal_id") },
	} {
		var m map[string]any
		json.Unmarshal(b, &m)
		mutate(m)
		bad, _ := json.Marshal(m)
		if json.Unmarshal(bad, &receipt) == nil {
			t.Fatal("accepted malformed metadata receipt")
		}
	}
}

func TestSharedExportReceiptCorpus(t *testing.T) {
	b, err := os.ReadFile("testdata/execution-export-v1.json")
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
	if len(corpus.Success) != 2 || len(corpus.Rejection) != 5 {
		t.Fatal("shared export corpus changed")
	}
	for _, c := range corpus.Success {
		t.Run(c.Name, func(t *testing.T) {
			var r ExecutionExportReceipt
			if err := json.Unmarshal(c.Value, &r); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, c := range corpus.Rejection {
		t.Run(c.Name, func(t *testing.T) {
			var r ExecutionExportReceipt
			if json.Unmarshal(c.Value, &r) == nil {
				t.Fatal("accepted rejected receipt")
			}
		})
	}
}

func TestArtifactBundleReceiptRejectsFalseFileProof(t *testing.T) {
	b, err := os.ReadFile("testdata/execution-export-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Success []struct{ Value json.RawMessage }
	}
	json.Unmarshal(b, &corpus)
	valid := corpus.Success[1].Value
	for _, mutate := range []func(map[string]any){
		func(m map[string]any) { delete(m, "artifact_files") },
		func(m map[string]any) {
			m["artifact_files"].([]any)[0].(map[string]any)["file_id"].(map[string]any)["item_id"] = "foreign"
		},
		func(m map[string]any) {
			m["artifact_files"].([]any)[0].(map[string]any)["file_id"].(map[string]any)["version"] = 2
		},
		func(m map[string]any) {
			m["artifact_files"].([]any)[0].(map[string]any)["file_id"].(map[string]any)["path"] = "foreign"
		},
		func(m map[string]any) { m["artifact_files"].([]any)[0].(map[string]any)["path"] = "../private.json" },
		func(m map[string]any) { m["artifact_files"].([]any)[0].(map[string]any)["disposition"] = "observed" },
		func(m map[string]any) {
			m["artifact_files"].([]any)[0].(map[string]any)["sha256"] = strings.Repeat("b", 64)
		},
		func(m map[string]any) { m["library"].(map[string]any)["sha256"] = m["manifest_sha256"] },
		func(m map[string]any) { m["library"].(map[string]any)["bytes"] = m["manifest_bytes"] },
	} {
		var m map[string]any
		json.Unmarshal(valid, &m)
		mutate(m)
		raw, _ := json.Marshal(m)
		var receipt ExecutionExportReceipt
		if json.Unmarshal(raw, &receipt) == nil {
			t.Fatal("accepted false immutable file proof")
		}
	}
}
