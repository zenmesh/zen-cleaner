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
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestCategorizeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		category ErrorCategory
	}{
		{"nil error", nil, ErrorCategoryUnknown},
		{"not found", errors.New("resource not found"), ErrorCategoryNotFound},
		{"does not exist", errors.New("namespace does not exist"), ErrorCategoryNotFound},
		{"already exists", errors.New("resource already exists"), ErrorCategoryConflict},
		{"duplicate", errors.New("duplicate key"), ErrorCategoryConflict},
		{"conflict", errors.New("write conflict"), ErrorCategoryConflict},
		{"unauthorized", errors.New("unauthorized request"), ErrorCategoryAuthentication},
		{"authentication failed", errors.New("authentication failed for user"), ErrorCategoryAuthentication},
		{"invalid token is authentication, not validation", errors.New("invalid token"), ErrorCategoryAuthentication},
		{"forbidden", errors.New("forbidden resource"), ErrorCategoryAuthorization},
		{"permission denied", errors.New("permission denied on pod"), ErrorCategoryAuthorization},
		{"access denied", errors.New("access denied to secret"), ErrorCategoryAuthorization},
		{"invalid", errors.New("invalid input"), ErrorCategoryValidation},
		{"validation", errors.New("validation failed"), ErrorCategoryValidation},
		{"malformed", errors.New("malformed request body"), ErrorCategoryValidation},
		{"timeout", errors.New("operation timeout"), ErrorCategoryTimeout},
		{"deadline exceeded", errors.New("context deadline exceeded"), ErrorCategoryTimeout},
		{"rate limit", errors.New("rate limit exceeded"), ErrorCategoryRateLimit},
		{"too many requests", errors.New("too many requests"), ErrorCategoryRateLimit},
		{"connection refused", errors.New("dial: connection refused"), ErrorCategoryNetwork},
		{"connection reset", errors.New("connection reset by peer"), ErrorCategoryNetwork},
		{"network", errors.New("network unreachable"), ErrorCategoryNetwork},
		{"database", errors.New("database unavailable"), ErrorCategoryDatabase},
		{"sql", errors.New("sql syntax error"), ErrorCategoryDatabase},
		{"transaction", errors.New("transaction aborted"), ErrorCategoryDatabase},
		{"config", errors.New("config missing"), ErrorCategoryConfig},
		{"configuration", errors.New("configuration error"), ErrorCategoryConfig},
		{
			"validation wins over config",
			errors.New("configuration invalid key"),
			ErrorCategoryValidation,
		},
		{"unknown message maps to internal", errors.New("something inexplicable happened"), ErrorCategoryInternal},
		{"case insensitive", errors.New("RESOURCE NOT FOUND"), ErrorCategoryNotFound},
		{
			"not found wins over timeout",
			errors.New("resource not found after timeout"),
			ErrorCategoryNotFound,
		},
		{
			"conflict wins over validation",
			errors.New("duplicate entry, invalid data"),
			ErrorCategoryConflict,
		},
		{
			"validation wins over timeout",
			errors.New("invalid request due to timeout"),
			ErrorCategoryValidation,
		},
		{
			"timeout wins over network",
			errors.New("network timeout"),
			ErrorCategoryTimeout,
		},
		{
			"network wins over database",
			errors.New("database connection refused"),
			ErrorCategoryNetwork,
		},
		{
			"timeout wins over config",
			errors.New("configuration error: timeout"),
			ErrorCategoryTimeout,
		},
		{
			"categorizes wrapped errors by message",
			fmt.Errorf("get user: %w", errors.New("user not found")),
			ErrorCategoryNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CategorizeError(tt.err); got != tt.category {
				t.Errorf("CategorizeError(%v) = %q, want %q", tt.err, got, tt.category)
			}
		})
	}
}

