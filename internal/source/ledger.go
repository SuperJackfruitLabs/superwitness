package source

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
)

// The hub's evidence reads are open shapes (agentpod.evidence): a key SuperWitness does not
// know is ignored. Every key the contract requires must be there, though, null when it has no
// value, so a missing key never reads as a known null.

var fingerprintDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// UnmarshalJSON reads the hub's run evidence (readRunEvidence): every key present, attempt and
// session ids with their prefixes, and a fingerprint whose digest is a sha256 or "unknown"
// (never an empty string standing in for it) and whose reporter is one the hub names.
func (l *LedgerFragment) UnmarshalJSON(raw []byte) error {
	type plain LedgerFragment
	var v plain
	if err := decodeOpen(raw, &v); err != nil {
		return err
	}
	*l = LedgerFragment(v)
	return l.validate()
}

func (l *LedgerFragment) validate() error {
	if l.ExternalSource == "" || l.ExternalRunID == "" {
		return fmt.Errorf("run evidence names no run")
	}
	if _, err := time.Parse(time.RFC3339Nano, l.AsOf); err != nil {
		return fmt.Errorf("invalid run evidence as_of")
	}
	if d := l.Dispatch; d != nil && (d.Outcome == "" || d.StationID == "") {
		return fmt.Errorf("invalid dispatch")
	}
	for _, a := range l.Attempts {
		if !ValidAttemptID(a.ID) || !strings.HasPrefix(a.SessionID, "acps_") || len(a.SessionID) == len("acps_") || a.State == "" || a.StationID == "" || a.StartSeq < 0 || (a.EndSeq != nil && *a.EndSeq < 0) {
			return fmt.Errorf("invalid attempt")
		}
		fp := a.Fingerprint
		if fp.Digest != "unknown" && !fingerprintDigest.MatchString(fp.Digest) {
			return fmt.Errorf("invalid fingerprint digest")
		}
		if !oneOf(fp.ReportedBy, "harness", "station", "hub") || fp.Harness == "" || fp.HarnessVersion == "" || fp.Model == "" || fp.Profile == "" || fp.SkillRelease == "" {
			return fmt.Errorf("invalid fingerprint")
		}
	}
	return nil
}

// UnmarshalJSON reads the hub's answer for an attempt (readAttemptRun): all three keys present,
// each a string or null.
func (l *AttemptLink) UnmarshalJSON(raw []byte) error {
	type plain AttemptLink
	var v plain
	if err := decodeOpen(raw, &v); err != nil {
		return err
	}
	*l = AttemptLink(v)
	return nil
}

// decodeOpen decodes raw into out, a pointer to a struct, once every key out's fields name is
// present in raw (nested structs and slices too; a nullable field may be null). Keys out does
// not name are ignored.
func decodeOpen(raw []byte, out any) error {
	if err := uniqueJSON(raw); err != nil {
		return err
	}
	if err := requireFields(raw, reflect.TypeOf(out).Elem()); err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func requireFields(raw json.RawMessage, t reflect.Type) error {
	isNull := bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
	switch {
	case t.Kind() == reflect.Pointer:
		if isNull {
			return nil
		}
		return requireFields(raw, t.Elem())
	case t.PkgPath() == "time":
		return nil // the decode checks the timestamp
	case t.Kind() == reflect.Struct:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			return fmt.Errorf("evidence: want an object")
		}
		for i := 0; i < t.NumField(); i++ {
			name := t.Field(i).Tag.Get("json")
			v, ok := fields[name]
			if !ok {
				return fmt.Errorf("evidence: missing %s", name)
			}
			if err := requireFields(v, t.Field(i).Type); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	case t.Kind() == reflect.Slice:
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil || values == nil {
			return fmt.Errorf("evidence: want an array")
		}
		for _, v := range values {
			if err := requireFields(v, t.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}
