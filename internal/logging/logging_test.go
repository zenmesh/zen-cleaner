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
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestGetLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		config  string
		want    zapcore.Level
		wantEnv string
	}{
		{name: "empty config and env defaults to info", env: "", config: "", want: zapcore.InfoLevel},
		{name: "debug", env: "", config: "debug", want: zapcore.DebugLevel},
		{name: "DEBUG uppercase", env: "", config: "DEBUG", want: zapcore.DebugLevel},
		{name: "info", env: "", config: "info", want: zapcore.InfoLevel},
		{name: "warn", env: "", config: "warn", want: zapcore.WarnLevel},
		{name: "warning", env: "", config: "warning", want: zapcore.WarnLevel},
		{name: "WARNING uppercase", env: "", config: "WARNING", want: zapcore.WarnLevel},
		{name: "error", env: "", config: "error", want: zapcore.ErrorLevel},
		{name: "unknown string falls back to info", env: "", config: "verbose", want: zapcore.InfoLevel},
		{name: "env used when config empty", env: "debug", config: "", want: zapcore.DebugLevel, wantEnv: "debug"},
		{name: "config wins over env", env: "debug", config: "error", want: zapcore.ErrorLevel, wantEnv: "debug"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", tt.env)
			if got := getLogLevel(tt.config); got != tt.want {
				t.Errorf("getLogLevel(%q) with LOG_LEVEL=%q = %v, want %v",
					tt.config, tt.env, got, tt.want)
			}
		})
	}
}

func TestIsDevelopment(t *testing.T) {
	tests := []struct {
		name       string
		logLevel   string
		develop    string
		env        string
		wantResult bool
	}{
		{name: "nothing set", logLevel: "", develop: "", env: "", wantResult: false},
		{name: "LOG_LEVEL=debug", logLevel: "debug", develop: "", env: "", wantResult: true},
		{name: "LOG_LEVEL=info is not dev", logLevel: "info", develop: "", env: "", wantResult: false},
		{name: "DEVELOPMENT=true", logLevel: "", develop: "true", env: "", wantResult: true},
		{name: "DEVELOPMENT=false is not dev", logLevel: "", develop: "false", env: "", wantResult: false},
		{name: "DEVELOPMENT=1 is not dev", logLevel: "", develop: "1", env: "", wantResult: false},
		{name: "ENV=development", logLevel: "", develop: "", env: "development", wantResult: true},
		{name: "ENV=prod is not dev", logLevel: "", develop: "", env: "prod", wantResult: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", tt.logLevel)
			t.Setenv("DEVELOPMENT", tt.develop)
			t.Setenv("ENV", tt.env)
			if got := isDevelopment(); got != tt.wantResult {
				t.Errorf("isDevelopment() = %v, want %v", got, tt.wantResult)
			}
		})
	}
}

func TestNewLoggerWithConfigDefaultsComponentName(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("DEVELOPMENT", "")
	t.Setenv("ENV", "")
	l := NewLoggerWithConfig(LoggerConfig{})
	if l.componentName != "unknown" {
		t.Errorf("componentName = %q, want unknown", l.componentName)
	}
}

func TestNewLoggerWritesComponentField(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("DEVELOPMENT", "")
	t.Setenv("ENV", "")
	l := NewLoggerWithConfig(LoggerConfig{ComponentName: "cleaner"})
	l.Info("hello")
	// NewLoggerWithConfig writes to stdout; here we only assert no panic and
	// that the wrapped logger carries the component context.
	if l.componentName != "cleaner" {
		t.Errorf("componentName = %q, want cleaner", l.componentName)
	}
}

func TestLoggerLevelMethods(t *testing.T) {
	logger, logs := newObservedLogger()
	logger.Debug("d", zap.Int("i", 1))
	logger.Info("i")
	logger.Warn("w")
	logger.Error(nil, "e")

	entries := logs.TakeAll()
	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(entries))
	}
	wantLevels := []zapcore.Level{zapcore.DebugLevel, zapcore.InfoLevel,
		zapcore.WarnLevel, zapcore.ErrorLevel}
	for i, want := range wantLevels {
		if entries[i].Level != want {
			t.Errorf("entry %d level = %v, want %v", i, entries[i].Level, want)
		}
	}
}