func TestErrorCategoryValues(t *testing.T) {
	tests := []struct {
		cat  ErrorCategory
		want string
	}{
		{ErrorCategoryUnknown, "unknown"},
		{ErrorCategoryValidation, "validation"},
		{ErrorCategoryAuthentication, "authentication"},
		{ErrorCategoryAuthorization, "authorization"},
		{ErrorCategoryNotFound, "not_found"},
		{ErrorCategoryConflict, "conflict"},
		{ErrorCategoryRateLimit, "rate_limit"},
		{ErrorCategoryTimeout, "timeout"},
		{ErrorCategoryNetwork, "network"},
		{ErrorCategoryDatabase, "database"},
		{ErrorCategoryExternal, "external"},
		{ErrorCategoryInternal, "internal"},
		{ErrorCategoryConfig, "config"},
		{ErrorCategoryTemporary, "temporary"},
	}
	for _, tt := range tests {
		if string(tt.cat) != tt.want {
			t.Errorf("category constant = %q, want %q", tt.cat, tt.want)
		}
	}
}

func TestGetStackTraceDevelopmentGated(t *testing.T) {
	t.Run("disabled outside development", func(t *testing.T) {
		t.Setenv("LOG_LEVEL", "info")
		t.Setenv("DEVELOPMENT", "false")
		t.Setenv("ENV", "production")
		if got := GetStackTrace(0); got != "" {
			t.Errorf("GetStackTrace in production = %q, want empty", got)
		}
	})

	t.Run("enabled when LOG_LEVEL=debug", func(t *testing.T) {
		t.Setenv("LOG_LEVEL", "debug")
		got := GetStackTrace(0)
		if got == "" {
			t.Fatal("GetStackTrace in development = empty, want stack frames")
		}
		if !strings.Contains(got, ".go:") {
			t.Errorf("GetStackTrace output %q should contain file:line frames", got)
		}
	})

	t.Run("enabled when DEVELOPMENT=true", func(t *testing.T) {
		t.Setenv("LOG_LEVEL", "")
		t.Setenv("DEVELOPMENT", "true")
		t.Setenv("ENV", "")
		if got := GetStackTrace(0); got == "" {
			t.Error("GetStackTrace with DEVELOPMENT=true = empty, want stack frames")
		}
	})

	t.Run("enabled when ENV=development", func(t *testing.T) {
		t.Setenv("LOG_LEVEL", "")
		t.Setenv("DEVELOPMENT", "")
		t.Setenv("ENV", "development")
		if got := GetStackTrace(0); got == "" {
			t.Error("GetStackTrace with ENV=development = empty, want stack frames")
		}
	})

	t.Run("skip beyond stack depth returns empty", func(t *testing.T) {
		t.Setenv("DEVELOPMENT", "true")
		if got := GetStackTrace(1000); got != "" {
			t.Errorf("GetStackTrace(1000) = %q, want empty", got)
		}
	})
}

func TestExtractErrorContext(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		ctx := ExtractErrorContext(nil, 0)
		if ctx.Category != ErrorCategoryUnknown {
			t.Errorf("Category = %q, want unknown", ctx.Category)
		}
		if ctx.Message != "" {
			t.Errorf("Message = %q, want empty", ctx.Message)
		}
		if ctx.Stack != "" {
			t.Errorf("Stack = %q, want empty outside development", ctx.Stack)
		}
	})

	t.Run("plain error wraps itself", func(t *testing.T) {
		err := errors.New("database down")
		ctx := ExtractErrorContext(err, 0)
		if ctx.Category != ErrorCategoryDatabase {
			t.Errorf("Category = %q, want database", ctx.Category)
		}
		if ctx.Message != "database down" {
			t.Errorf("Message = %q, want %q", ctx.Message, "database down")
		}
		if !errors.Is(ctx.WrappedErr, err) {
			t.Errorf("WrappedErr should be the error itself for non-wrapping errors")
		}
	})

	t.Run("wrapped error unwraps one level", func(t *testing.T) {
		base := errors.New("user not found")
		wrapped := fmt.Errorf("lookup failed: %w", base)
		ctx := ExtractErrorContext(wrapped, 0)
		if !errors.Is(ctx.WrappedErr, base) {
			t.Errorf("WrappedErr = %v, want the wrapped base error", ctx.WrappedErr)
		}
		if ctx.Category != ErrorCategoryNotFound {
			t.Errorf("Category = %q, want not_found", ctx.Category)
		}
	})
}

