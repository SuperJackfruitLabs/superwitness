package canary

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
)

func zeroRand() *bytes.Reader { return bytes.NewReader(make([]byte, 64)) }

func TestMarkersAndNonceAreShaped(t *testing.T) {
	r := bytes.NewReader([]byte("0123456789abcdefghijklmn"))
	m, err := NewMarkers(r)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^swcanary[0-9a-f]{16}$`).MatchString(m.Plain) {
		t.Errorf("plain %q", m.Plain)
	}
	if !regexp.MustCompile(`^sk-swcanary-[0-9a-f]{24}$`).MatchString(m.Secret) {
		t.Errorf("secret %q", m.Secret)
	}
	n, err := newNonce(r)
	if err != nil || !regexp.MustCompile(`^swcerr-[0-9a-f]{8}$`).MatchString(n) {
		t.Errorf("nonce %q err %v", n, err)
	}
	if _, err := NewMarkers(bytes.NewReader([]byte("short"))); err == nil {
		t.Error("short random source accepted")
	}
}

func allPassing() []Check {
	var cs []Check
	for _, id := range RunFlows {
		cs = append(cs, Check{ID: id, Pass: true, Detail: "ok " + id, Query: "q " + id})
	}
	return cs
}

func TestResultPassNeedsAllNineChecksInOrder(t *testing.T) {
	r := Result{Checks: allPassing()}
	r.Finish(time.Now())
	if !r.Pass {
		t.Fatal("nine passing checks did not pass")
	}
	r.Checks = r.Checks[:8]
	r.Finish(time.Now())
	if r.Pass {
		t.Error("eight checks passed")
	}
	r.Checks = allPassing()
	r.Error = "setup"
	r.Finish(time.Now())
	if r.Pass {
		t.Error("a result with an error passed")
	}
}

func TestDeriveProofsSkipsFlowSixAndFailures(t *testing.T) {
	at := time.Date(2026, 10, 10, 2, 40, 0, 0, time.UTC)
	r := Result{BoardID: "brd_c", RunID: "run_1", Checks: allPassing()}
	r.Checks[4].Pass = false // flow 5
	r.Finish(at)
	r.DeriveProofs()
	var flows []int
	for _, p := range r.Proofs {
		flows = append(flows, p.Flow)
		if p.RunRef != "superpipeline:brd_c/run_1" || !p.At.Equal(at) || p.Query == "" || p.Output == "" {
			t.Errorf("proof %+v", p)
		}
	}
	if got := len(flows); got != 7 { // 1,2,3,4,7,8,9
		t.Fatalf("proved flows %v", flows)
	}
	for _, f := range flows {
		if f == 5 || f == 6 {
			t.Errorf("flow %d proved", f)
		}
	}
	none := Result{Checks: allPassing()}
	none.Finish(at)
	none.DeriveProofs()
	if len(none.Proofs) != 0 {
		t.Error("a result with no run id proved flows")
	}
}

func TestRedactReplacesNeedlesEverywhere(t *testing.T) {
	r := Result{Error: "saw swcanaryX", Checks: []Check{{Detail: "and sk-swcanary-Y", Query: "swcanaryX"}},
		GateResolutions: []GateResolution{{Detail: "swcanaryX again"}}}
	r.Redact(map[string]string{LabelPlain: "swcanaryX", LabelSecretMarker: "sk-swcanary-Y"})
	b, _ := json.Marshal(r)
	if bytes.Contains(b, []byte("swcanaryX")) || bytes.Contains(b, []byte("sk-swcanary-Y")) {
		t.Fatalf("needle survived: %s", b)
	}
	if !strings.Contains(r.Error, "<marker_plain>") {
		t.Errorf("error = %q", r.Error)
	}
}

func TestDecodeRunDocKeepsUnknownsAndErrors(t *testing.T) {
	doc, err := DecodeRunDoc(strings.NewReader(`{
	  "run": {"ref": "superpipeline:brd_c/run_1", "card": {"id": "crd_1", "title": "t"}, "agent": "unknown"},
	  "attempts": [{"id": "attempt_1", "fingerprint": {"digest": "unknown"}, "span_count": "unknown"}],
	  "trace": {"trace_ids": [], "status": "none", "sampled": true},
	  "errors": [{"service": "superwitness-canary", "message": "m", "trace_id": "t1", "at": "x"}],
	  "verdicts": [{"id": "gate_1", "kind": "gate", "value": {"decision": null}, "status": "pending"}],
	  "sources": {"traces": "timeout"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Attempts[0].SpanCount(); ok {
		t.Error("span_count \"unknown\" read as a number")
	}
	if doc.Run.Card.ID != "crd_1" || doc.Errors[0].TraceID != "t1" || doc.Verdicts[0].Status != "pending" {
		t.Errorf("doc = %+v", doc)
	}
	if doc.Source("traces") != "timeout" || doc.Source("logs") != "missing" {
		t.Errorf("sources = %v", doc.Sources)
	}
}

func TestDecodeRunDocToleratesFlagStates(t *testing.T) {
	for _, tc := range []struct{ sampled, want string }{
		{`"sampled":"unknown"`, "unknown"}, {`"sampled":true`, "true"}, {`"sampled":false`, "false"}, {``, ""},
	} {
		body := `{"trace":{"trace_ids":["t"],"status":"ok"` + map[bool]string{true: "," + tc.sampled, false: ""}[tc.sampled != ""] + `},
		  "attempts":[{"id":"a","span_count":7}]}`
		doc, err := DecodeRunDoc(strings.NewReader(body))
		if err != nil {
			t.Fatalf("%s: %v", tc.sampled, err)
		}
		if got := doc.Trace.SampledState(); got != tc.want {
			t.Errorf("%s: state %q, want %q", tc.sampled, got, tc.want)
		}
		if n, ok := doc.Attempts[0].SpanCount(); !ok || n != 7 {
			t.Errorf("span_count = %d %v", n, ok)
		}
	}
}
