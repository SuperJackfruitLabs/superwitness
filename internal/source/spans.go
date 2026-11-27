package source

import "time"

// The span and log attribute keys SuperWitness queries and reads, each one a key the suite's
// telemetry conventions name (treaty's schemas/telemetry/suite.v1.json, vendored in
// internal/treaty): a run's spans and log lines are found by AttrRunID, and its spans counted
// per attempt by AttrAttemptID. A test holds every key here, and every key the trace and log
// queries use, to the conventions.
const (
	AttrRunID     = "run.id"
	AttrAttemptID = "attempt.id"
)

// QueriedAttributes is every attribute key SuperWitness queries or reads.
var QueriedAttributes = []string{AttrRunID, AttrAttemptID}

// TraceIDsOf returns the distinct trace ids in spans, ordered by each trace's earliest span.
func TraceIDsOf(spans []Span) []string {
	first := map[string]time.Time{}
	var order []string
	for _, s := range spans {
		t, seen := first[s.TraceID]
		if !seen {
			order = append(order, s.TraceID)
			first[s.TraceID] = s.Start
			continue
		}
		if s.Start.Before(t) {
			first[s.TraceID] = s.Start
		}
	}
	// insertion sort: there are a handful of traces per run
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && first[order[j]].Before(first[order[j-1]]); j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	if order == nil {
		return []string{}
	}
	return order
}
