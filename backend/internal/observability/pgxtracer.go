package observability

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// PGXTracer implements pgx.QueryTracer with OTel spans: one span per query
// carrying the statement, operation, and table, so traces show the full
// gateway → handler → database path.
type PGXTracer struct {
	// Pool names the traced pool ("social" | "dating") — the database role
	// boundary is a first-class dimension in traces because the isolation
	// model (docs/ARCHITECTURE.md) makes it security-relevant.
	Pool string
}

// Compile-time check against the pgx v5 tracer interface.
var _ pgx.QueryTracer = PGXTracer{}

type ctxKey int

const queryStartKey ctxKey = iota

type queryStartData struct {
	span trace.Span
}

func (t PGXTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	spanCtx, span := Tracer().Start(ctx, "db."+t.Pool+"."+queryOp(data.SQL),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.name", t.Pool),
			attribute.String("db.statement", data.SQL),
			attribute.String("db.operation", queryOp(data.SQL)),
			attribute.String("db.sql.table", firstTable(data.SQL)),
		))
	_ = spanCtx
	return context.WithValue(ctx, queryStartKey, queryStartData{span: span})
}

func (t PGXTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span := ctx.Value(queryStartKey).(queryStartData).span
	if data.Err != nil {
		span.RecordError(data.Err)
		span.SetStatus(codes.Error, data.Err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.SetAttributes(attribute.Int64("db.rows_affected", data.CommandTag.RowsAffected()))
	span.End()
}

// queryOp extracts the leading SQL keyword (SELECT/INSERT/...) lowercased.
func queryOp(sql string) string {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return "query"
	}
	for i, r := range sql {
		if r == ' ' || r == '\n' || r == '\t' {
			return strings.ToLower(sql[:i])
		}
	}
	return strings.ToLower(sql)
}

// firstTable best-effort extracts the primary table for the span attribute.
func firstTable(sql string) string {
	fields := strings.Fields(sql)
	for i, f := range fields {
		switch strings.ToUpper(f) {
		case "FROM", "INTO", "UPDATE", "JOIN":
			if i+1 < len(fields) {
				return strings.Trim(fields[i+1], `",();`)
			}
		}
	}
	return ""
}

func (t PGXTracer) String() string { return fmt.Sprintf("PGXTracer(%s)", t.Pool) }