func TestErrorCategoryField(t *testing.T) {
	f := ErrorCategoryField(ErrorCategoryNotFound)
	if f.Key != "error_category" {
		t.Errorf("Key = %q, want error_category", f.Key)
	}
	if f.Type != zapcore.StringType {
		t.Errorf("Type = %v, want StringType", f.Type)
	}
	if f.String != "not_found" {
		t.Errorf("String = %q, want not_found", f.String)
	}
}

func TestErrorStackField(t *testing.T) {
	t.Run("empty stack returns zap.Skip placeholder", func(t *testing.T) {
		f := ErrorStackField("")
		if f.Type != zapcore.SkipType {
			t.Errorf("empty stack: type = %v, want SkipType", f.Type)
		}
	})

	t.Run("stack skipped outside development", func(t *testing.T) {
		t.Setenv("LOG_LEVEL", "info")
		t.Setenv("DEVELOPMENT", "false")
		t.Setenv("ENV", "production")
		f := ErrorStackField("frame.go:1")
		if f.Type != zapcore.SkipType {
			t.Errorf("ErrorStackField outside development type = %v, want SkipType", f.Type)
		}
	})

	t.Run("stack kept in development", func(t *testing.T) {
		t.Setenv("DEVELOPMENT", "true")
		f := ErrorStackField("frame.go:1")
		if f.Key != "error_stack" || f.String != "frame.go:1" {
			t.Errorf("ErrorStackField = {%q %q}, want error_stack frame.go:1", f.Key, f.String)
		}
	})
}

func TestErrorFields(t *testing.T) {
	t.Run("nil error yields no fields", func(t *testing.T) {
		fields := ErrorFields(nil, "CODE")
		if len(fields) != 0 {
			t.Errorf("ErrorFields(nil) = %+v, want empty", fields)
		}
	})

	t.Run("non-nil error without code", func(t *testing.T) {
		fields := ErrorFields(errors.New("not found"), "")
		requireStringField(t, fields, "error_category", "not_found")
		requireNoField(t, fields, "error_code")
		requireNoField(t, fields, "error_stack")
		errField := requireField(t, fields, "error")
		if errField.Type != zapcore.ErrorType {
			t.Errorf("error field type = %v, want ErrorType", errField.Type)
		}
	})

	t.Run("error code included when provided", func(t *testing.T) {
		fields := ErrorFields(errors.New("not found"), "USER_404")
		requireStringField(t, fields, "error_code", "USER_404")
	})

	t.Run("stack field added in development", func(t *testing.T) {
		t.Setenv("DEVELOPMENT", "true")
		fields := ErrorFields(errors.New("not found"), "")
		if f, ok := findField(fields, "error_stack"); !ok || f.String == "" {
			t.Errorf("expected non-empty error_stack field, got %+v", fields)
		}
	})
}

func TestEnhancedErrorLoggerLogError(t *testing.T) {
	t.Run("nil error logs nothing", func(t *testing.T) {
		logger, logs := newObservedLogger()
		NewEnhancedErrorLogger(logger).LogError(nil, "ignored", "CODE")
		if got := len(logs.TakeAll()); got != 0 {
			t.Errorf("LogError(nil) emitted %d entries, want 0", got)
		}
	})

	t.Run("error logged with category and code", func(t *testing.T) {
		logger, logs := newObservedLogger()
		NewEnhancedErrorLogger(logger).LogError(
			errors.New("resource not found"), "load failed", "RES_404",
			zap.String("extra", "value"),
		)
		fields := lastEntryFields(t, logs)
		requireStringField(t, fields, "error_category", "not_found")
		requireStringField(t, fields, "error_code", "RES_404")
		requireStringField(t, fields, "extra", "value")
	})
}

func TestWrapError(t *testing.T) {
	t.Run("nil error returns nil", func(t *testing.T) {
		if got := WrapError(nil, "ctx"); got != nil {
			t.Errorf("WrapError(nil) = %v, want nil", got)
		}
	})

	t.Run("wraps preserving chain", func(t *testing.T) {
		base := errors.New("boom")
		got := WrapError(base, "context")
		if got == nil || got.Error() != "context: boom" {
			t.Errorf("WrapError = %v, want context: boom", got)
		}
		if !errors.Is(got, base) {
			t.Error("WrapError should preserve errors.Is chain")
		}
	})
}

