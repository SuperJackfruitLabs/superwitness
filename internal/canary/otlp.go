package canary

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// OTLPLogs sends flow 8's one synthetic error line to the host's local OpenTelemetry
// collector (local senders use 127.0.0.1:4318). Its service.name is superwitness-canary, so it
// cannot be mistaken for a product's error, and it carries no prompt content.
type OTLPLogs struct {
	URL  string
	HTTP *http.Client
}

func (o OTLPLogs) InjectError(ctx context.Context, runID, traceID, nonce string, at time.Time) error {
	attr := func(k, v string) map[string]any {
		return map[string]any{"key": k, "value": map[string]any{"stringValue": v}}
	}
	rec := map[string]any{
		"timeUnixNano":   strconv.FormatInt(at.UnixNano(), 10),
		"severityNumber": 17,
		"severityText":   "ERROR",
		"body":           map[string]any{"stringValue": "superwitness canary injected error " + nonce + " (synthetic; safe to ignore)"},
		"attributes":     []any{attr("run.id", runID)},
	}
	if traceID != "" {
		rec["traceId"] = traceID
	}
	payload := map[string]any{"resourceLogs": []any{map[string]any{
		"resource": map[string]any{"attributes": []any{attr("service.name", canaryService)}},
		"scopeLogs": []any{map[string]any{
			"scope":      map[string]any{"name": canaryService},
			"logRecords": []any{rec},
		}},
	}}}
	if err := doJSON(ctx, o.HTTP, http.MethodPost, o.URL, "", payload, http.StatusOK, nil); err != nil {
		return fmt.Errorf("OTLP logs: %w", err)
	}
	return nil
}
