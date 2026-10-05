package runs

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

// Cursor is the last run of a page; the next page starts after it in list order.
type Cursor struct {
	SortAt      time.Time `json:"t"`
	Source      string    `json:"s"`
	ExternalRef string    `json:"r"`
}

func CursorOf(r Run) Cursor {
	return Cursor{SortAt: r.SortAt(), Source: r.Source, ExternalRef: r.ExternalRef}
}

var errCursor = errors.New("cursor is not one this API issued")

func EncodeCursor(c Cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func DecodeCursor(s string) (*Cursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, errCursor
	}
	var c Cursor
	if json.Unmarshal(b, &c) != nil || c.SortAt.IsZero() || c.Source == "" || c.ExternalRef == "" {
		return nil, errCursor
	}
	c.SortAt = c.SortAt.UTC()
	return &c, nil
}

// after reports whether r comes after c in list order: sort key descending, then source and
// external_ref ascending, byte by byte.
func (c Cursor) after(r Run) bool {
	at := r.SortAt()
	switch {
	case at.Before(c.SortAt):
		return true
	case at.After(c.SortAt):
		return false
	case r.Source != c.Source:
		return r.Source > c.Source
	}
	return r.ExternalRef > c.ExternalRef
}

func compareRuns(a, b Run) int {
	switch at, bt := a.SortAt(), b.SortAt(); {
	case at.After(bt):
		return -1
	case at.Before(bt):
		return 1
	case a.Source != b.Source:
		if a.Source < b.Source {
			return -1
		}
		return 1
	case a.ExternalRef < b.ExternalRef:
		return -1
	case a.ExternalRef > b.ExternalRef:
		return 1
	}
	return 0
}
