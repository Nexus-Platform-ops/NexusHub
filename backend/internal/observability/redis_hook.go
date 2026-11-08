package observability

import (
	"context"

	"github.com/redis/go-redis/v9"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// RedisHook traces every Redis command with a client span, so a whisper
// write or OTP check appears inside the request trace alongside the DB
// spans. It implements the go-redis Hook interface directly (no contrib
// dependency).
type RedisHook struct{}

var _ redis.Hook = RedisHook{}

func (RedisHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (RedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		_, span := Tracer().Start(ctx, "redis."+cmd.Name(),
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attribute.String("db.system", "redis")))
		err := next(ctx, cmd)
		if err != nil && err != redis.Nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
		return err
	}
}

func (RedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		_, span := Tracer().Start(ctx, "redis.pipeline",
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attribute.Int("db.redis.commands", len(cmds))))
		err := next(ctx, cmds)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
		return err
	}
}
