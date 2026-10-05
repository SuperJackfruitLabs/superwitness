package source

import (
	"errors"
	"testing"
)

func TestParseRunRef(t *testing.T) {
	r, err := ParseRunRef("superpipeline:brd_01/run_01")
	if err != nil {
		t.Fatal(err)
	}
	if r.BoardID != "brd_01" || r.RunID != "run_01" || r.Source != "superpipeline" {
		t.Errorf("got %+v", r)
	}
	if r.String() != "superpipeline:brd_01/run_01" {
		t.Errorf("String() = %q", r.String())
	}
	for _, bad := range []string{"", "superpipeline:", "superpipeline:brd_01", "github:brd/run",
		"superpipeline:brd 01/run_01", `superpipeline:brd_01/run"01`, "superpipeline:brd_01/run_01/x"} {
		if _, err := ParseRunRef(bad); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("%q: err = %v", bad, err)
		}
	}
}

func TestWithTraceIDsCopies(t *testing.T) {
	ids := []string{"a"}
	r := RunRef{}.WithTraceIDs(ids)
	ids[0] = "b"
	if r.TraceIDs[0] != "a" {
		t.Error("WithTraceIDs aliased the caller's slice")
	}
}

func TestValidators(t *testing.T) {
	if !ValidAttemptID("attempt_01") || ValidAttemptID("run_01") || ValidAttemptID("attempt_") {
		t.Error("ValidAttemptID")
	}
	if !ValidTraceID("4bf92f3577b34da6a3ce929d0e0e4736") || ValidTraceID("4BF9") || ValidTraceID(`") or 1`) {
		t.Error("ValidTraceID")
	}
}

func TestAttemptLinkRef(t *testing.T) {
	s, run, board := "superpipeline", "run_01", "brd_01"
	if r, ok := (AttemptLink{ExternalSource: &s, ExternalRunID: &run, BoardID: &board}).Ref(); !ok || r.RunID != "run_01" {
		t.Errorf("got %+v %v", r, ok)
	}
	if _, ok := (AttemptLink{}).Ref(); ok {
		t.Error("an undispatched attempt has no run")
	}
}
