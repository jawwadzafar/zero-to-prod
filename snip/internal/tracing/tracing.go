// Package tracing turns on OpenTelemetry tracing when an OTLP endpoint is
// configured (chapter 13.3). With no endpoint, tracing stays off and every
// span is a cheap no-op.
package tracing

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Setup configures the global tracer provider from the standard OTEL_*
// environment variables, e.g. OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4318.
// It returns a shutdown function that flushes spans still in memory.
func Setup(ctx context.Context, service string) (shutdown func(context.Context) error, enabled bool, err error) {
	// W3C Trace Context: read and pass on the traceparent header, so traces
	// continue across services. Harmless when tracing is off.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	noop := func(context.Context) error { return nil }
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return noop, false, nil
	}
	exporter, err := otlptracehttp.New(ctx) // endpoint, headers, etc. from OTEL_* variables
	if err != nil {
		return noop, false, err
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", service)),
		resource.WithFromEnv(), // OTEL_RESOURCE_ATTRIBUTES, e.g. deployment.environment=staging
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return noop, false, err
	}
	// Spans are batched and sent in the background; the sampler follows
	// OTEL_TRACES_SAMPLER (default: record everything, respecting the caller's decision).
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)
	return tp.Shutdown, true, nil
}
