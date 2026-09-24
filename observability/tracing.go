// SUPPORT2-050: real OpenTelemetry span producers for zen-cleaner's
// reconcile path. Uses the otel SDK directly (cleaner does not depend
// on zen-sdk). Three canonical span families bound to real controller
// behavior: reconcile, error reconcile, batch deletion.
// All attributes are bounded: policy name/namespace, resource kind,
// count, and error type. No payload data, no secret material.

package observability

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/zenmesh/zen-cleaner"

// InitTracing wires the global OTel tracer provider. Safe no-op default
// unless OTEL_EXPORTER_OTLP_ENDPOINT is set (then uses OTLP HTTP export).
func InitTracing(ctx context.Context) (func(context.Context) error, error) {
	ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if ep == "" {
		otel.SetTracerProvider(trace.NewNoopTracerProvider())
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(ep), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return func(ctx context.Context) error { return tp.Shutdown(ctx) }, nil
}

// StartReconcileSpan starts a span for one policy reconcile cycle.
func StartReconcileSpan(ctx context.Context, policyNS, policyName string) (context.Context, trace.Span) {
	tr := otel.Tracer(tracerName)
	return tr.Start(ctx, "cleaner.reconcile",
		trace.WithAttributes(
			attribute.String("zen.cleaner.policy.namespace", policyNS),
			attribute.String("zen.cleaner.policy.name", policyName),
		))
}

// EndReconcileSpan records the reconcile outcome.
func EndReconcileSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.Bool("error", true))
	}
	span.End()
}

// StartDeleteBatchSpan starts a span for one batch deletion cycle.
func StartDeleteBatchSpan(ctx context.Context, policyNS, policyName string, batchSize int) (context.Context, trace.Span) {
	tr := otel.Tracer(tracerName)
	return tr.Start(ctx, "cleaner.delete.batch",
		trace.WithAttributes(
			attribute.String("zen.cleaner.policy.namespace", policyNS),
			attribute.String("zen.cleaner.policy.name", policyName),
			attribute.Int("zen.cleaner.batch.size", batchSize),
		))
}

// EndDeleteBatchSpan records the batch outcome.
func EndDeleteBatchSpan(span trace.Span, deleted int, err error) {
	span.SetAttributes(attribute.Int("zen.cleaner.batch.deleted", deleted))
	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.Bool("error", true))
	}
	span.End()
}
