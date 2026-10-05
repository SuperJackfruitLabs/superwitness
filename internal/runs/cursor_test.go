package runs

import (
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	c := Cursor{SortAt: time.Date(2026, 10, 6, 10, 0, 0, 123456000, time.UTC), Source: "superpipeline", ExternalRef: "brd_01/run_01"}
	got, err := DecodeCursor(EncodeCursor(c))
	if err != nil || got == nil || !got.SortAt.Equal(c.SortAt) || got.Source != c.Source || got.ExternalRef != c.ExternalRef {
		t.Errorf("round trip: %+v %v", got, err)
	}
	if got, err := DecodeCursor(""); got != nil || err != nil {
		t.Errorf("empty: %v %v", got, err)
	}
	for _, bad := range []string{"!!!", "e30", "eyJ0IjoieCJ9"} { // not base64; {}; {"t":"x"}
		if _, err := DecodeCursor(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
