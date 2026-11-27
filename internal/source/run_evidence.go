package source

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"time"
)

// UnmarshalJSON reads SuperPipeline's run evidence (board-read's readRunEvidence) as the
// contract states it: a closed shape, so an unknown key anywhere is refused rather than
// dropped, every key present (nullable ones as null), a gate's status and decision from their
// enums, and usage either reported with its numbers or unreported with none. The same rules
// hold for the copy evidence-v1 nests as `pipeline`.
func (f *RunFragment) UnmarshalJSON(raw []byte) error {
	type plain RunFragment
	if err := uniqueJSON(raw); err != nil {
		return err
	}
	if err := strictWorkShape(raw, reflect.TypeOf(plain{})); err != nil {
		return err
	}
	var v plain
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	*f = RunFragment(v)
	return f.validate()
}

func (f *RunFragment) validate() error {
	if _, err := time.Parse(time.RFC3339Nano, f.AsOf); err != nil {
		return fmt.Errorf("invalid run evidence as_of")
	}
	for _, g := range f.Gates {
		if !oneOf(g.Status, "pending", "resolved", "cancelled") {
			return fmt.Errorf("invalid gate status")
		}
		if g.Decision != nil && !oneOf(*g.Decision, "approved", "changes_requested", "rejected") {
			return fmt.Errorf("invalid gate decision")
		}
	}
	u := f.Usage
	switch u.Status {
	case "reported":
		if u.InputTokens == nil || u.OutputTokens == nil || u.CostUSD == nil {
			return fmt.Errorf("reported usage without its numbers")
		}
	case "unreported":
		if u.InputTokens != nil || u.OutputTokens != nil || u.CostUSD != nil {
			return fmt.Errorf("unreported usage with numbers")
		}
	default:
		return fmt.Errorf("invalid usage status")
	}
	return nil
}
