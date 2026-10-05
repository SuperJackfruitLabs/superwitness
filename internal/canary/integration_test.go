//go:build integration

package canary

// Victoria engine contract: the canary's TracesClient and LogsClient against real
// VictoriaTraces and VictoriaLogs (the floors test/integration pins), fed through the
// same OTLP exporters. Run: go test -tags integration ./internal/canary/ -run Integration -v

import (
	"bytes"
	"context"
	"crypto/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	itTracesImage = "victoriametrics/victoria-traces:v0.12.0"
	itLogsImage   = "victoriametrics/victoria-logs:v1.53.0"
)

func itStartVictoria(t *testing.T, image, port string) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: image, ExposedPorts: []string{port},
			WaitingFor: wait.ForHTTP("/health").WithPort(port).WithStartupTimeout(90 * time.Second),
		},
		Started: true,
	})
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start %s: %v", image, err)
	}
	ep, err := ctr.PortEndpoint(ctx, port, "http")
	if err != nil {
		t.Fatal(err)
	}
	return ep
}

func itTracer(t *testing.T, vt, service string) *sdktrace.TracerProvider {
	t.Helper()
	exp, err := otlptracehttp.New(context.Background(), otlptracehttp.WithEndpointURL(vt+"/insert/opentelemetry/v1/traces"))
	if err != nil {
		t.Fatal(err)
	}
	return sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", service))))
}

func itEventually(t *testing.T, within time.Duration, what string, cond func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(within)
	last := ""
	for time.Now().Before(deadline) {
		ok, why := cond()
		if ok {
			return
		}
		last = why
		time.Sleep(time.Second)
	}
	t.Fatalf("%s not met within %v: %s", what, within, last)
}

type itSeed struct {
	runID, traceID string
	markers        Markers
	start          time.Time
}

