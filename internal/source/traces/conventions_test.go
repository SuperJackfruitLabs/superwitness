package traces

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"sort"
	"strings"
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

// serveTags lists one span with the given tags from the hub and returns it as SuperWitness reads it.
func serveTags(t *testing.T, name, service string, tags []map[string]any) source.Span {
	t.Helper()
	trace, _ := json.Marshal(map[string]any{"data": []any{map[string]any{
		"traceID": "4bf92f3577b34da6a3ce929d0e0e4736",
		"spans": []any{map[string]any{"traceID": "4bf92f3577b34da6a3ce929d0e0e4736", "spanID": "00f067aa0ba902b7",
			"operationName": name, "startTime": 1, "duration": 1, "tags": tags, "processID": "p1"}},
		"processes": map[string]any{"p1": map[string]any{"serviceName": service}},
	}}})
	srv := jaegerServer(t, map[string]string{"agentpod-hub": string(trace)}, nil)
	spans, st := New(srv.URL, "", srv.Client()).ListSpans(context.Background(), runRef(t))
	if st != source.StatusOK || len(spans) != 1 {
		t.Fatalf("status %s, %d spans", st, len(spans))
	}
	return spans[0]
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Telemetry is best-effort: a span is shown with whatever attributes it carries. Every span case
// of the conventions, the invalid ones too (a missing key, a number as text), reads with every
// key it has; a valid case comes back with its values and their types as the producer set them.
// A key a case lacks fails a producer's test, never this read.
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
				for k, v := range attrs {
					tags = append(tags, map[string]any{"key": k, "value": v})
				}
				got := serveTags(t, name, conv.Spans[name].Service, tags).Attributes
				for k, v := range attrs {
					if _, ok := got[k]; !ok {
						t.Errorf("%s is missing", k)
					} else if strings.HasPrefix(label, "valid: ") && jsonOf(t, got[k]) != jsonOf(t, v) {
						t.Errorf("%s = %s, want %s", k, jsonOf(t, got[k]), jsonOf(t, v))
					}
				}
			})
		}
	}
}

// A trace store's Jaeger API may send every tag as a string, numbers included (VictoriaTraces
// does: `{"key": "acp.seq_from", "type": "string", "value": "1"}`). Every valid span case sent
// that way is served with the conventions' types again, so it passes the same check the cases
// are held to; a key the conventions do not name stays the string the store sent.
func TestStoreStringsAreServedInConventionTypes(t *testing.T) {
	conv, err := treaty.TelemetryConventions()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := treaty.TelemetryExamples()
	if err != nil {
		t.Fatal(err)
	}
	numeric := 0
	for name, c := range cases.Spans {
		for label, body := range c.Valid {
			t.Run(name+"/"+label, func(t *testing.T) {
				var attrs map[string]any
				if err := json.Unmarshal(body, &attrs); err != nil {
					t.Fatal(err)
				}
				var tags []map[string]any
				for k, v := range attrs {
					text, ok := v.(string)
					if !ok {
						text = strings.Trim(jsonOf(t, v), `"`)
					}
					tags = append(tags, map[string]any{"key": k, "type": "string", "value": text})
				}
				got := serveTags(t, name, conv.Spans[name].Service, tags).Attributes
				for k, v := range attrs {
					a, named := conv.Attributes[k]
					switch {
					case !named:
						if want := strings.Trim(jsonOf(t, v), `"`); got[k] != want {
							t.Errorf("%s (not in the conventions) = %#v, want the store's %q", k, got[k], want)
						}
					case !a.Admits(got[k]):
						t.Errorf("%s = %#v, not a conventions %s", k, got[k], a.Type)
					case jsonOf(t, got[k]) != jsonOf(t, v):
						t.Errorf("%s = %s, want %s", k, jsonOf(t, got[k]), jsonOf(t, v))
					}
					if _, isText := v.(string); named && !isText {
						numeric++
					}
				}
			})
		}
	}
	if numeric == 0 {
		t.Fatal("no valid case has a typed attribute, so nothing was coerced")
	}
}

// A value that does not read as its type is served as the store sent it, and the span with it.
// A tag the store types int64, float64 or bool is read as that type, whatever its key.
func TestTagTypesAndUnreadableValues(t *testing.T) {
	got := serveTags(t, "turn", "agentpod-hub", []map[string]any{
		{"key": "attempt.id", "type": "string", "value": "attempt_01"},
		{"key": "acp.seq_from", "type": "string", "value": "three"},
		{"key": "acp.seq_to", "type": "string", "value": "1.5"},
		{"key": "run.epoch", "value": "2"},
		{"key": "http.response.status_code", "type": "int64", "value": "200"},
		{"key": "http.response.status_code.text", "type": "string", "value": "200"},
		{"key": "sampled", "type": "bool", "value": "true"},
		{"key": "ratio", "type": "float64", "value": "0.25"},
		{"key": "garbled", "type": "int64", "value": "lots"},
		{"key": "card.id", "type": "string", "value": 42},
	}).Attributes
	want := map[string]any{
		"attempt.id": "attempt_01", "acp.seq_from": "three", "acp.seq_to": "1.5", "run.epoch": int64(2),
		"http.response.status_code": int64(200), "http.response.status_code.text": "200",
		"sampled": true, "ratio": 0.25, "garbled": "lots", "card.id": "42",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v", k, got[k], v)
		}
	}
}
