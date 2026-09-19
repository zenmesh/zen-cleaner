package errors

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestClassifyError_Nil(t *testing.T) {
	if ClassifyError(nil) != nil {
		t.Error("expected nil for nil error")
	}
}

func TestClassifyError_TransientHTTP(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want ErrorClass
		code int
	}{
		{"429 rate limit", "API error: status 429: too many requests", Transient, 429},
		{"502 bad gateway", "API error: status 502: upstream error", Transient, 502},
		{"503 unavailable", "API error: status 503: service unavailable", Transient, 503},
		{"504 gateway timeout", "API error: status 504: gateway timeout", Transient, 504},
		{"500 internal", "API error: status 500: internal server error", Transient, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ce := ClassifyError(fmt.Errorf("%s", tt.err))
			if ce.Class != tt.want {
				t.Errorf("ClassifyError(%q).Class = %v, want %v", tt.err, ce.Class, tt.want)
			}
			if ce.StatusCode != tt.code {
				t.Errorf("ClassifyError(%q).StatusCode = %d, want %d", tt.err, ce.StatusCode, tt.code)
			}
		})
	}
}

func TestClassifyError_PermanentHTTP(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want ErrorClass
		code int
	}{
		{"401 unauthorized", "API error: status 401: unauthorized", Permanent, 401},
		{"403 forbidden", "API error: status 403: forbidden", Permanent, 403},
		{"400 bad request", "API error: status 400: bad request", Permanent, 400},
		{"422 unprocessable", "API error: status 422: unprocessable entity", Permanent, 422},
		{"404 not found", "API error: status 404: not found", Permanent, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ce := ClassifyError(fmt.Errorf("%s", tt.err))
			if ce.Class != tt.want {
				t.Errorf("ClassifyError(%q).Class = %v, want %v", tt.err, ce.Class, tt.want)
			}
			if ce.StatusCode != tt.code {
				t.Errorf("ClassifyError(%q).StatusCode = %d, want %d", tt.err, ce.StatusCode, tt.code)
			}
		})
	}
}

func TestClassifyError_ResourceExhausted(t *testing.T) {
	tests := []struct {
		name string
		err  string
	}{
		{"token budget", "token budget exhausted"},
		{"budget exceeded", "budget exceeded: limit is 15000"},
		{"context length", "context_length_exceeded"},
		{"llama.cpp context overflow", "status 400: request (3338 tokens) exceeds the available context size (3328 tokens)"},
		{"max context", "maximum context length exceeded"},
		{"rate limit", "rate limit exceeded"},
		{"quota", "quota exceeded for model X"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ce := ClassifyError(fmt.Errorf("%s", tt.err))
			if ce.Class != ResourceExhausted {
				t.Errorf("ClassifyError(%q).Class = %v, want %v", tt.err, ce.Class, ResourceExhausted)
			}
		})
	}
}

func TestClassifyError_NetworkErrors(t *testing.T) {
	msgTests := []string{
		"connection refused",
		"connection reset by peer",
		"broken pipe",
		"i/o timeout",
		"no such host",
	}
	for _, msg := range msgTests {
		t.Run(msg, func(t *testing.T) {
			ce := ClassifyError(fmt.Errorf("request failed: %s", msg))
			if ce.Class != Transient {
				t.Errorf("expected transient for %q, got %v", msg, ce.Class)
			}
		})
	}
}

func TestClassifyError_NetOpError(t *testing.T) {
	connRefused := &net.OpError{Op: "dial", Net: "tcp", Err: fmt.Errorf("connection refused")}
	wrapped := fmt.Errorf("request failed: %w", connRefused)
	ce := ClassifyError(wrapped)
	if ce.Class != Transient {
		t.Errorf("expected transient for net.OpError, got %v", ce.Class)
	}
}

