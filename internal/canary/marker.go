package canary

import (
	"encoding/hex"
	"io"
)

// skPrefix makes the secret marker look like an API key.
const skPrefix = "sk-swcanary-"

// Needle labels. A ScanResult reports hits by label, so a marker's value never has to
// leave this process in anything the canary prints or stores.
const (
	LabelPlain        = "marker_plain"
	LabelSecretMarker = "marker_secret"
	LabelControlRun   = "control_run"
	LabelControlTrace = "control_trace"
)

// Markers are planted in the card spec, which becomes the agent's prompt. Plain is one
// token with no separators, so a tokenising log index keeps it whole. Secret is shaped
// like an API key, so it also exercises the collector's redaction.
type Markers struct {
	Plain  string
	Secret string
}

func NewMarkers(r io.Reader) (Markers, error) {
	b := make([]byte, 20)
	if _, err := io.ReadFull(r, b); err != nil {
		return Markers{}, err
	}
	h := hex.EncodeToString(b)
	return Markers{Plain: "swcanary" + h[:16], Secret: skPrefix + h[16:]}, nil
}

func (m Markers) Needles() map[string]string {
	return map[string]string{LabelPlain: m.Plain, LabelSecretMarker: m.Secret}
}

// newNonce names the injected error line (flow 8). It is not a marker: it is meant to be
// found in telemetry.
func newNonce(r io.Reader) (string, error) {
	b := make([]byte, 4)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return "swcerr-" + hex.EncodeToString(b), nil
}

// Controls are needles that MUST be found. A scan that finds no control proves nothing
// about absence: its window, index or credentials are wrong.
func Controls(runID string, traceIDs []string) map[string]string {
	c := map[string]string{LabelControlRun: runID}
	if len(traceIDs) > 0 {
		c[LabelControlTrace] = traceIDs[0]
	}
	return c
}

func merge(ms ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range ms {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
