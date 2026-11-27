package agentpod

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
)

// The transcript routes' bodies pass through to the caller exactly as the hub sent them, but
// only once they have the shape agentpod.evidence gives them (TranscriptPage,
// TranscriptItemResponse and WireItem): a body that does not is unavailable, never shown. The
// shapes are open: a key not named here is passed on untouched.

const maxPageItems = 200

type object map[string]json.RawMessage

// validPage reports whether body is a transcript page of session.
func validPage(body []byte, session string) bool {
	o, ok := asObject(body)
	if !ok || !o.session(session) || !o.ints(0, "seq_from", "seq_to", "redactions", "truncated_fields") {
		return false
	}
	if c, ok := o["next_cursor"]; !ok || (!isNull(c) && !nonEmptyString(c)) {
		return false
	}
	var items []json.RawMessage
	if json.Unmarshal(o["items"], &items) != nil || items == nil || len(items) > maxPageItems {
		return false
	}
	for _, it := range items {
		if !validItem(it) {
			return false
		}
	}
	return true
}

// validItemResponse reports whether body is one transcript item of session.
func validItemResponse(body []byte, session string) bool {
	o, ok := asObject(body)
	return ok && o.session(session) && validItem(o["item"])
}

// validItem is WireItem: one of eight kinds, each with its own required keys, and always its
// redaction count.
func validItem(raw json.RawMessage) bool {
	o, ok := asObject(raw)
	if !ok || !o.ints(0, "redactions") {
		return false
	}
	var kind string
	if json.Unmarshal(o["kind"], &kind) != nil {
		return false
	}
	switch kind {
	case "prompt":
		return o.ints(1, "seq") && o.text("text") && o.list("images", "name", "mimeType")
	case "message", "reasoning":
		return o.ints(1, "seq_from", "seq_to") && o.text("text")
	case "tool_call":
		var status string
		out, outOK := asObject(o["output"])
		_, hasInput := o["input"]
		return o.ints(1, "seq_from", "seq_to") && o.strings("id") && o.text("title") && o.nullableString("tool_kind") &&
			json.Unmarshal(o["status"], &status) == nil && oneOf(status, "pending", "in_progress", "completed", "failed") &&
			hasInput && outOK && out.has("content", "raw") && o.partial()
	case "permission":
		var outcome string
		return o.ints(1, "seq") && o.optionalInts(1, "answer_seq") && o.optionalStrings("tool_call_id") && o.text("title") &&
			o.list("options", "optionId", "name", "kind") && json.Unmarshal(o["outcome"], &outcome) == nil &&
			(oneOf(outcome, "cancelled", "auto", "pending") || (strings.HasPrefix(outcome, "selected:") && len(outcome) > len("selected:"))) &&
			o.partial()
	case "state":
		_, hasReason := o["reason"]
		return o.ints(1, "seq") && o.strings("status") && (!hasReason || o.text("reason"))
	case "error":
		return o.ints(1, "seq") && o.strings("error_kind") && o.text("message")
	case "other":
		return o.ints(1, "seq") && o.strings("type")
	default:
		return false
	}
}

func asObject(raw json.RawMessage) (object, bool) {
	var o object
	if len(raw) == 0 || json.Unmarshal(raw, &o) != nil || o == nil {
		return nil, false
	}
	return o, true
}

func (o object) has(keys ...string) bool {
	for _, k := range keys {
		if _, ok := o[k]; !ok {
			return false
		}
	}
	return true
}

// session: the session the page or item is about, acps_…, and the one asked for.
func (o object) session(want string) bool {
	var s string
	return json.Unmarshal(o["session_id"], &s) == nil && strings.HasPrefix(s, "acps_") && s == want
}

// ints: each key is a whole number at least min.
func (o object) ints(min float64, keys ...string) bool {
	for _, k := range keys {
		if !wholeAtLeast(o[k], min) {
			return false
		}
	}
	return true
}

func (o object) optionalInts(min float64, keys ...string) bool {
	for _, k := range keys {
		if v, ok := o[k]; ok && !wholeAtLeast(v, min) {
			return false
		}
	}
	return true
}

func (o object) strings(keys ...string) bool {
	for _, k := range keys {
		if !isString(o[k]) {
			return false
		}
	}
	return true
}

func (o object) optionalStrings(keys ...string) bool {
	for _, k := range keys {
		if v, ok := o[k]; ok && !isString(v) {
			return false
		}
	}
	return true
}

func (o object) nullableString(key string) bool {
	v, ok := o[key]
	return ok && (isNull(v) || isString(v))
}

// text is the contract's Text: a string, or a truncation marker with its size and head.
func (o object) text(key string) bool {
	v := o[key]
	if isString(v) {
		return true
	}
	m, ok := asObject(v)
	return ok && bytes.Equal(bytes.TrimSpace(m["truncated"]), []byte("true")) && wholeAtLeast(m["bytes"], 1) && isString(m["head"])
}

// list: key is an array of objects, each with these string keys.
func (o object) list(key string, fields ...string) bool {
	var values []json.RawMessage
	if json.Unmarshal(o[key], &values) != nil || values == nil {
		return false
	}
	for _, v := range values {
		m, ok := asObject(v)
		if !ok || !m.strings(fields...) {
			return false
		}
	}
	return true
}

// partial, when present, is true: a partial item says so, a whole one leaves it out.
func (o object) partial() bool {
	v, ok := o["partial"]
	return !ok || bytes.Equal(bytes.TrimSpace(v), []byte("true"))
}

func wholeAtLeast(raw json.RawMessage, min float64) bool {
	var n float64
	if len(raw) == 0 || isNull(raw) || json.Unmarshal(raw, &n) != nil {
		return false
	}
	return n >= min && n == math.Trunc(n)
}

func isNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

func isString(raw json.RawMessage) bool {
	var s string
	return len(raw) > 0 && !isNull(raw) && json.Unmarshal(raw, &s) == nil
}

func nonEmptyString(raw json.RawMessage) bool {
	var s string
	return !isNull(raw) && json.Unmarshal(raw, &s) == nil && s != ""
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
