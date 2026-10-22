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
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestPerformanceFieldHelpers(t *testing.T) {
	tests := []struct {
		name  string
		field zap.Field
		key   string
		value int64
	}{
		{"RequestSizeBytes", RequestSizeBytes(11), "request_size_bytes", 11},
		{"ResponseSizeBytes", ResponseSizeBytes(22), "response_size_bytes", 22},
		{"RowsAffected", RowsAffected(2), "rows_affected", 2},
		{"MessageCount", MessageCount(4), "message_count", 4},
		{"FileSizeBytes", FileSizeBytes(88), "file_size_bytes", 88},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireInt64Field(t, []zap.Field{tt.field}, tt.key, tt.value)
		})
	}

	t.Run("string helpers", func(t *testing.T) {
		requireStringField(t, []zap.Field{QueueName("jobs")}, "queue_name", "jobs")
		requireStringField(t, []zap.Field{ExternalService("api")}, "external_service", "api")
		requireStringField(t, []zap.Field{ExternalEndpoint("/v1")}, "external_endpoint", "/v1")
		requireStringField(t, []zap.Field{FilePath("/tmp/f")}, "file_path", "/tmp/f")
	})
	t.Run("ThroughputBytesPerSecond", func(t *testing.T) {
		requireFloat64Field(t, []zap.Field{ThroughputBytesPerSecond(12.5)},
			"throughput_bytes_per_second", 12.5)
	})
	t.Run("ConcurrentOperations", func(t *testing.T) {
		requireInt64Field(t, []zap.Field{ConcurrentOperations(6)}, "concurrent_operations", 6)
	})
}

func TestPerformanceLoggerLogRequestProcessed(t *testing.T) {
	logger, logs := newObservedLogger()
	pl := NewPerformanceLogger(logger)
	pl.LogRequestProcessed(context.Background(), "sync", 1500*time.Millisecond, 200, 10, 20,
		zap.String("extra", "e"))

	fields := lastEntryFields(t, logs)
	requireStringField(t, fields, "operation", "sync")
	requireInt64Field(t, fields, "latency_ms", 1500)
	requireInt64Field(t, fields, "http_status", 200)
	requireInt64Field(t, fields, "request_size_bytes", 10)
	requireInt64Field(t, fields, "response_size_bytes", 20)
	requireStringField(t, fields, "extra", "e")
}

func TestPerformanceLoggerLogDBCall(t *testing.T) {
	t.Run("success logs at info with sanitized query", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		pl.LogDBCall(context.Background(), "select", "SELECT * FROM t WHERE k = 'v'",
			5*time.Millisecond, 1, nil)

		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.InfoLevel {
			t.Fatalf("expected one info entry, got %+v", entries)
		}
		fields := entries[0].Context
		requireStringField(t, fields, "operation", "select")
		requireStringField(t, fields, "db_query", "SELECT * FROM t WHERE k = '?'")
		requireInt64Field(t, fields, "db_duration_ms", 5)
		requireInt64Field(t, fields, "rows_affected", 1)
	})

	t.Run("failure logs at error", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		pl.LogDBCall(context.Background(), "insert", "INSERT INTO t", time.Millisecond, 0,
			errors.New("transaction aborted"))

		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.ErrorLevel {
			t.Fatalf("expected one error entry, got %+v", entries)
		}
		requireStringField(t, entries[0].Context, "error_category", "database")
	})
}

func TestPerformanceLoggerLogCacheOperation(t *testing.T) {
	t.Run("hit records cache_status hit", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		pl.LogCacheOperation(context.Background(), "get", "k1", true, time.Millisecond, nil)

		fields := lastEntryFields(t, logs)
		requireBoolField(t, fields, "cache_hit", true)
		requireStringField(t, fields, "cache_status", "hit")
	})

	t.Run("miss records cache_status miss", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		pl.LogCacheOperation(context.Background(), "get", "k2", false, time.Millisecond, nil)

		fields := lastEntryFields(t, logs)
		requireBoolField(t, fields, "cache_hit", false)
		requireStringField(t, fields, "cache_status", "miss")
	})

	t.Run("error skips cache_status and logs at error", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		pl.LogCacheOperation(context.Background(), "get", "k3", false, time.Millisecond,
			errors.New("cache unavailable"))

		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.ErrorLevel {
			t.Fatalf("expected one error entry, got %+v", entries)
		}
		requireNoField(t, entries[0].Context, "cache_status")
	})
}

