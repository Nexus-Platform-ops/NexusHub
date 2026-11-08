// OpenTelemetry tracing for the API: OTLP/http to a collector when
// OTEL_EXPORTER_OTLP_ENDPOINT is set, stdout when OTEL_TRACES_EXPORTER is
// "stdout", and a no-op otherwise. Postgres and Redis are traced through
// hand-rolled pgx/redis adapters in this package (pgx.QueryTracer and the
// go-redis Hook interface) so the repo depends only on the OTel core, not
// the contrib instrumentation tree.
package observability

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
)

// TracingConfig selects the exporter. Endpoint set → OTLP/http; Exporter
// "stdout" → stdout; anything else → disabled (no-op provider).
type TracingConfig struct {
	Exporter string // "", "stdout", or any other value with Endpoint set
	Endpoint string // OTEL_EXPORTER_OTLP_ENDPOINT
	Insecure bool   // plain http for local collectors (OTEL_EXPORTER_OTLP_INSECURE=1)
}

// TracingConfigFromEnv reads the standard OTEL variables.
func TracingConfigFromEnv() TracingConfig {
	return TracingConfig{
		Exporter: os.Getenv("OTEL_TRACES_EXPORTER"),
		Endpoint: os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		Insecure: os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") == "1",
	}
}

// InitTracing installs the global tracer provider and propagator. The
// returned shutdown flushes and must be called on exit; it is non-nil even
// when tracing is disabled.
func InitTracing(ctx context.Context, service, env string, cfg TracingConfig) (func(context.Context) error, error) {
	if cfg.Endpoint == "" && !strings.EqualFold(cfg.Exporter, "stdout") {
		otel.SetTracerProvider(trace.NewNoopTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	// Schemaless on purpose: pinning a semconv schema URL here conflicts with
	// the SDK default resource's schema during Merge.
	res, err := resource.Merge(resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceName(service),
			semconv.DeploymentEnvironmentName(env),
		))
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	var exp sdktrace.SpanExporter
	if cfg.Endpoint != "" {
		opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.Endpoint)}
		if cfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		exp, err = otlptracehttp.New(ctx, opts...)
	} else {
		exp, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	}
	if err != nil {
		return nil, fmt.Errorf("otel exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exp),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	return tp.Shutdown, nil
}

// Tracer returns the package tracer for hand-rolled spans.
func Tracer() trace.Tracer { return otel.Tracer("nexus/backend") }
