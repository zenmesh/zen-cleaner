package observability

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func testTP(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev); tp.Shutdown(context.Background()) })
	return rec
}

func TestStartReconcileSpan(t *testing.T) {
	rec := testTP(t)
	_, span := StartReconcileSpan(context.Background(), "ns1", "pol1")
	if span == nil {
		t.Fatal("nil span")
	}
	if !span.IsRecording() {
		t.Fatal("not recording")
	}
	EndReconcileSpan(span, nil)
	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d want 1", len(spans))
	}
	if spans[0].Name() != "cleaner.reconcile" {
		t.Errorf("name %q", spans[0].Name())
	}
}

func TestStartReconcileSpan_Error(t *testing.T) {
	rec := testTP(t)
	_, span := StartReconcileSpan(context.Background(), "ns1", "pol1")
	EndReconcileSpan(span, errors.New("boom"))
	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d", len(spans))
	}
	for _, a := range spans[0].Attributes() {
		if a.Key == "error" && a.Value.AsBool() {
			return
		}
	}
	t.Error("error=true not set")
}

func TestStartDeleteBatchSpan(t *testing.T) {
	rec := testTP(t)
	_, span := StartDeleteBatchSpan(context.Background(), "ns1", "pol1", 50)
	if span == nil {
		t.Fatal("nil span")
	}
	EndDeleteBatchSpan(span, 42, nil)
	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d", len(spans))
	}
	if spans[0].Name() != "cleaner.delete.batch" {
		t.Errorf("name %q", spans[0].Name())
	}
	for _, a := range spans[0].Attributes() {
		if a.Key == "zen.cleaner.batch.deleted" && a.Value.AsInt64() == 42 {
			return
		}
	}
	t.Error("deleted count attribute missing")
}

func TestEndDeleteBatchSpan_WithError(t *testing.T) {
	rec := testTP(t)
	_, span := StartDeleteBatchSpan(context.Background(), "ns1", "pol1", 50)
	EndDeleteBatchSpan(span, 3, errors.New("partial failure"))
	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d", len(spans))
	}
	for _, a := range spans[0].Attributes() {
		if a.Key == "error" && a.Value.AsBool() {
			return
		}
	}
	t.Error("error=true not set")
}
