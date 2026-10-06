package api_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/fake"
)

// syncBuffer is a bytes.Buffer that handlers may write from several goroutines.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// auditLines decodes every transcript.read line in buf.
func auditLines(t *testing.T, buf *syncBuffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not a JSON log line: %q", line)
		}
		if m["msg"] == "transcript.read" {
			out = append(out, m)
		}
	}
	return out
}

func TestTranscriptAuditLine(t *testing.T) {
	var buf syncBuffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	d, err := fake.NewDev()
	if err != nil {
		t.Fatal(err)
	}
	a := newAppServerWith(t, nil, func(s *api.Server) { s.Ops.Transcripts, s.Ops.Logger = d, logger })

	a.call(t, "GET", txPath+"?seq_from=3", "", "cookie-human01", "", "")
	lines := auditLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("%d transcript.read lines for one read: %v", len(lines), lines)
	}
	want := map[string]any{"level": "INFO", "principal": "prn_human01", "via": "session", "run": "superpipeline:brd_01/run_01",
		"attempt": "attempt_01", "seq_from": 3.0, "seq_to": 9.0, "item": false, "full": false, "items": 4.0, "redactions": 0.0}
	for k, v := range want {
		if lines[0][k] != v {
			t.Errorf("%s = %v, want %v", k, lines[0][k], v)
		}
	}

	a.call(t, "GET", txPath+"/items/1?full=1", "dev:prn_grader01:agent:transcripts:read", "", "", "")
	item := auditLines(t, &buf)[1]
	for k, v := range map[string]any{"principal": "prn_grader01", "via": "bearer", "item": true, "full": true, "seq_from": 1.0,
		"items": 1.0, "redactions": 2.0} {
		if item[k] != v {
			t.Errorf("item read: %s = %v, want %v", k, item[k], v)
		}
	}

	a.call(t, "GET", txPath, "dev:prn_grader01:agent:evidence:read", "", "", "")
	refused := auditLines(t, &buf)[2]
	if refused["level"] != "WARN" || refused["code"] != "transcripts_forbidden" || refused["principal"] != "prn_grader01" {
		t.Errorf("refused read: %v", refused)
	}
	if _, ok := refused["items"]; ok {
		t.Errorf("a refused read reported items: %v", refused)
	}
	if n := len(auditLines(t, &buf)); n != 3 {
		t.Errorf("%d lines for three reads", n)
	}
}
