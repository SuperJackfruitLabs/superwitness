package verdicts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

// ValidateEvidenceRefs accepts span ids and {session_id, seq_from, seq_to} ranges only:
// pointers to evidence, never the evidence itself.
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
