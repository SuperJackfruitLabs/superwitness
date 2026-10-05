package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

// Health is liveness plus a reachability check per dependency. It answers 200
// whenever the process can answer at all, so an uptime monitor separates "superwitness
// is down" from "a source is down".
type Health struct {
	Version string
	Pingers map[string]source.Pinger
	Timeout time.Duration
}

func (h *Health) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d := h.Timeout
	if d <= 0 {
		d = time.Second
	}
	out := map[string]source.SourceStatus{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, p := range h.Pingers {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			done := make(chan source.SourceStatus, 1)
			go func() { done <- p.Ping(ctx) }()
			var st source.SourceStatus
			select {
			case st = <-done:
				if st != source.StatusOK && ctx.Err() != nil {
					st = source.StatusTimeout
				}
			case <-ctx.Done():
				st = source.StatusTimeout
			}
			if st == "" {
				st = source.StatusUnavailable
			}
			mu.Lock()
			out[name] = st
			mu.Unlock()
		})
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": h.Version, "sources": out})
}