func TestWrapErrorf(t *testing.T) {
	t.Run("nil error returns nil", func(t *testing.T) {
		if got := WrapErrorf(nil, "ctx %d", 1); got != nil {
			t.Errorf("WrapErrorf(nil) = %v, want nil", got)
		}
	})

	t.Run("wraps with formatted message", func(t *testing.T) {
		base := errors.New("boom")
		got := WrapErrorf(base, "attempt %d", 3)
		if got == nil || got.Error() != "attempt 3: boom" {
			t.Errorf("WrapErrorf = %v, want attempt 3: boom", got)
		}
		if !errors.Is(got, base) {
			t.Error("WrapErrorf should preserve errors.Is chain")
		}
	})
}

func TestErrorWithCode(t *testing.T) {
	t.Run("message without cause", func(t *testing.T) {
		err := NewErrorWithCode("E_TEST", "something broke")
		if got, want := err.Error(), "[E_TEST] something broke"; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
		if got := errors.Unwrap(err); got != nil {
			t.Errorf("Unwrap() = %v, want nil", got)
		}
	})

	t.Run("message with cause", func(t *testing.T) {
		cause := errors.New("disk full")
		err := NewErrorWithCodeAndCause("E_IO", "write failed", cause)
		if got, want := err.Error(), "[E_IO] write failed: disk full"; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
		if !errors.Is(err, cause) {
			t.Error("ErrorWithCode should unwrap to its cause")
		}
	})
}

func TestExtractErrorCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil error", nil, ""},
		{"plain error", errors.New("boom"), ""},
		{"error with code", NewErrorWithCode("E_1", "m"), "E_1"},
		{
			"wrapped error with code",
			WrapError(NewErrorWithCode("E_2", "m"), "ctx"),
			"E_2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractErrorCode(tt.err); got != tt.want {
				t.Errorf("ExtractErrorCode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"nil error", nil, false},
		{"timeout is retryable", errors.New("request timeout"), true},
		{"deadline exceeded is retryable", errors.New("context deadline exceeded"), true},
		{"network is retryable", errors.New("connection reset"), true},
		{"rate limit is retryable", errors.New("too many requests"), true},
		{"not found is not retryable", errors.New("not found"), false},
		{"validation is not retryable", errors.New("invalid input"), false},
		{"unknown is not retryable", errors.New("mystery failure"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryableError(tt.err); got != tt.retryable {
				t.Errorf("IsRetryableError(%v) = %v, want %v", tt.err, got, tt.retryable)
			}
		})
	}
}

func TestIsClientError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		client bool
	}{
		{"nil error", nil, false},
		{"validation", errors.New("invalid input"), true},
		{"authentication", errors.New("unauthorized"), true},
		{"authorization", errors.New("forbidden"), true},
		{"not found", errors.New("resource not found"), true},
		{"conflict", errors.New("already exists"), true},
		{"timeout is not client", errors.New("timeout"), false},
		{"internal is not client", errors.New("database down"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsClientError(tt.err); got != tt.client {
				t.Errorf("IsClientError(%v) = %v, want %v", tt.err, got, tt.client)
			}
		})
	}
}

func TestIsServerError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		server bool
	}{
		{"nil error", nil, false},
		{"internal", errors.New("mystery failure"), true},
		{"database", errors.New("sql error"), true},
		{"config", errors.New("config missing"), true},
		{"not found is not server", errors.New("not found"), false},
		{"validation is not server", errors.New("invalid input"), false},
		{"timeout is not server", errors.New("timeout"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsServerError(tt.err); got != tt.server {
				t.Errorf("IsServerError(%v) = %v, want %v", tt.err, got, tt.server)
			}
		})
	}
}

func TestErrorContextWithZapFields(t *testing.T) {
	ctx := &ErrorContext{}
	if got := ctx.WithZapFields(zap.String("a", "1")); got != ctx {
		t.Fatal("WithZapFields should return the same context pointer")
	}
	ctx.WithZapFields(zap.String("b", "2"))
	if len(ctx.Fields) != 2 {
		t.Fatalf("Fields length = %d, want 2", len(ctx.Fields))
	}
	requireStringField(t, ctx.Fields, "a", "1")
	requireStringField(t, ctx.Fields, "b", "2")
}
