package runs

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)

func TestParseSingle(t *testing.T) {
	body := report(map[string]any{"scope": map[string]any{"id": "brd_01", "name": "Press"},
		"started_at": "2026-10-06T09:00:00.1234567+05:30", "title": "Draft the release note"})
	got, batch, err := ParseBody([]byte(body), t0)
	if err != nil || batch || len(got) != 1 {
		t.Fatalf("%v %v %v", got, batch, err)
	}
	r := got[0]
	if r.Source != "superpipeline" || r.ExternalRef != "brd_01/run_01" || *r.ScopeID != "brd_01" || *r.ScopeName != "Press" ||
		r.Executor != nil || r.Status != "running" || r.SubjectRef() != "superpipeline:brd_01/run_01" {
		t.Errorf("run = %+v", r)
	}
	if want := time.Date(2026, 10, 6, 3, 30, 0, 123456000, time.UTC); !r.StartedAt.Equal(want) || r.StartedAt.Location() != time.UTC {
		t.Errorf("started_at = %v, want %v", r.StartedAt, want)
	}
	low, _, err := ParseBody([]byte(report(map[string]any{"reported_at": "2026-10-06t10:00:00z"})), t0)
	if err != nil || !low[0].ReportedAt.Equal(t0) {
		t.Errorf("lowercase t and z: %v %v", low, err)
	}
}

func TestParseBatchReportsTheBadIndex(t *testing.T) {
	body := `{"runs":[` + report(nil) + `,` + report(nil) + `,` + report(map[string]any{"status": "done"}) + `]}`
	_, batch, err := ParseBody([]byte(body), t0)
	var ve *ValidationError
	if !batch || !errors.As(err, &ve) || ve.Index != 2 || ve.Field != "status" || !strings.HasPrefix(ve.Error(), "runs[2].status: ") {
		t.Fatalf("batch=%v err=%v", batch, err)
	}
	_, _, err = ParseBody([]byte(report(map[string]any{"status": "done"})), t0)
	if !errors.As(err, &ve) || ve.Index != -1 || !strings.HasPrefix(ve.Error(), "status: ") {
		t.Errorf("single report: %v", err)
	}
	var be *BodyError
	if _, _, err := ParseBody([]byte(`{"runs":[]}`), t0); !errors.As(err, &be) {
		t.Errorf("empty batch: %v; want a BodyError", err)
	}
}

func TestReportedAtInTheFuture(t *testing.T) {
	ok := report(map[string]any{"reported_at": t0.Add(FutureSkew).Format(time.RFC3339)})
	if _, _, err := ParseBody([]byte(ok), t0); err != nil {
		t.Errorf("exactly FutureSkew ahead: %v", err)
	}
	ahead := report(map[string]any{"reported_at": t0.Add(FutureSkew + time.Second).Format(time.RFC3339)})
	_, _, err := ParseBody([]byte(ahead), t0)
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "reported_at" || !strings.Contains(ve.Msg, "ahead") {
		t.Errorf("more than FutureSkew ahead: %v", err)
	}
}

func TestRunJSON(t *testing.T) {
	got, _, err := ParseBody([]byte(report(map[string]any{"executor": map[string]any{"name": "drafter"}})), t0)
	if err != nil {
		t.Fatal(err)
	}
	r := got[0]
	r.FirstSeenAt, r.UpdatedAt = t0, t0
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"ref":"superpipeline:brd_01/run_01"`, `"scope":null`, `"title":null`,
		`"executor":{"id":null,"name":"drafter"}`, `"started_at":null`, `"latest_verdict":null`,
		`"first_seen_at":"2026-10-06T10:00:00Z"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("%s lacks %s", b, want)
		}
	}
}
