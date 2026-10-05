package canary

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
)

// ScanResult reports needle hits by LABEL. Where never contains a needle's value.
type ScanResult struct {
	Scanned   int
	Hits      map[string]int
	Where     map[string]string
	Truncated bool
}

func newScanResult() ScanResult {
	return ScanResult{Hits: map[string]int{}, Where: map[string]string{}}
}

func (s *ScanResult) hit(label, where string) {
	s.Hits[label]++
	if _, ok := s.Where[label]; !ok {
		s.Where[label] = where
	}
}

// redact replaces every needle value in where with "<label>", so a location copied from
// telemetry (a span name, a stream label) can never carry a marker into a report.
func redact(where string, needles map[string]string) string {
	labels := make([]string, 0, len(needles))
	for l, n := range needles {
		if n != "" {
			labels = append(labels, l)
		}
	}
	sort.Slice(labels, func(i, j int) bool {
		if len(needles[labels[i]]) != len(needles[labels[j]]) {
			return len(needles[labels[i]]) > len(needles[labels[j]])
		}
		return labels[i] < labels[j]
	})
	for _, l := range labels {
		where = strings.ReplaceAll(where, needles[l], "<"+l+">")
	}
	return where
}

func setBearer(req *http.Request, tokenFile string) error {
	if tokenFile == "" {
		return nil
	}
	tok, err := auth.ReadSecretFile(tokenFile) // the service's reader: trims, refuses group/world-readable and empty files
	if err != nil {
		return fmt.Errorf("reading token file: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return nil
}

// CheckMarkerAbsent is flow 9. Absence is only believed when both queries succeeded,
// neither was truncated, and both found their positive control. A leak is reported even
// when a control is missing, because a hit is proof on its own.
func CheckMarkerAbsent(logs ScanResult, logsErr error, traces ScanResult, tracesErr error) Check {
	c := check(9)
	if logsErr != nil {
		c.Detail = "VictoriaLogs query failed, absence unproven: " + logsErr.Error()
		return c
	}
	if tracesErr != nil {
		c.Detail = "VictoriaTraces query failed, absence unproven: " + tracesErr.Error()
		return c
	}
	var leaks []string
	for _, label := range []string{LabelPlain, LabelSecretMarker} {
		if n := logs.Hits[label]; n > 0 {
			leaks = append(leaks, fmt.Sprintf("%s in %d log line(s), first at %s", label, n, logs.Where[label]))
		}
		if n := traces.Hits[label]; n > 0 {
			leaks = append(leaks, fmt.Sprintf("%s in %d span(s), first at %s", label, n, traces.Where[label]))
		}
	}
	switch {
	case len(leaks) > 0:
		c.Detail = "content leaked: " + strings.Join(leaks, "; ")
	case traces.Truncated:
		c.Detail = "a trace search hit its limit, absence unproven"
	case logs.Hits[LabelControlRun]+logs.Hits[LabelControlTrace] == 0:
		c.Detail = fmt.Sprintf("positive control: neither the run id nor its trace id appears in %d log lines, so the logs query may be blind", logs.Scanned)
	case traces.Hits[LabelControlRun] == 0:
		c.Detail = fmt.Sprintf("positive control: the run id appears in none of %d spans, so the trace search may be blind", traces.Scanned)
	default:
		c.Pass = true
		c.Detail = fmt.Sprintf("marker absent from %d log lines and %d spans", logs.Scanned, traces.Scanned)
	}
	return c
}