func TestClassifyError_NetTimeout(t *testing.T) {
	timeoutErr := &net.OpError{Op: "dial", Net: "tcp", Err: timeoutError{}}
	wrapped := fmt.Errorf("request failed: %w", timeoutErr)
	ce := ClassifyError(wrapped)
	if ce.Class != Transient {
		t.Errorf("expected transient for timeout, got %v", ce.Class)
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestClassifyError_UnknownIsTransient(t *testing.T) {
	ce := ClassifyError(fmt.Errorf("something weird happened"))
	if ce.Class != Transient {
		t.Errorf("expected transient for unknown error, got %v", ce.Class)
	}
}

func TestClassifyError_UnwrapsClassified(t *testing.T) {
	inner := &ClassifiedError{
		Err:        fmt.Errorf("auth failed"),
		Class:      Permanent,
		StatusCode: 401,
	}
	wrapped := fmt.Errorf("outer: %w", inner)
	ce := ClassifyError(wrapped)
	if ce != inner {
		t.Error("expected same ClassifiedError pointer after unwrap")
	}
}

func TestClassifyError_UnwrapsMulti(t *testing.T) {
	inner := &ClassifiedError{
		Err:        fmt.Errorf("rate limited"),
		Class:      Transient,
		StatusCode: 429,
	}
	wrapped := fmt.Errorf("layer1: %w", fmt.Errorf("layer2: %w", inner))
	ce := ClassifyError(wrapped)
	if ce != inner {
		t.Error("expected same ClassifiedError after multi-unwrap")
	}
}

func TestClassifyError_CustomConfig(t *testing.T) {
	cfg := ClassifyConfig{
		ExhaustionPatterns: []string{"custom limit hit", "insufficient funds"},
	}
	ce := ClassifyErrorWithConfig(fmt.Errorf("custom limit hit for account X"), cfg)
	if ce.Class != ResourceExhausted {
		t.Errorf("expected resource_exhausted, got %v", ce.Class)
	}

	// Default patterns should not match.
	ce2 := ClassifyErrorWithConfig(fmt.Errorf("token budget exhausted"), cfg)
	if ce2.Class != Transient {
		t.Errorf("expected transient (default pattern not in custom config), got %v", ce2.Class)
	}
}

func TestExtractHTTPStatus(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"status 429: too many", 429},
		{"http 500 internal error", 500},
		{"no status here", 0},
		{"status abc invalid", 0},
		{"status 99 bad range", 0},
		{"status 600 too high", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractHTTPStatus(tt.input)
			if got != tt.want {
				t.Errorf("extractHTTPStatus(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsTransient(t *testing.T) {
	if !IsTransient(fmt.Errorf("status 429: too many")) {
		t.Error("expected transient for 429")
	}
	if IsTransient(fmt.Errorf("status 401: auth")) {
		t.Error("expected not transient for 401")
	}
	if IsTransient(fmt.Errorf("token budget exhausted")) {
		t.Error("expected not transient for budget")
	}
}

func TestIsPermanent(t *testing.T) {
	if !IsPermanent(fmt.Errorf("status 403: forbidden")) {
		t.Error("expected permanent for 403")
	}
	if IsPermanent(fmt.Errorf("status 503: unavailable")) {
		t.Error("expected not permanent for 503")
	}
}

func TestIsResourceExhausted(t *testing.T) {
	if !IsResourceExhausted(fmt.Errorf("token budget exhausted")) {
		t.Error("expected resource exhausted")
	}
	if IsResourceExhausted(fmt.Errorf("status 429")) {
		t.Error("expected not resource exhausted for 429")
	}
}

func TestRetryableHTTPStatus(t *testing.T) {
	retryable := []int{429, 502, 503, 504, 500}
	notRetryable := []int{200, 400, 401, 403, 404, 422}
	for _, code := range retryable {
		if !RetryableHTTPStatus(code) {
			t.Errorf("expected %d to be retryable", code)
		}
	}
	for _, code := range notRetryable {
		if RetryableHTTPStatus(code) {
			t.Errorf("expected %d to not be retryable", code)
		}
	}
}

func TestClassifiedError_Error(t *testing.T) {
	// With status code
	ce := &ClassifiedError{
		Err:        fmt.Errorf("too many requests"),
		Class:      Transient,
		StatusCode: 429,
	}
	msg := ce.Error()
	want := "[transient] status 429: too many requests"
	if msg != want {
		t.Errorf("got %q, want %q", msg, want)
	}

	// Without status code
	ce2 := &ClassifiedError{
		Err:   fmt.Errorf("connection refused"),
		Class: Transient,
	}
	msg2 := ce2.Error()
	want2 := "[transient] connection refused"
	if msg2 != want2 {
		t.Errorf("got %q, want %q", msg2, want2)
	}
}

func TestErrorClass_String(t *testing.T) {
	tests := []struct {
		class ErrorClass
		want  string
	}{
		{Transient, "transient"},
		{Permanent, "permanent"},
		{ResourceExhausted, "resource_exhausted"},
		{ErrorClass(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.class.String(); got != tt.want {
			t.Errorf("ErrorClass(%d).String() = %q, want %q", tt.class, got, tt.want)
		}
	}
}

func TestClassifiedError_ErrorsIs(t *testing.T) {
	inner := fmt.Errorf("base error")
	ce := &ClassifiedError{Err: inner, Class: Permanent, StatusCode: 401}
	if !errors.Is(ce, inner) {
		t.Error("errors.Is should unwrap to inner error")
	}
}
