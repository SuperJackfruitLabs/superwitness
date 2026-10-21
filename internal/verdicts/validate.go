package verdicts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"regexp"
	"strings"
)

var (
	rubricStd   = regexp.MustCompile(`^rubric:([A-Za-z0-9_.-]{1,64})@([1-9][0-9]{0,8})$`)
	stageStd    = regexp.MustCompile(`^stage:[A-Za-z0-9_.-]{1,64}$`)
	caseStd     = regexp.MustCompile(`^case:[A-Za-z0-9_.-]{1,128}$`)
	caseRef     = regexp.MustCompile(`^case:[A-Za-z0-9_.-]{1,128}@sha256:[0-9a-f]{64}$`)
	spanID      = regexp.MustCompile(`^[0-9a-f]{16}$`)
	errValueMsg = errors.New("value must be a JSON object with exactly one of decision, score, label, text")
)

// ValidateValue accepts {decision}, {score 0..1}, {label} or {text}.
func ValidateValue(raw json.RawMessage) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil || len(m) != 1 {
		return errValueMsg
	}
	for k, v := range m {
		switch k {
		case "score":
			var f float64
			if json.Unmarshal(v, &f) != nil || f < 0 || f > 1 {
				return errors.New("score must be a number from 0 to 1")
			}
		case "decision", "label", "text":
			var s string
			if json.Unmarshal(v, &s) != nil || strings.TrimSpace(s) == "" {
				return fmt.Errorf("%s must be a non-empty string", k)
			}
		default:
			return errValueMsg
		}
	}
	return nil
}

// ValidateEvidenceRefs accepts pointers to spans, transcript ranges, and typed
// receipt/artifact metadata. Typed references also require a joined subject snapshot.
func ValidateEvidenceRefs(raw json.RawMessage) error {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return errors.New("evidence_refs must be an array")
	}
	if len(items) > 100 {
		return errors.New("evidence_refs holds at most 100 references")
	}
	for i, it := range items {
		var s string
		if json.Unmarshal(it, &s) == nil {
			if !spanID.MatchString(s) {
				return fmt.Errorf("evidence_refs[%d]: a span id is 16 lowercase hex characters", i)
			}
			continue
		}
		var typed struct {
			Kind   string `json:"kind"`
			ID     string `json:"id"`
			Source string `json:"source,omitempty"`
		}
		td := json.NewDecoder(bytes.NewReader(it))
		td.DisallowUnknownFields()
		if td.Decode(&typed) == nil && ((typed.Kind == "tool_receipt" && typed.Source != "") || ((typed.Kind == "artifact" || typed.Kind == "execution_export") && typed.Source == "")) && typed.ID != "" {
			continue
		}
		var r struct {
			SessionID string `json:"session_id"`
			SeqFrom   *int64 `json:"seq_from"`
			SeqTo     *int64 `json:"seq_to"`
		}
		dec := json.NewDecoder(bytes.NewReader(it))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&r); err != nil || r.SessionID == "" || r.SeqFrom == nil || r.SeqTo == nil ||
			*r.SeqFrom < 0 || *r.SeqTo < *r.SeqFrom {
			return fmt.Errorf("evidence_refs[%d]: want a span id or {session_id, seq_from, seq_to}", i)
		}
	}
	return nil
}

func validateScopedEvidence(raw json.RawMessage, evidence *source.ExecutionEnvelope, exports ...source.ExecutionExportReceipt) error {
	var refs []json.RawMessage
	if err := json.Unmarshal(raw, &refs); err != nil {
		return err
	}
	for _, r := range refs {
		var ref struct {
			Kind   string `json:"kind"`
			ID     string `json:"id"`
			Source string `json:"source,omitempty"`
		}
		if json.Unmarshal(r, &ref) != nil || ref.Kind == "" {
			continue
		}
		if ref.Kind == "execution_export" {
			found := false
			for _, r := range exports {
				if r.ExportID == ref.ID {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("export receipt is not in the joined subject evidence")
			}
			continue
		}
		if evidence == nil || !evidence.ContainsTypedReference(ref.Kind, ref.ID, ref.Source) {
			return fmt.Errorf("receipt or artifact is not in the joined subject evidence")
		}
	}
	return nil
}