func TestPerformanceLoggerLogExternalAPICall(t *testing.T) {
	t.Run("success logs at info", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		pl.LogExternalAPICall(context.Background(), "registry", "/v1/pods", "GET", 200,
			300*time.Millisecond, 5, 100, nil)

		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.InfoLevel {
			t.Fatalf("expected one info entry, got %+v", entries)
		}
		fields := entries[0].Context
		requireStringField(t, fields, "operation", "external_api_call")
		requireStringField(t, fields, "external_service", "registry")
		requireStringField(t, fields, "external_endpoint", "/v1/pods")
		requireStringField(t, fields, "http_method", "GET")
		requireInt64Field(t, fields, "http_status", 200)
		requireInt64Field(t, fields, "latency_ms", 300)
	})

	t.Run("failure logs at error", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		pl.LogExternalAPICall(context.Background(), "registry", "/v1", "POST", 500, 0, 0, 0,
			errors.New("connection reset"))

		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.ErrorLevel {
			t.Fatalf("expected one error entry, got %+v", entries)
		}
		requireStringField(t, entries[0].Context, "error_category", "network")
	})
}

func TestPerformanceLoggerLogMessageQueueOperation(t *testing.T) {
	t.Run("success logs at info", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		pl.LogMessageQueueOperation(context.Background(), "publish", "events", 3,
			10*time.Millisecond, nil)

		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.InfoLevel {
			t.Fatalf("expected one info entry, got %+v", entries)
		}
		fields := entries[0].Context
		requireStringField(t, fields, "queue_name", "events")
		requireInt64Field(t, fields, "message_count", 3)
	})

	t.Run("failure logs at error", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		pl.LogMessageQueueOperation(context.Background(), "consume", "events", 0, 0,
			errors.New("broker down"))

		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.ErrorLevel {
			t.Fatalf("expected one error entry, got %+v", entries)
		}
	})
}

func TestPerformanceLoggerLogFileOperation(t *testing.T) {
	t.Run("success logs at info", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		pl.LogFileOperation(context.Background(), "read", "/data/f", 1024,
			20*time.Millisecond, nil)

		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.InfoLevel {
			t.Fatalf("expected one info entry, got %+v", entries)
		}
		fields := entries[0].Context
		requireStringField(t, fields, "file_path", "/data/f")
		requireInt64Field(t, fields, "file_size_bytes", 1024)
	})

	t.Run("failure logs at error", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		pl.LogFileOperation(context.Background(), "write", "/data/f", 0, 0,
			errors.New("disk full"))

		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.ErrorLevel {
			t.Fatalf("expected one error entry, got %+v", entries)
		}
	})
}

func TestMeasureDuration(t *testing.T) {
	t.Run("returns fn error and logs at error", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		sentinel := errors.New("boom")

		got := pl.MeasureDuration(context.Background(), "op", func() error { return sentinel })
		if !errors.Is(got, sentinel) {
			t.Fatalf("MeasureDuration = %v, want sentinel", got)
		}
		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.ErrorLevel {
			t.Fatalf("expected one error entry, got %+v", entries)
		}
		requireStringField(t, entries[0].Context, "operation", "op")
	})

	t.Run("nil error logs at info", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)

		if got := pl.MeasureDuration(context.Background(), "op", func() error { return nil }); got != nil {
			t.Fatalf("MeasureDuration = %v, want nil", got)
		}
		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.InfoLevel {
			t.Fatalf("expected one info entry, got %+v", entries)
		}
	})
}

func TestMeasureDurationWithFields(t *testing.T) {
	t.Run("extra fields attached", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		_ = pl.MeasureDurationWithFields(context.Background(), "op",
			[]zap.Field{zap.String("extra", "x")}, func() error { return nil })

		fields := lastEntryFields(t, logs)
		requireStringField(t, fields, "operation", "op")
		requireStringField(t, fields, "extra", "x")
	})

	t.Run("error returned", func(t *testing.T) {
		logger, logs := newObservedLogger()
		pl := NewPerformanceLogger(logger)
		sentinel := errors.New("boom")

		got := pl.MeasureDurationWithFields(context.Background(), "op", nil,
			func() error { return sentinel })
		if !errors.Is(got, sentinel) {
			t.Fatalf("MeasureDurationWithFields = %v, want sentinel", got)
		}
		if entries := logs.TakeAll(); len(entries) != 1 {
			t.Fatalf("got %d entries, want 1", len(entries))
		}
	})
}