func TestLoggerErrorAddsCategory(t *testing.T) {
	t.Run("categorizes automatically", func(t *testing.T) {
		logger, logs := newObservedLogger()
		logger.Error(errors.New("resource not found"), "load failed")
		fields := lastEntryFields(t, logs)
		requireStringField(t, fields, "error_category", "not_found")
		if f, ok := findField(fields, "error"); !ok || f.Type != zapcore.ErrorType {
			t.Error("expected error field of ErrorType")
		}
	})

	t.Run("respects caller-provided category", func(t *testing.T) {
		t.Setenv("DEVELOPMENT", "true")
		logger, logs := newObservedLogger()
		logger.Error(errors.New("resource not found"), "load failed",
			ErrorCategoryField(ErrorCategoryConflict))
		fields := lastEntryFields(t, logs)
		requireStringField(t, fields, "error_category", "conflict")
		requireNoField(t, fields, "error_stack")
	})

	t.Run("adds stack trace in development", func(t *testing.T) {
		t.Setenv("DEVELOPMENT", "true")
		logger, logs := newObservedLogger()
		logger.Error(errors.New("resource not found"), "load failed")
		fields := lastEntryFields(t, logs)
		requireStringField(t, fields, "error_category", "not_found")
		if f, ok := findField(fields, "error_stack"); !ok || f.String == "" {
			t.Error("expected non-empty error_stack field in development")
		}
	})

	t.Run("no stack trace outside development", func(t *testing.T) {
		t.Setenv("LOG_LEVEL", "info")
		t.Setenv("DEVELOPMENT", "false")
		t.Setenv("ENV", "production")
		logger, logs := newObservedLogger()
		logger.Error(errors.New("boom"), "failed")
		requireNoField(t, lastEntryFields(t, logs), "error_stack")
	})
}

func TestLoggerWithMethods(t *testing.T) {
	t.Run("WithComponent", func(t *testing.T) {
		logger, logs := newObservedLogger()
		child := logger.WithComponent("other")
		child.Info("msg")
		if child.componentName != "other" {
			t.Errorf("componentName = %q, want other", child.componentName)
		}
		requireStringField(t, lastEntryFields(t, logs), "component", "other")
	})

	t.Run("WithField", func(t *testing.T) {
		logger, logs := newObservedLogger()
		logger.WithField("k", "v").Info("msg")
		requireStringField(t, lastEntryFields(t, logs), "k", "v")
	})

	t.Run("WithFields", func(t *testing.T) {
		logger, logs := newObservedLogger()
		child := logger.WithFields(map[string]interface{}{"a": "1", "b": "2"})
		child.Info("msg")
		fields := lastEntryFields(t, logs)
		requireStringField(t, fields, "a", "1")
		requireStringField(t, fields, "b", "2")
	})

	t.Run("WithFields preserves component name", func(t *testing.T) {
		logger, _ := newObservedLogger()
		if got := logger.WithFields(nil).componentName; got != "test-component" {
			t.Errorf("componentName = %q, want test-component", got)
		}
	})
}

func TestLoggerWithContext(t *testing.T) {
	t.Run("nil context returns same logger", func(t *testing.T) {
		logger, _ := newObservedLogger()
		var nilCtx context.Context
		if got := logger.WithContext(nilCtx); got != logger {
			t.Error("WithContext(nil) should return the same logger")
		}
	})

	t.Run("empty context returns same logger", func(t *testing.T) {
		logger, _ := newObservedLogger()
		if got := logger.WithContext(context.Background()); got != logger {
			t.Error("WithContext(empty) should return the same logger")
		}
	})

	t.Run("extracts and masks context values", func(t *testing.T) {
		logger, logs := newObservedLogger()
		ctx := WithResourceID(WithClusterID(WithTenantID(WithUserID(WithSpanID(
			WithTraceID(WithRequestID(context.Background(),
				"req-1"), "tr-1"), "sp-1"),
			"550e8400-e29b-41d4-a716-446655440000"),
			"12345678-90ab-41d4-a716-446655440000"),
			"cluster-1"),
			"res-1")
		child := logger.WithContext(ctx)
		if child == logger {
			t.Fatal("expected a new logger with context fields")
		}
		child.Info("msg")
		fields := lastEntryFields(t, logs)
		requireStringField(t, fields, "request_id", "req-1")
		requireStringField(t, fields, "trace_id", "tr-1")
		requireStringField(t, fields, "span_id", "sp-1")
		// Tenant and user ids must be masked by default.
		requireStringField(t, fields, "tenant_id", "12345678...")
		requireStringField(t, fields, "user_id", "550e8400...")
		requireStringField(t, fields, "cluster_id", "cluster-1")
		requireStringField(t, fields, "resource_id", "res-1")
	})
}

func TestLoggerContextAwareMethods(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-7")

	t.Run("InfoC carries request id", func(t *testing.T) {
		logger, logs := newObservedLogger()
		logger.InfoC(ctx, "msg")
		requireStringField(t, lastEntryFields(t, logs), "request_id", "req-7")
	})

	t.Run("DebugC carries request id", func(t *testing.T) {
		logger, logs := newObservedLogger()
		logger.DebugC(ctx, "msg")
		requireStringField(t, lastEntryFields(t, logs), "request_id", "req-7")
	})

	t.Run("WarnC carries request id", func(t *testing.T) {
		logger, logs := newObservedLogger()
		logger.WarnC(ctx, "msg")
		requireStringField(t, lastEntryFields(t, logs), "request_id", "req-7")
	})

	t.Run("ErrorC carries request id and category", func(t *testing.T) {
		logger, logs := newObservedLogger()
		logger.ErrorC(ctx, errors.New("not found"), "msg")
		fields := lastEntryFields(t, logs)
		requireStringField(t, fields, "request_id", "req-7")
		requireStringField(t, fields, "error_category", "not_found")
	})
}
