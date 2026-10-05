// Package telemetry sends superwitness's own traces and metrics (service.name
// "superwitness"). Exporters are bounded and drop on overflow; nothing here blocks a request.
package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

func Setup(ctx context.Context, endpoint, version string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	endpoint = strings.TrimRight(endpoint, "/")
	res := resource.NewSchemaless(attribute.String("service.name", "superwitness"), attribute.String("service.version", version))

	texp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint+"/v1/traces"))
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res),
		sdktrace.WithBatcher(texp, sdktrace.WithMaxQueueSize(2048))) // drops when full
	mexp, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint+"/v1/metrics"))
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(mexp, sdkmetric.WithInterval(30*time.Second))))
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	return func(ctx context.Context) error { return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx)) }, nil
}

// SourceObserver counts every source fetch by source and status. The labels are exactly
// those two: never a run, attempt, station or fingerprint id.
func SourceObserver(logger *slog.Logger) func(source.Name, source.SourceStatus) {
	counter, err := otel.Meter("superwitness").Int64Counter("superwitness.source.fetches",
		metric.WithDescription("source fetches by source and status"))
	if err != nil {
		logger.Error("cannot create source counter", "err", err)
	}
	return func(n source.Name, st source.SourceStatus) {
		if counter != nil {
			counter.Add(context.Background(), 1, metric.WithAttributes(
				attribute.String("source", string(n)), attribute.String("status", string(st))))
		}
		if st == source.StatusUnauthorized {
			logger.Warn("source refused superwitness's credential", "source", string(n))
		}
	}
}
