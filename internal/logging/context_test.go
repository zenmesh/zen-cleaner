/*
Copyright 2026 Zen Mesh

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logging

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestContextIDRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		with func(context.Context, string) context.Context
		get  func(context.Context) string
		key  string
	}{
		{"request id", WithRequestID, GetRequestID, "req-1"},
		{"tenant id", WithTenantID, GetTenantID, "tenant-1"},
		{"cluster id", WithClusterID, GetClusterID, "cluster-1"},
		{"user id", WithUserID, GetUserID, "user-1"},
		{"trace id", WithTraceID, GetTraceID, "trace-1"},
		{"span id", WithSpanID, GetSpanID, "span-1"},
		{"resource id", WithResourceID, GetResourceID, "resource-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.with(context.Background(), tt.key)
			if got := tt.get(ctx); got != tt.key {
				t.Errorf("got %q, want %q", got, tt.key)
			}
		})
	}
}

func TestGetIDFromEmptyContext(t *testing.T) {
	tests := []struct {
		name string
		get  func(context.Context) string
	}{
		{"request id", GetRequestID},
		{"tenant id", GetTenantID},
		{"cluster id", GetClusterID},
		{"user id", GetUserID},
		{"trace id", GetTraceID},
		{"span id", GetSpanID},
		{"resource id", GetResourceID},
	}
	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.get(ctx); got != "" {
				t.Errorf("got %q, want empty", got)
			}
		})
	}
}

func TestGetTraceIDPrefersRecordingSpan(t *testing.T) {
	tracer := sdktrace.NewTracerProvider().Tracer("test")
	ctx, span := tracer.Start(context.Background(), "op")
	defer span.End()

	got := GetTraceID(ctx)
	want := span.SpanContext().TraceID().String()
	if got != want {
		t.Errorf("GetTraceID = %q, want span trace id %q", got, want)
	}
	if GetSpanID(ctx) != span.SpanContext().SpanID().String() {
		t.Errorf("GetSpanID = %q, want span id %q", GetSpanID(ctx),
			span.SpanContext().SpanID().String())
	}
}

func TestGetTraceIDFallsBackToCustomKey(t *testing.T) {
	// A non-recording span context falls through to the custom context key.
	ctx := WithTraceID(WithSpanID(context.Background(), "span-custom"), "trace-custom")
	if got := GetTraceID(ctx); got != "trace-custom" {
		t.Errorf("GetTraceID = %q, want trace-custom", got)
	}
	if got := GetSpanID(ctx); got != "span-custom" {
		t.Errorf("GetSpanID = %q, want span-custom", got)
	}
}

func TestExtractTraceContextFromHeaders(t *testing.T) {
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	spanID := "00f067aa0ba902b7"

	t.Run("W3C traceparent", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Traceparent", "00-"+traceID+"-"+spanID+"-01")
		ctx := ExtractTraceContext(req)
		if got := GetTraceID(ctx); got != traceID {
			t.Errorf("trace id = %q, want %q", got, traceID)
		}
		if got := GetSpanID(ctx); got != spanID {
			t.Errorf("span id = %q, want %q", got, spanID)
		}
	})

	t.Run("W3C traceparent wins over X-Trace-ID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Traceparent", "00-"+traceID+"-"+spanID+"-01")
		req.Header.Set("X-Trace-ID", "other-trace")
		ctx := ExtractTraceContext(req)
		if got := GetTraceID(ctx); got != traceID {
			t.Errorf("trace id = %q, want %q from traceparent", got, traceID)
		}
	})

	t.Run("X-Trace-ID fallback", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Trace-ID", "trace-xyz")
		ctx := ExtractTraceContext(req)
		if got := GetTraceID(ctx); got != "trace-xyz" {
			t.Errorf("trace id = %q, want trace-xyz", got)
		}
	})

	t.Run("request id doubles as trace id", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Request-ID", "req-42")
		ctx := ExtractTraceContext(req)
		if got := GetRequestID(ctx); got != "req-42" {
			t.Errorf("request id = %q, want req-42", got)
		}
		if got := GetTraceID(ctx); got != "req-42" {
			t.Errorf("trace id = %q, want req-42", got)
		}
	})

	t.Run("no headers yields plain context", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		ctx := ExtractTraceContext(req)
		if got := GetTraceID(ctx); got != "" {
			t.Errorf("trace id = %q, want empty", got)
		}
		if got := GetRequestID(ctx); got != "" {
			t.Errorf("request id = %q, want empty", got)
		}
	})
}

func TestPropagateTraceHeaders(t *testing.T) {
	t.Run("injects trace headers", func(t *testing.T) {
		ctx := WithRequestID(
			WithSpanID(WithTraceID(context.Background(), "4bf92f3577b34da6a3ce929d0e0e4736"),
				"00f067aa0ba902b7"),
			"req-1",
		)
		req := httptest.NewRequest("GET", "/", nil)
		PropagateTraceHeaders(ctx, req)

		traceparent := req.Header.Get("Traceparent")
		want := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
		if traceparent != want {
			t.Errorf("Traceparent = %q, want %q", traceparent, want)
		}
		if got := req.Header.Get("X-Trace-ID"); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Errorf("X-Trace-ID = %q, want trace id", got)
		}
		if got := req.Header.Get("X-Request-ID"); got != "req-1" {
			t.Errorf("X-Request-ID = %q, want req-1", got)
		}
	})

	t.Run("span id defaults to zeros", func(t *testing.T) {
		ctx := WithTraceID(context.Background(), "4bf92f3577b34da6a3ce929d0e0e4736")
		req := httptest.NewRequest("GET", "/", nil)
		PropagateTraceHeaders(ctx, req)
		traceparent := req.Header.Get("Traceparent")
		if !strings.HasSuffix(traceparent, "-0000000000000000-01") {
			t.Errorf("Traceparent = %q, want zero span id suffix", traceparent)
		}
	})

	t.Run("does not overwrite existing headers", func(t *testing.T) {
		ctx := WithRequestID(WithTraceID(context.Background(), "new-trace"), "new-req")
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Traceparent", "00-existing")
		req.Header.Set("X-Trace-ID", "existing-trace")
		req.Header.Set("X-Request-ID", "existing-req")
		PropagateTraceHeaders(ctx, req)

		if got := req.Header.Get("X-Trace-ID"); got != "existing-trace" {
			t.Errorf("X-Trace-ID = %q, want existing value kept", got)
		}
		if got := req.Header.Get("X-Request-ID"); got != "existing-req" {
			t.Errorf("X-Request-ID = %q, want existing value kept", got)
		}
		if got := req.Header.Get("Traceparent"); got != "00-existing" {
			t.Errorf("Traceparent = %q, want existing value kept", got)
		}
	})

	t.Run("nil arguments are safe", func(t *testing.T) {
		var nilCtx context.Context
		var nilReq *http.Request
		PropagateTraceHeaders(nilCtx, nilReq)
		PropagateTraceHeaders(context.Background(), nilReq)
	})
}

func TestExtractTraceContextSkipsShortTraceparent(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Traceparent", "00-short")
	req.Header.Set("X-Request-ID", "req-9")
	ctx := ExtractTraceContext(req)
	// Malformed traceparent is ignored; request id still becomes the trace id.
	if got := GetTraceID(ctx); got != "req-9" {
		t.Errorf("trace id = %q, want req-9", got)
	}
}
