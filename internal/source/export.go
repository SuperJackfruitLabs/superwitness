package source

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
)

const ExecutionExportSchemaV1 = "agentpod-execution-export/v1"

type ExecutionExportFileID struct {
	ItemID  string `json:"item_id"`
	Version int64  `json:"version"`
	Path    string `json:"path"`
}
type ExecutionExportFile struct {
	ArtifactID  string                `json:"artifact_id"`
	FileID      ExecutionExportFileID `json:"file_id"`
	Path        string                `json:"path"`
	SHA256      string                `json:"sha256"`
	Bytes       int64                 `json:"bytes"`
	Disposition string                `json:"disposition"`
}
type ExecutionExportReceipt struct {
	ArtifactFiles  []ExecutionExportFile  `json:"artifact_files"`
	SchemaVersion  string                 `json:"schema_version"`
	ExportID       string                 `json:"export_id"`
	ExternalSource string                 `json:"external_source"`
	ExternalRunID  string                 `json:"external_run_id"`
	BoardID        string                 `json:"board_id"`
	CardID         string                 `json:"card_id"`
	StageKey       string                 `json:"stage_key"`
	PrincipalID    string                 `json:"principal_id"`
	RunEpoch       int64                  `json:"run_epoch"`
	ManifestSHA256 string                 `json:"manifest_sha256"`
	ManifestBytes  int64                  `json:"manifest_bytes"`
	ArtifactKind   string                 `json:"artifact_kind"`
	Library        ExecutionExportLibrary `json:"library"`
	Verification   string                 `json:"verification"`
}
type ExecutionExportLibrary struct {
	ItemID  string `json:"item_id"`
	Version int64  `json:"version"`
	SHA256  string `json:"sha256"`
	Bytes   int64  `json:"bytes"`
	Entry   string `json:"entry"`
}
type ExportResolver interface {
	FetchExport(context.Context, string) (ExecutionExportReceipt, SourceStatus)
}

func (r *ExecutionExportReceipt) UnmarshalJSON(raw []byte) error {
	type plain ExecutionExportReceipt
	if err := uniqueJSON(raw); err != nil {
		return err
	}
	if err := strictShape(raw, reflect.TypeOf(plain{})); err != nil {
		return err
	}
	var value plain
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&value); err != nil {
		return err
	}
	*r = ExecutionExportReceipt(value)

	hash := regexp.MustCompile(`^[a-f0-9]{64}$`)
	if r.SchemaVersion != ExecutionExportSchemaV1 || !evidenceUUID.MatchString(r.ExportID) || r.ExternalSource != "superpipeline" || r.Verification != "download-sha256-verified" || r.Library.Version < 1 || !hash.MatchString(r.ManifestSHA256) || !hash.MatchString(r.Library.SHA256) {
		return fmt.Errorf("invalid immutable export receipt")
	}
	if r.ArtifactKind == "execution-metadata-manifest" {
		if len(r.ArtifactFiles) != 0 || r.Library.Entry != "execution-evidence.json" || r.Library.SHA256 != r.ManifestSHA256 || r.Library.Bytes != r.ManifestBytes {
			return fmt.Errorf("conflicting metadata export receipt")
		}
		return nil
	}
	if r.ArtifactKind != "execution-artifact-bundle" || r.Library.Entry != "execution-manifest.json" || len(r.ArtifactFiles) < 1 || len(r.ArtifactFiles) > 16 {
		return fmt.Errorf("invalid artifact bundle receipt")
	}
	pairs := [][2]string{{r.Library.Entry, r.ManifestSHA256}}
	total := r.ManifestBytes
	seen := map[string]bool{}
	for _, f := range r.ArtifactFiles {
		expected := "artifacts/" + f.ArtifactID + ".json"
		if !evidenceUUID.MatchString(f.ArtifactID) || f.Path != expected || f.FileID.Path != f.Path || f.FileID.ItemID != r.Library.ItemID || f.FileID.Version != r.Library.Version || !hash.MatchString(f.SHA256) || !oneOf(f.Disposition, "proposed", "produced", "integrated") || seen[f.Path] {
			return fmt.Errorf("conflicting immutable artifact file identity")
		}
		seen[f.Path] = true
		total += f.Bytes
		pairs = append(pairs, [2]string{f.Path, f.SHA256})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i][0] < pairs[j][0] })
	// Match the canonical JSON.stringify array of sorted [path, sha256] pairs.
	var canonical bytes.Buffer
	enc := json.NewEncoder(&canonical)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(pairs); err != nil {
		return err
	}
	digest := sha256.Sum256(bytes.TrimSuffix(canonical.Bytes(), []byte("\n")))
	if r.Library.Bytes != total || r.Library.SHA256 != hex.EncodeToString(digest[:]) {
		return fmt.Errorf("conflicting bundle hash or byte total")
	}
	return nil
}
