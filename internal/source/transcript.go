package source

import (
	"context"
	"encoding/json"
)

// The hub's transcript routes answer two things no other source does.
const (
	StatusTooLarge SourceStatus = "too_large" // 413: the item is over the hub's 1 MiB
	StatusBadRange SourceStatus = "bad_range" // 400 bad_range: the range is outside the session
)

// TranscriptQuery reads one page of a session's transcript. A nil bound is the hub's default,
// the session's first or last seq.
type TranscriptQuery struct {
	SessionID  string
	SeqFrom    *int64
	SeqTo      *int64
	Cursor     string // the hub's next_cursor, opaque
	OnBehalfOf string // the caller's prn_ id, sent as X-On-Behalf-Of; anything else sends none
}

// ItemQuery reads one transcript item whole, by its first seq.
type ItemQuery struct {
	SessionID  string
	SeqFrom    int64
	Full       bool
	RangeFrom  *int64 // the range the item was shown in; nil sends none and the hub folds the whole session
	RangeTo    *int64
	OnBehalfOf string
}

// TranscriptReader reads session content from the hub. The body is the hub's JSON exactly:
// superwitness passes it on and never stores, caches or logs it.
type TranscriptReader interface {
	TranscriptPage(ctx context.Context, q TranscriptQuery) (json.RawMessage, SourceStatus)
	TranscriptItem(ctx context.Context, q ItemQuery) (json.RawMessage, SourceStatus)
}