// itSeedTelemetry writes a run as the canary would see it: hub spans carrying run.id, a
// Workers span in its own trace carrying run.id, one span whose name leaked the plain
// marker, a log line linked by run.id and trace context, and one whose message leaked the
// secret marker.
func itSeedTelemetry(t *testing.T, vt, vl string) itSeed {
	t.Helper()
	ctx := context.Background()
	m, err := NewMarkers(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	nonce, _ := newNonce(rand.Reader)
	s := itSeed{runID: "run_swcit" + strings.TrimPrefix(nonce, "swcerr-"), markers: m, start: time.Now().Add(-time.Minute)}

	hub := itTracer(t, vt, hubService)
	tr := hub.Tracer("seed")
	ctx1, dispatch := tr.Start(ctx, "dispatch", trace.WithAttributes(attribute.String("run.id", s.runID),
		attribute.String("board.id", "brd_it"), attribute.String("external.source", "superpipeline")))
	attemptCtx, attempt := tr.Start(ctx1, "attempt", trace.WithAttributes(attribute.String("attempt.id", "attempt_it"),
		attribute.String("run.id", s.runID)))
	_, turn := tr.Start(attemptCtx, "turn", trace.WithAttributes(attribute.String("attempt.id", "attempt_it")))
	_, leak := tr.Start(attemptCtx, "tool_call "+m.Plain) // a harness that put prompt text in a span name
	leak.End()
	turn.End()
	attempt.End()
	dispatch.End()
	if err := hub.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	s.traceID = dispatch.SpanContext().TraceID().String()

	workers := itTracer(t, vt, workersService)
	_, hb := workers.Tracer("seed").Start(ctx, "POST /v1/boards/:id/runs/:runId/heartbeat",
		trace.WithAttributes(attribute.String("run.id", s.runID)))
	hb.End()
	if err := workers.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}

	exp, err := otlploghttp.New(ctx, otlploghttp.WithEndpointURL(vl+"/insert/opentelemetry/v1/logs"))
	if err != nil {
		t.Fatal(err)
	}
	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)),
		sdklog.WithResource(resource.NewSchemaless(attribute.String("service.name", hubService))))
	lg := lp.Logger("seed")
	var linked otellog.Record
	linked.SetTimestamp(time.Now())
	linked.SetSeverity(otellog.SeverityInfo)
	linked.SetSeverityText("INFO")
	linked.SetBody(attribute.StringValue("attempt opened"))
	linked.AddAttributes(attribute.String("run.id", s.runID))
	lg.Emit(attemptCtx, linked)
	var leaked otellog.Record
	leaked.SetTimestamp(time.Now())
	leaked.SetSeverity(otellog.SeverityWarn)
	leaked.SetSeverityText("WARN")
	leaked.SetBody(attribute.StringValue("harness echoed its prompt: use key " + m.Secret + " for the release note"))
	lg.Emit(ctx, leaked)
	if err := lp.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIntegrationVictoriaEngines(t *testing.T) {
	vt := itStartVictoria(t, itTracesImage, "10428/tcp")
	vl := itStartVictoria(t, itLogsImage, "9428/tcp")
	seed := itSeedTelemetry(t, vt, vl)
	traces := TracesClient{BaseURL: vt}
	logs := LogsClient{BaseURL: vl}
	ctx := context.Background()
	end := time.Now().Add(time.Minute)

	// Ingestion is asynchronous: wait until both engines answer for the seeded run.
	var traceSpans []Span
	itEventually(t, 60*time.Second, "trace queryable", func() (bool, string) {
		var err error
		traceSpans, err = traces.Trace(ctx, seed.traceID)
		if err != nil {
			return false, err.Error()
		}
		return len(traceSpans) == 4, "spans: " + itOps(traceSpans)
	})
	needles := merge(seed.markers.Needles(), Controls(seed.runID, []string{seed.traceID}))
	var logRes ScanResult
	itEventually(t, 60*time.Second, "logs queryable", func() (bool, string) {
		var err error
		logRes, err = logs.Scan(ctx, seed.start, end, needles)
		if err != nil {
			return false, err.Error()
		}
		return logRes.Scanned >= 2, "scanned " + strconv.Itoa(logRes.Scanned)
	})

	t.Run("Trace converts Jaeger spans", func(t *testing.T) {
		if c := CheckHubSpansJoined(RunDoc{Sources: map[string]string{"traces": "ok"},
			Trace:    DocTrace{TraceIDs: []string{seed.traceID}, Status: "joined"},
			Attempts: []DocAttempt{{ID: "attempt_it", SpanCountRaw: []byte("2")}}}, seed.runID, traceSpans); !c.Pass {
			t.Errorf("flow 3 on VictoriaTraces' spans: %s (spans %s)", c.Detail, itOps(traceSpans))
		}
		for _, s := range traceSpans {
			if s.TraceID != seed.traceID || s.Service != hubService {
				t.Errorf("span %+v", s)
			}
		}
	})

	t.Run("FindByTag finds the Workers span in its own trace", func(t *testing.T) {
		var found []Span
		var query string
		itEventually(t, 30*time.Second, "tag search", func() (bool, string) {
			var err error
			found, query, err = traces.FindByTag(ctx, workersService, "run.id", seed.runID, seed.start, end)
			if err != nil {
				return false, err.Error()
			}
			return len(found) > 0, "none found by " + query
		})
		if c := CheckWorkersSpan(seed.runID, traceSpans, found, nil); !c.Pass || !strings.Contains(c.Detail, "in the joined trace: 0; by tag search: 1") {
			t.Errorf("flow 5: pass=%v %s", c.Pass, c.Detail)
		}
		if got, q, err := traces.FindByTag(ctx, workersService, "run.id", "run_nobody", seed.start, end); err != nil || len(got) != 0 {
			t.Errorf("tag search for an absent run: %d span(s), err %v (%s)", len(got), err, q)
		}
	})

	traceRes, err := traces.Scan(ctx, seed.start, end, needles)
	if err != nil {
		t.Fatalf("traces scan: %v", err)
	}

	t.Run("Scans count controls and needles and never echo a needle", func(t *testing.T) {
		if traceRes.Hits[LabelControlRun] < 3 { // dispatch, attempt, heartbeat
			t.Errorf("traces control_run hits = %d, want >= 3", traceRes.Hits[LabelControlRun])
		}
		if traceRes.Hits[LabelPlain] != 1 || traceRes.Hits[LabelSecretMarker] != 0 {
			t.Errorf("traces hits = %v", traceRes.Hits)
		}
		if w := traceRes.Where[LabelPlain]; !strings.Contains(w, "key=operation") || !strings.Contains(w, "<"+LabelPlain+">") {
			t.Errorf("traces where = %q", w)
		}
		if logRes.Hits[LabelControlRun] != 1 || logRes.Hits[LabelControlTrace] != 1 {
			t.Errorf("logs control hits = %v; want the linked line by run.id and by trace_id", logRes.Hits)
		}
		if logRes.Hits[LabelSecretMarker] != 1 || logRes.Hits[LabelPlain] != 0 {
			t.Errorf("logs hits = %v", logRes.Hits)
		}
		if w := logRes.Where[LabelSecretMarker]; !strings.Contains(w, "fields=_msg") {
			t.Errorf("logs where = %q, want the needle located in _msg", w)
		}
		for _, n := range needles {
			for label, w := range traceRes.Where {
				if strings.Contains(w, n) {
					t.Errorf("traces Where[%s] carries a needle: %q", label, w)
				}
			}
			for label, w := range logRes.Where {
				if strings.Contains(w, n) {
					t.Errorf("logs Where[%s] carries a needle: %q", label, w)
				}
			}
		}
	})

	t.Run("CheckMarkerAbsent", func(t *testing.T) {
		c := CheckMarkerAbsent(logRes, nil, traceRes, nil)
		if c.Pass || !strings.HasPrefix(c.Detail, "content leaked: ") ||
			!strings.Contains(c.Detail, LabelPlain+" in 1 span(s)") || !strings.Contains(c.Detail, LabelSecretMarker+" in 1 log line(s)") {
			t.Errorf("planted markers: pass=%v %s", c.Pass, c.Detail)
		}
		if strings.Contains(c.Detail, seed.markers.Plain) || strings.Contains(c.Detail, seed.markers.Secret) {
			t.Errorf("detail carries a marker: %s", c.Detail)
		}

		// Fresh markers nobody planted, same run: absent, and the controls make that believable.
		fresh, _ := NewMarkers(rand.Reader)
		clean := merge(fresh.Needles(), Controls(seed.runID, []string{seed.traceID}))
		cl, lerr := logs.Scan(ctx, seed.start, end, clean)
		ct, terr := traces.Scan(ctx, seed.start, end, clean)
		if c := CheckMarkerAbsent(cl, lerr, ct, terr); !c.Pass {
			t.Errorf("unplanted markers: %s", c.Detail)
		}

		// A window holding nothing: the scans succeed, find no control, and absence is not believed.
		blindStart, blindEnd := seed.start.Add(-48*time.Hour), seed.start.Add(-47*time.Hour)
		bl, lerr := logs.Scan(ctx, blindStart, blindEnd, clean)
		bt, terr := traces.Scan(ctx, blindStart, blindEnd, clean)
		if lerr != nil || terr != nil {
			t.Fatalf("blind scans: logs %v, traces %v", lerr, terr)
		}
		if bl.Scanned != 0 || bt.Scanned != 0 {
			t.Errorf("empty window scanned logs %d, spans %d", bl.Scanned, bt.Scanned)
		}
		if c := CheckMarkerAbsent(bl, nil, bt, nil); c.Pass || !strings.HasPrefix(c.Detail, "positive control: neither the run id nor its trace id appears in 0 log lines") {
			t.Errorf("blind logs: pass=%v %s", c.Pass, c.Detail)
		}
		// Logs see the run but traces are blind: the traces control fails on its own.
		if c := CheckMarkerAbsent(cl, nil, bt, nil); c.Pass || !strings.HasPrefix(c.Detail, "positive control: the run id appears in none of 0 spans") {
			t.Errorf("blind traces: pass=%v %s", c.Pass, c.Detail)
		}
	})
}

