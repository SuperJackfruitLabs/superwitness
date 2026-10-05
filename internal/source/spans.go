package source

import "time"

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
