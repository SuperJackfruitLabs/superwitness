package verdicts

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// Scale is a rubric scale in one of the shapes the app can offer an input for. A legacy
// {"min":a,"max":b} is a score over that range; the app sends its value rescaled to 0..1.
type Scale struct {
	Kind    string   `json:"kind"` // decision | score | label | text
	Options []string `json:"options,omitempty"`
	Labels  []string `json:"labels,omitempty"`
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
}

// RecogniseScale reads a rubric's free-form scale. It is false for any other shape.
func RecogniseScale(raw json.RawMessage) (Scale, bool) {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return Scale{}, false
	}
	k, hasKind := m["kind"]
	if !hasKind {
		return legacyScore(m)
	}
	var kind string
	if json.Unmarshal(k, &kind) != nil {
		return Scale{}, false
	}
	switch kind {
	case "decision":
		if opts, ok := stringList(m, "options", 2, 20); ok && len(m) == 2 {
			return Scale{Kind: "decision", Options: opts}, true
		}
	case "label":
		if labels, ok := stringList(m, "labels", 1, 50); ok && len(m) == 2 {
			return Scale{Kind: "label", Labels: labels}, true
		}
	case "score":
		if len(m) == 1 {
			lo, hi := 0.0, 1.0
			return Scale{Kind: "score", Min: &lo, Max: &hi}, true
		}
	case "text":
		if len(m) == 1 {
			return Scale{Kind: "text"}, true
		}
	}
	return Scale{}, false
}

func legacyScore(m map[string]json.RawMessage) (Scale, bool) {
	var lo, hi *float64
	if len(m) != 2 || json.Unmarshal(m["min"], &lo) != nil || json.Unmarshal(m["max"], &hi) != nil || lo == nil || hi == nil || !(*lo < *hi) {
		return Scale{}, false
	}
	return Scale{Kind: "score", Min: lo, Max: hi}, true
}

func stringList(m map[string]json.RawMessage, key string, minN, maxN int) ([]string, bool) {
	var xs []string
	if json.Unmarshal(m[key], &xs) != nil || len(xs) < minN || len(xs) > maxN {
		return nil, false
	}
	seen := map[string]bool{}
	for _, x := range xs {
		if strings.TrimSpace(x) == "" || utf8.RuneCountInString(x) > 64 || seen[x] {
			return nil, false
		}
		seen[x] = true
	}
	return xs, true
}
