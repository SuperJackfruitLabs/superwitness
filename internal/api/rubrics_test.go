package api_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

func TestRubricRoutes(t *testing.T) {
	r := newRegistryServer(t)
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, rb := range []verdicts.Rubric{
		{ID: "press", Version: 1, Name: "Press", Scale: json.RawMessage(`{"min":0,"max":1}`), Body: "Reads well.", CreatedBy: "prn_human01", CreatedAt: at},
		{ID: "press", Version: 2, Name: "Press", Scale: json.RawMessage(`{"kind":"decision","options":["pass","fail"]}`), Body: "Pass or fail.", CreatedBy: "prn_human01", CreatedAt: at},
		{ID: "odd", Version: 1, Name: "Odd", Scale: json.RawMessage(`{"stars":5}`), Body: "?", CreatedBy: "prn_human01", CreatedAt: at},
	} {
		if err := r.store.InsertRubric(t.Context(), rb); err != nil {
			t.Fatal(err)
		}
	}
	resp, b := do(t, r.srv, "GET", "/v1/rubrics", humanTok, "")
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	var list struct {
		Rubrics []struct {
			Standard        string          `json:"standard"`
			RecognisedScale json.RawMessage `json:"recognised_scale"`
			Body            *string         `json:"body"`
		} `json:"rubrics"`
	}
	_ = json.Unmarshal(b, &list)
	if len(list.Rubrics) != 3 || list.Rubrics[0].Standard != "rubric:odd@1" || string(list.Rubrics[0].RecognisedScale) != "null" ||
		list.Rubrics[1].Standard != "rubric:press@2" || list.Rubrics[0].Body != nil {
		t.Errorf("list = %s", b)
	}
	if !strings.Contains(string(b), `"recognised_scale":{"kind":"score","min":0,"max":1}`) {
		t.Errorf("the legacy scale is not recognised as score: %s", b)
	}
	resp, b = do(t, r.srv, "GET", "/v1/rubrics/press/2", humanTok, "")
	if resp.StatusCode != 200 || !strings.Contains(string(b), `"body":"Pass or fail."`) ||
		!strings.Contains(string(b), `"recognised_scale":{"kind":"decision","options":["pass","fail"]}`) {
		t.Errorf("one rubric: %d %s", resp.StatusCode, b)
	}
	for path, want := range map[string]string{
		"/v1/rubrics/press/9":   "rubric_not_found",
		"/v1/rubrics/press/0":   "invalid_rubric_ref",
		"/v1/rubrics/press/x":   "invalid_rubric_ref",
		"/v1/rubrics/pr%20ss/1": "invalid_rubric_ref",
	} {
		if _, b := do(t, r.srv, "GET", path, humanTok, ""); errCode(t, b) != want {
			t.Errorf("%s: %s; want %s", path, b, want)
		}
	}
}
