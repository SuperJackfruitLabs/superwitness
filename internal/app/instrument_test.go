package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// The Host header is client-controlled and arrives before auth, so it must never become a
// metric attribute value; server.address and server.port come from SW_PUBLIC_URL instead.
func TestHostHeaderIsNotAMetricLabel(t *testing.T) {
	for _, tc := range []struct{ publicURL, wantAddr string }{
		{"https://witness.example.com", "witness.example.com"},
		{"http://127.0.0.1:8790", "127.0.0.1"},
	} {
		t.Run(tc.publicURL, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			h, err := instrument(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }),
				tc.publicURL, otelhttp.WithMeterProvider(mp))
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest("GET", "/health", nil)
			req.Host = "attacker-chosen.invalid:31337"
			h.ServeHTTP(httptest.NewRecorder(), req)

			var rm metricdata.ResourceMetrics
			if err := reader.Collect(context.Background(), &rm); err != nil {
				t.Fatal(err)
			}
			var sets []attribute.Set
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					switch d := m.Data.(type) {
					case metricdata.Histogram[float64]:
						for _, p := range d.DataPoints {
							sets = append(sets, p.Attributes)
						}
					case metricdata.Histogram[int64]:
						for _, p := range d.DataPoints {
							sets = append(sets, p.Attributes)
						}
					case metricdata.Sum[int64]:
						for _, p := range d.DataPoints {
							sets = append(sets, p.Attributes)
						}
					}
				}
			}
			if len(sets) == 0 {
				t.Fatal("no metric data points recorded")
			}
			for _, set := range sets {
				for _, kv := range set.ToSlice() {
					if v := kv.Value.Emit(); strings.Contains(v, "attacker-chosen") || strings.Contains(v, "31337") {
						t.Errorf("metric attribute %s = %q came from the Host header", kv.Key, v)
					}
				}
				if v, ok := set.Value("server.address"); !ok || v.AsString() != tc.wantAddr {
					t.Errorf("server.address = %v (present %v), want %s", v.Emit(), ok, tc.wantAddr)
				}
			}
		})
	}
}

func TestServerName(t *testing.T) {
	for in, want := range map[string]string{
		"https://witness.example.com": "witness.example.com:443",
		"http://witness.example.com":  "witness.example.com:80",
		"http://127.0.0.1:8790":       "127.0.0.1:8790",
		"http://:8790":                ":8790",
	} {
		if got, err := serverName(in); err != nil || got != want {
			t.Errorf("serverName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := serverName("not a url"); err == nil {
		t.Error("serverName accepted a relative URL")
	}
}
