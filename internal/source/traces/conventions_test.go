package traces

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"sort"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/treaty"
)

// The trace search uses only attribute keys the suite's telemetry conventions name, and it
// searches every service the conventions say emits a span.
func TestSearchUsesConventionKeys(t *testing.T) {
	conv, err := treaty.TelemetryConventions()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(New("http://vt", "", nil).SearchURL("agentpod-hub", "run_01"))
	if err != nil {
		t.Fatal(err)
	}
	var tags map[string]string
	if err := json.Unmarshal([]byte(u.Query().Get("tags")), &tags); err != nil || len(tags) == 0 {
		t.Fatalf("tags %q: %v", u.Query().Get("tags"), err)
	}
	for k := range tags {
		if _, ok := conv.Attributes[k]; !ok {
			t.Errorf("the trace search queries %q, a key the conventions do not name", k)
		}
		if !slices.Contains(source.QueriedAttributes, k) {
			t.Errorf("the trace search queries %q, which source.QueriedAttributes does not list", k)
		}
	}
	for name, s := range conv.Spans {
		if !slices.Contains(DefaultServices, s.Service) {
			t.Errorf("span %s comes from %s, a service the search never asks", name, s.Service)
		}
	}
}

// Telemetry is best-effort: a span is shown with whatever attributes it carries. Every span case
// of the conventions, the invalid ones too (a missing key, a number as text), reads with its
// attributes intact; a key it lacks fails a producer's test, never this read.
func TestEverySpanCaseIsRead(t *testing.T) {
	conv, err := treaty.TelemetryConventions()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := treaty.TelemetryExamples()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(cases.Spans))
	for name := range cases.Spans {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c := cases.Spans[name]
		bodies := map[string]json.RawMessage{}
		for k, v := range c.Valid {
			bodies["valid: "+k] = v
		}
		for _, inv := range c.Invalid {
			bodies["invalid: "+inv.Why] = inv.Body
		}
		for label, body := range bodies {
			t.Run(name+"/"+label, func(t *testing.T) {
				var attrs map[string]any
				if err := json.Unmarshal(body, &attrs); err != nil {
					t.Fatal(err)
				}
				var tags []map[string]any
				want := map[string]string{}
				for k, v := range attrs {
					tags = append(tags, map[string]any{"key": k, "value": v})
					if s, ok := v.(string); ok {
						want[k] = s
					} else {
						b, _ := json.Marshal(v)
						want[k] = string(b)
					}
				}
				trace, _ := json.Marshal(map[string]any{"data": []any{map[string]any{
					"traceID": "4bf92f3577b34da6a3ce929d0e0e4736",
					"spans": []any{map[string]any{"traceID": "4bf92f3577b34da6a3ce929d0e0e4736", "spanID": "00f067aa0ba902b7",
						"operationName": name, "startTime": 1, "duration": 1, "tags": tags, "processID": "p1"}},
					"processes": map[string]any{"p1": map[string]any{"serviceName": conv.Spans[name].Service}},
				}}})
				srv := jaegerServer(t, map[string]string{"agentpod-hub": string(trace)}, nil)
				spans, st := New(srv.URL, "", srv.Client()).ListSpans(context.Background(), runRef(t))
				if st != source.StatusOK || len(spans) != 1 {
					t.Fatalf("status %s, %d spans", st, len(spans))
				}
				for k, v := range want {
					if spans[0].Attributes[k] != v {
						t.Errorf("%s = %q, want %q", k, spans[0].Attributes[k], v)
					}
				}
			})
		}
	}
}