// Does VictoriaTraces' Jaeger services list name a service whose only spans are older than
// an hour? TracesClient.Scan enumerates services from that list, so a service missing from
// it would be a blind spot for flow 9.
func TestIntegrationServicesListsOldOnlyService(t *testing.T) {
	vt := itStartVictoria(t, itTracesImage, "10428/tcp")
	ctx := context.Background()
	old := time.Now().Add(-3 * time.Hour)
	tp := itTracer(t, vt, "swcit-old-only")
	_, sp := tp.Tracer("seed").Start(ctx, "old work", trace.WithTimestamp(old),
		trace.WithAttributes(attribute.String("run.id", "run_old")))
	sp.End(trace.WithTimestamp(old.Add(time.Second)))
	if err := tp.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	traceID := sp.SpanContext().TraceID().String()
	traces := TracesClient{BaseURL: vt}

	// The span is stored: fetch it by id.
	itEventually(t, 60*time.Second, "old trace queryable by id", func() (bool, string) {
		spans, err := traces.Trace(ctx, traceID)
		if err != nil {
			return false, err.Error()
		}
		return len(spans) == 1, itOps(spans)
	})
	raw, err := traces.get(ctx, "/select/jaeger/api/services", nil)
	if err != nil {
		t.Fatal(err)
	}
	listed := bytes.Contains(raw, []byte(`"swcit-old-only"`))
	t.Logf("services list after ingesting a 3h-old span: %s (old-only service listed: %v)", raw, listed)
	if !listed {
		t.Errorf("VictoriaTraces does not list a service whose only spans are 3h old: Scan would not search it")
	}
	res, err := traces.Scan(ctx, old.Add(-time.Minute), old.Add(time.Minute), map[string]string{LabelControlRun: "run_old"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Hits[LabelControlRun] != 1 {
		t.Errorf("Scan over the old window: %+v", res)
	}
}

func itOps(spans []Span) string {
	ops := make([]string, 0, len(spans))
	for _, s := range spans {
		ops = append(ops, s.Service+"/"+s.Operation)
	}
	return strings.Join(ops, ",")
}
