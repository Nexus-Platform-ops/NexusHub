package observability

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// installTestTracer swaps the global provider for an in-memory recorder and
// returns the recorder plus a restore function.
func installTestTracer(t *testing.T) (*tracetest.SpanRecorder, func()) {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	return sr, func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	}
}

func TestPGXTracerSpanShape(t *testing.T) {
	sr, restore := installTestTracer(t)
	defer restore()

	tr := PGXTracer{Pool: "social"}
	ctx := tr.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{
		SQL: "SELECT id FROM social.users WHERE id=$1",
	})
	tr.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{
		CommandTag: pgconn.NewCommandTag("SELECT 1"),
	})

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	span := spans[0]
	if span.Name() != "db.social.select" {
		t.Errorf("name = %q", span.Name())
	}
	attrs := map[string]string{}
	for _, a := range span.Attributes() {
		attrs[string(a.Key)] = a.Value.AsString()
	}
	if attrs["db.system"] != "postgresql" || attrs["db.name"] != "social" ||
		attrs["db.operation"] != "select" || attrs["db.sql.table"] != "social.users" {
		t.Errorf("attrs = %v", attrs)
	}
	if attrs["db.statement"] == "" {
		t.Error("missing statement attr")
	}
}

func TestPGXTracerErrorStatus(t *testing.T) {
	sr, restore := installTestTracer(t)
	defer restore()

	tr := PGXTracer{Pool: "dating"}
	ctx := tr.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
	boom := errors.New("boom")
	tr.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: boom})

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	if spans[0].Status().Code != codes.Error {
		t.Errorf("status = %v, want error", spans[0].Status())
	}
}

func TestRedisHookSpan(t *testing.T) {
	sr, restore := installTestTracer(t)
	defer restore()

	cmd := redis.NewStringCmd(context.Background(), "get", "k")
	hookFn := RedisHook{}.ProcessHook(func(ctx context.Context, _ redis.Cmder) error {
		time.Sleep(time.Millisecond)
		return nil
	})
	if err := hookFn(context.Background(), cmd); err != nil {
		t.Fatalf("hook: %v", err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	if spans[0].Name() != "redis.get" {
		t.Errorf("name = %q", spans[0].Name())
	}
}

// redis.Nil is the "key absent" sentinel, not an error: the span must stay OK.
func TestRedisHookNilIsNotError(t *testing.T) {
	sr, restore := installTestTracer(t)
	defer restore()

	cmd := redis.NewStringCmd(context.Background(), "get", "k")
	hookFn := RedisHook{}.ProcessHook(func(context.Context, redis.Cmder) error {
		return redis.Nil
	})
	if err := hookFn(context.Background(), cmd); err != redis.Nil {
		t.Fatalf("err = %v", err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	if spans[0].Status().Code != codes.Ok {
		t.Errorf("status = %v, want ok", spans[0].Status())
	}
}

func TestRedisHookPipelineSpan(t *testing.T) {
	sr, restore := installTestTracer(t)
	defer restore()

	cmds := []redis.Cmder{redis.NewStringCmd(context.Background(), "set", "a", 1)}
	hookFn := RedisHook{}.ProcessPipelineHook(func(context.Context, []redis.Cmder) error {
		return nil
	})
	if err := hookFn(context.Background(), cmds); err != nil {
		t.Fatalf("hook: %v", err)
	}

	spans := sr.Ended()
	if len(spans) != 1 || spans[0].Name() != "redis.pipeline" {
		t.Fatalf("spans = %v", spans)
	}
}

func TestPGXTracerString(t *testing.T) {
	if got := (PGXTracer{Pool: "dating"}).String(); got != "PGXTracer(dating)" {
		t.Errorf("String() = %q", got)
	}
}

func TestRedisHookDialHookPassthrough(t *testing.T) {
	called := false
	next := func(context.Context, string, string) (net.Conn, error) {
		called = true
		return nil, nil
	}
	hook := RedisHook{}.DialHook(next)
	if _, err := hook(context.Background(), "tcp", "127.0.0.1:6379"); err != nil {
		t.Fatalf("dial hook: %v", err)
	}
	if !called {
		t.Fatal("dial hook did not call through")
	}
}

func TestRedisHookPipelineError(t *testing.T) {
	sr, restore := installTestTracer(t)
	defer restore()

	boom := errors.New("boom")
	hookFn := RedisHook{}.ProcessPipelineHook(func(context.Context, []redis.Cmder) error {
		return boom
	})
	cmds := []redis.Cmder{redis.NewStringCmd(context.Background(), "get", "k")}
	if err := hookFn(context.Background(), cmds); err != boom {
		t.Fatalf("err = %v", err)
	}
	spans := sr.Ended()
	if len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Fatalf("spans = %v", spans)
	}
}
