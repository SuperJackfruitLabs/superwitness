package source

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"time"
)

// WorkEvidence is the separately versioned Pipeline projection. Saved metadata
// remains in its owner; this projection carries bounded, typed provenance only.
type WorkEvidence struct {
	SchemaVersion    string               `json:"schema_version"`
	Pipeline         RunFragment          `json:"pipeline"`
	Provenance       WorkProvenance       `json:"provenance"`
	Usage            WorkUsage            `json:"usage"`
	ExecutorEvidence WorkExecutorEvidence `json:"executor_evidence"`
}
type WorkProvenance struct {
	ReferencesTruncated bool            `json:"references_truncated"`
	LeaseEpoch          int64           `json:"lease_epoch"`
	QueuedBy            *string         `json:"queued_by"`
	Completion          *WorkCompletion `json:"completion"`
	HandoffRecorded     bool            `json:"handoff_recorded"`
	References          []WorkReference `json:"references"`
}
type WorkCompletion struct {
	Met            bool  `json:"met"`
	UncheckedCount int64 `json:"unchecked_count"`
}
type WorkExecutionExport struct {
	SchemaVersion  string `json:"schema_version"`
	ExportID       string `json:"export_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Verification   string `json:"verification"`
}
type WorkReference struct {
	ExecutionExport *WorkExecutionExport `json:"execution_export"`
	ID              string               `json:"id"`
	CardID          string               `json:"cardId"`
	RunID           string               `json:"runId"`
	URL             string               `json:"url"`
	Provider        string               `json:"provider"`
	SourceType      string               `json:"sourceType"`
	ExternalID      *string              `json:"externalId"`
	CreatedAt       string               `json:"createdAt"`
}
type WorkUsage struct {
	Status           string   `json:"status"`
	RecordedCostUSD  *float64 `json:"recorded_cost_usd"`
	EstimatedCostUSD *float64 `json:"estimated_cost_usd"`
	CostProvenance   string   `json:"cost_provenance"`
}
type WorkExecutorEvidence struct {
	ExternalSource string `json:"external_source"`
	ExternalRunID  string `json:"external_run_id"`
	AgentPodPath   string `json:"agentpod_path"`
}

func (w *WorkEvidence) UnmarshalJSON(raw []byte) error {
	type plain WorkEvidence
	if err := uniqueJSON(raw); err != nil {
		return err
	}
	if err := strictWorkShape(raw, reflect.TypeOf(plain{})); err != nil {
		return err
	}
	var value plain
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&value); err != nil {
		return err
	}
	*w = WorkEvidence(value)
	if w.SchemaVersion != "superpipeline-run-evidence/v1" || w.Provenance.LeaseEpoch < 0 || w.ExecutorEvidence.ExternalSource != "superpipeline" || w.ExecutorEvidence.ExternalRunID != w.Pipeline.Run.ID || w.Pipeline.Card.ID != w.Pipeline.Run.CardID || !oneOf(w.Usage.Status, "known", "estimated", "unknown", "unreported", "partial") || !oneOf(w.Usage.CostProvenance, "agent-reported-not-provider-bill", "unknown") {
		return fmt.Errorf("invalid work evidence")
	}
	seen := map[string]bool{}
	for _, r := range w.Provenance.References {
		if r.RunID != w.Pipeline.Run.ID || r.CardID != w.Pipeline.Run.CardID || seen[r.ID] {
			return fmt.Errorf("conflicting work reference")
		}
		if _, err := url.ParseRequestURI(r.URL); err != nil || len(r.URL) > 8192 {
			return fmt.Errorf("invalid reference URL")
		}
		if _, err := time.Parse(time.RFC3339Nano, r.CreatedAt); err != nil {
			return err
		}
		if r.ExecutionExport != nil {
			hint := r.ExecutionExport
			if hint.SchemaVersion != "agentpod-execution-export/v1" || !evidenceUUID.MatchString(hint.ExportID) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(hint.ManifestSHA256) || hint.Verification != "unverified" {
				return fmt.Errorf("invalid unverified export locator")
			}
		}
		seen[r.ID] = true
	}
	return nil
}

// Work evidence nests the unchanged legacy shape, including its nullable strings
// and timestamps. Unknown and omitted keys still fail the versioned transport.
func strictWorkShape(raw json.RawMessage, t reflect.Type) error {
	if t.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		return strictWorkShape(raw, t.Elem())
	}
	if t.PkgPath() == "time" {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return fmt.Errorf("invalid timestamp")
		}
		return nil
	}
	if t.Kind() == reflect.Struct {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		if len(fields) != t.NumField() {
			return fmt.Errorf("missing or unknown work evidence fields")
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			v, ok := fields[f.Tag.Get("json")]
			if !ok {
				return fmt.Errorf("missing work evidence field")
			}
			if t.Name() == "SPRun" && f.Name == "AgentPrincipalID" || t.Name() == "SPGate" && oneOf(f.Name, "DecidedByPrincipalID", "DecidedByHubSub", "ProducedByPrincipalID") {
				if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
					continue
				}
			}
			if t.Name() == "WorkReference" && f.Name == "URL" {
				var value string
				if json.Unmarshal(v, &value) != nil || value == "" || len(value) > 8192 {
					return fmt.Errorf("invalid reference URL")
				}
				continue
			}
			if oneOf(t.Name(), "SPRun", "SPGate", "SPCard") && f.Type.Kind() == reflect.String {
				var value string
				if json.Unmarshal(v, &value) != nil {
					return fmt.Errorf("invalid legacy string")
				}
				continue
			}
			if err := strictWorkShape(v, f.Type); err != nil {
				return err
			}
		}
		return nil
	}
	if t.Kind() == reflect.Slice {
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil || values == nil || len(values) > 256 {
			return fmt.Errorf("invalid work evidence collection")
		}
		for _, v := range values {
			if err := strictWorkShape(v, t.Elem()); err != nil {
				return err
			}
		}
		return nil
	}
	return strictShape(raw, t)
}
