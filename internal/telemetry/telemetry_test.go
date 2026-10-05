package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

func TestSetupWithoutEndpointIsNoop(t *testing.T) {
	shutdown, err := Setup(context.Background(), "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSourceObserverCountsAndWarns(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	var logs bytes.Buffer
	observe := SourceObserver(slog.New(slog.NewJSONHandler(&logs, nil)))
	observe(source.Traces, source.StatusOK)
	observe(source.Traces, source.StatusOK)
	observe(source.AgentPod, source.StatusUnauthorized)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "superwitness.source.fetches" {
				continue
			}
			for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
				src, _ := dp.Attributes.Value(attribute.Key("source"))
				st, _ := dp.Attributes.Value(attribute.Key("status"))
				got[src.AsString()+"/"+st.AsString()] = dp.Value
				if dp.Attributes.Len() != 2 {
					t.Errorf("labels = %v; only source and status are allowed", dp.Attributes.ToSlice())
				}
			}
		}
	}
	if got["traces/ok"] != 2 || got["agentpod/unauthorized"] != 1 {
		t.Errorf("counts = %v", got)
	}
	if !strings.Contains(logs.String(), `"source":"agentpod"`) {
		t.Errorf("no warning logged for unauthorized: %s", logs.String())
	}
}
