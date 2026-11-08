package observability

import (
	"context"
	"testing"
)

func TestTracingConfigFromEnv(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "1")
	cfg := TracingConfigFromEnv()
	if cfg.Exporter != "otlp" || cfg.Endpoint != "localhost:4318" || !cfg.Insecure {
		t.Errorf("cfg = %+v", cfg)
	}
}

// InitTracing with no endpoint and no stdout exporter must install a no-op
// provider and still return a usable shutdown — the process must boot fine
// with tracing simply off.
func TestInitTracingDisabled(t *testing.T) {
	shutdown, err := InitTracing(context.Background(), "svc", "test", TracingConfig{})
	if err != nil {
		t.Fatalf("InitTracing: %v", err)
	}
	if shutdown == nil {
		t.Fatal("nil shutdown")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

// stdout exporter path: a real provider is installed and shuts down cleanly.
func TestInitTracingStdout(t *testing.T) {
	shutdown, err := InitTracing(context.Background(), "svc", "test", TracingConfig{Exporter: "stdout"})
	if err != nil {
		t.Fatalf("InitTracing: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestTracerHelperNonNil(t *testing.T) {
	if Tracer() == nil {
		t.Fatal("nil tracer")
	}
}

// InitTracing with an OTLP endpoint installs a real batching provider. The
// exporter connects lazily, so an unreachable collector endpoint still
// initialises — spans buffer until the first failed flush, and shutdown
// flushes without blocking the caller.
func TestInitTracingOTLPEndpoint(t *testing.T) {
	shutdown, err := InitTracing(context.Background(), "svc", "test", TracingConfig{
		Endpoint: "localhost:4318",
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("InitTracing: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}
