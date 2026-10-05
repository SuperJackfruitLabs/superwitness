package verdicts

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRecogniseScale(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	for _, c := range []struct {
		raw  string
		want *Scale
	}{
		{`{"kind":"decision","options":["pass","fail"]}`, &Scale{Kind: "decision", Options: []string{"pass", "fail"}}},
		{`{"kind":"score"}`, &Scale{Kind: "score", Min: f(0), Max: f(1)}},
		{`{"kind":"label","labels":["clear"]}`, &Scale{Kind: "label", Labels: []string{"clear"}}},
		{`{"kind":"text"}`, &Scale{Kind: "text"}},
		{`{"min":0,"max":1}`, &Scale{Kind: "score", Min: f(0), Max: f(1)}},
		{`{"min":1,"max":5}`, &Scale{Kind: "score", Min: f(1), Max: f(5)}},

		{`{"kind":"decision","options":["pass"]}`, nil},             // fewer than two options
		{`{"kind":"decision","options":["pass","pass"]}`, nil},      // duplicates
		{`{"kind":"decision","options":["pass",""]}`, nil},          // an empty option
		{`{"kind":"decision","options":["a","b"],"note":"x"}`, nil}, // another key
		{`{"kind":"label","labels":[]}`, nil},
		{`{"kind":"score","min":0}`, nil},
		{`{"kind":"vibes"}`, nil},
		{`{"min":1,"max":1}`, nil},
		{`{"min":"0","max":1}`, nil},
		{`{"min":0,"max":1,"step":0.1}`, nil},
		{`[]`, nil},
		{`"score"`, nil},
	} {
		got, ok := RecogniseScale(json.RawMessage(c.raw))
		if (c.want != nil) != ok {
			t.Errorf("%s: recognised = %v, want %v", c.raw, ok, c.want != nil)
			continue
		}
		if c.want == nil {
			continue
		}
		gb, _ := json.Marshal(got)
		wb, _ := json.Marshal(c.want)
		if string(gb) != string(wb) {
			t.Errorf("%s: %s, want %s", c.raw, gb, wb)
		}
	}
	if _, ok := RecogniseScale(json.RawMessage(`{"kind":"decision","options":["` + strings.Repeat("o", 65) + `","b"]}`)); ok {
		t.Error("an option over 64 characters was recognised")
	}
}
