package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

// DevSession is attempt_01's session in the development data set.
const DevSession = "acps_01"

const devSessionEnd = 9 // the session's last seq

// devChangelog is a tool output over the hub's 16 KiB cut: 1,200 lines.
func devChangelog() string {
	var b strings.Builder
	b.WriteString("## 0.4\n")
	for i := 0; i < 1200; i++ {
		fmt.Fprintf(&b, "- change %d: a line of the development changelog\n", i)
	}
	return b.String()
}

// devItems is the development transcript as the hub answers it: folded and redacted. In a page
// (whole false) tc_01's output text is cut at 16 KiB; the item route returns it whole. Each call
// builds fresh maps, so a caller may mark one partial.
func devItems(whole bool) []map[string]any {
	changelog := devChangelog()
	var text any = changelog
	if !whole {
		text = map[string]any{"truncated": true, "bytes": len(changelog), "head": changelog[:16<<10]}
	}
	return []map[string]any{
		{"kind": "prompt", "seq": 1, "images": []any{}, "redactions": 2,
			"text": "Draft the notes for version 0.4 from CHANGELOG.md. Registry key: [redacted:anthropic-key]. " +
				"Header: Authorization: Bearer [redacted:authorization]."},
		{"kind": "reasoning", "seq_from": 2, "seq_to": 2, "redactions": 0,
			"text": "The entries are in the changelog; read it before writing."},
		{"kind": "tool_call", "id": "tc_01", "seq_from": 3, "seq_to": 4, "title": "Read CHANGELOG.md", "tool_kind": "read",
			"status": "completed", "redactions": 0, "input": map[string]any{"path": "CHANGELOG.md"},
			"output": map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "raw": nil}},
		{"kind": "permission", "seq": 5, "answer_seq": 6, "tool_call_id": "tc_02", "title": "Write release-note.md", "redactions": 0,
			"options": []any{
				map[string]any{"optionId": "allow_once", "name": "Allow once", "kind": "allow_once"},
				map[string]any{"optionId": "reject_once", "name": "Reject", "kind": "reject_once"},
			},
			"outcome": "selected:allow_once"},
		{"kind": "tool_call", "id": "tc_02", "seq_from": 7, "seq_to": 8, "title": "Write release-note.md", "tool_kind": "edit",
			"status": "failed", "redactions": 0,
			"input":  map[string]any{"path": "release-note.md", "content": "# 0.4\n\nA draft of the notes."},
			"output": map[string]any{"content": []any{map[string]any{"type": "text", "text": "permission denied: release-note.md is read-only"}}, "raw": nil}},
		{"kind": "message", "seq_from": 9, "seq_to": 9, "redactions": 0,
			"text": "I could not write the notes: release-note.md is read-only."},
	}
}

// devRange is an item's first and last seq.
func devRange(it map[string]any) (int64, int64) {
	if s, ok := it["seq"].(int); ok {
		if a, ok := it["answer_seq"].(int); ok {
			return int64(s), int64(a)
		}
		return int64(s), int64(s)
	}
	return int64(it["seq_from"].(int)), int64(it["seq_to"].(int))
}

// TranscriptPage answers like the hub's transcript route for DevSession, in one page: every item
// that overlaps the range, an item that starts before it marked partial.
func (d *Dev) TranscriptPage(_ context.Context, q source.TranscriptQuery) (json.RawMessage, source.SourceStatus) {
	if q.SessionID != DevSession {
		return nil, source.StatusNotFound
	}
	from, to := int64(1), int64(devSessionEnd)
	if q.SeqFrom != nil {
		from = *q.SeqFrom
	}
	if q.SeqTo != nil {
		to = *q.SeqTo
	}
	if from < 1 || to > devSessionEnd || from > to {
		return nil, source.StatusBadRange
	}
	items := []map[string]any{}
	redactions, truncated := 0, 0
	for _, it := range devItems(false) {
		lo, hi := devRange(it)
		if hi < from || lo > to {
			continue
		}
		if lo < from {
			it["partial"] = true
		}
		redactions += it["redactions"].(int)
		if it["id"] == "tc_01" {
			truncated++
		}
		items = append(items, it)
	}
	b, err := json.Marshal(map[string]any{"session_id": DevSession, "seq_from": from, "seq_to": to, "items": items,
		"next_cursor": nil, "redactions": redactions, "truncated_fields": truncated})
	if err != nil {
		return nil, source.StatusUnavailable
	}
	return b, source.StatusOK
}

// TranscriptItem answers like the hub's item route: the item that starts at SeqFrom, whole.
func (d *Dev) TranscriptItem(_ context.Context, q source.ItemQuery) (json.RawMessage, source.SourceStatus) {
	if q.SessionID != DevSession {
		return nil, source.StatusNotFound
	}
	for _, it := range devItems(true) {
		if lo, _ := devRange(it); lo == q.SeqFrom {
			b, err := json.Marshal(map[string]any{"session_id": DevSession, "item": it})
			if err != nil {
				return nil, source.StatusUnavailable
			}
			return b, source.StatusOK
		}
	}
	return nil, source.StatusNotFound
}
