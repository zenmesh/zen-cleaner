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

func TestMonitoringFieldHelpers(t *testing.T) {
	tests := []struct {
		name  string
		field zap.Field
		key   string
		value string
	}{
		{"MetricName", MetricName("cpu"), "metric_name", "cpu"},
		{"MetricUnit", MetricUnit("seconds"), "metric_unit", "seconds"},
		{"HealthCheckName", HealthCheckName("db"), "health_check_name", "db"},
		{"EventType", EventType("metric"), "event_type", "metric"},
		{"Severity", Severity("critical"), "severity", "critical"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireStringField(t, []zap.Field{tt.field}, tt.key, tt.value)
		})
	}

	t.Run("MetricValue", func(t *testing.T) {
		requireFloat64Field(t, []zap.Field{MetricValue(0.75)}, "metric_value", 0.75)
	})
	t.Run("HealthCheckHealthy", func(t *testing.T) {
		requireBoolField(t, []zap.Field{HealthCheckHealthy(true)}, "health_check_healthy", true)
	})
	t.Run("AlertFlag", func(t *testing.T) {
		requireBoolField(t, []zap.Field{AlertFlag(false)}, "alert", false)
	})
}

func TestMonitoringLoggerLogMetric(t *testing.T) {
	logger, logs := newObservedLogger()
	ml := NewMonitoringLogger(logger)
	ml.LogMetric(context.Background(), "queue_depth", 12.5, "items", zap.String("queue", "jobs"))

	fields := lastEntryFields(t, logs)
	requireStringField(t, fields, "metric_name", "queue_depth")
	requireFloat64Field(t, fields, "metric_value", 12.5)
	requireStringField(t, fields, "metric_unit", "items")
	requireStringField(t, fields, "event_type", "metric")
	requireStringField(t, fields, "queue", "jobs")
}

func TestMonitoringLoggerLogCounter(t *testing.T) {
	logger, logs := newObservedLogger()
	ml := NewMonitoringLogger(logger)
	ml.LogCounter(context.Background(), "requests_total", 3)

	fields := lastEntryFields(t, logs)
	requireStringField(t, fields, "counter_name", "requests_total")
	requireInt64Field(t, fields, "counter_increment", 3)
	requireStringField(t, fields, "event_type", "counter")
}

func TestMonitoringLoggerLogHistogram(t *testing.T) {
	logger, logs := newObservedLogger()
	ml := NewMonitoringLogger(logger)
	ml.LogHistogram(context.Background(), "latency", 0.95, "le_100")

	fields := lastEntryFields(t, logs)
	requireStringField(t, fields, "histogram_name", "latency")
	requireFloat64Field(t, fields, "histogram_value", 0.95)
	requireStringField(t, fields, "histogram_bucket", "le_100")
	requireStringField(t, fields, "event_type", "histogram")
}

func TestMonitoringLoggerLogGauge(t *testing.T) {
	logger, logs := newObservedLogger()
	ml := NewMonitoringLogger(logger)
	ml.LogGauge(context.Background(), "connections", 42)

	fields := lastEntryFields(t, logs)
	requireStringField(t, fields, "gauge_name", "connections")
	requireFloat64Field(t, fields, "gauge_value", 42)
	requireStringField(t, fields, "event_type", "gauge")
}

func TestMonitoringLoggerLogHealthCheck(t *testing.T) {
	t.Run("healthy logs at info", func(t *testing.T) {
		logger, logs := newObservedLogger()
		ml := NewMonitoringLogger(logger)
		ml.LogHealthCheck(context.Background(), "db", true, 250*time.Millisecond, nil)

		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.InfoLevel {
			t.Fatalf("expected one info entry, got %+v", entries)
		}
		fields := entries[0].Context
		requireStringField(t, fields, "health_check_name", "db")
		requireBoolField(t, fields, "health_check_healthy", true)
		requireInt64Field(t, fields, "latency_ms", 250)
		requireNoField(t, fields, "health_check_error")
	})

	t.Run("failure logs at error with reason", func(t *testing.T) {
		logger, logs := newObservedLogger()
		ml := NewMonitoringLogger(logger)
		ml.LogHealthCheck(context.Background(), "db", false, time.Second,
			errors.New("connection refused"))

		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.ErrorLevel {
			t.Fatalf("expected one error entry, got %+v", entries)
		}
		fields := entries[0].Context
		requireBoolField(t, fields, "health_check_healthy", false)
		requireStringField(t, fields, "health_check_error", "connection refused")
	})
}

func TestMonitoringLoggerLogCriticalEvent(t *testing.T) {
	logger, logs := newObservedLogger()
	ml := NewMonitoringLogger(logger)
	ml.LogCriticalEvent(context.Background(), "deadlock", "critical", Severity("page"))

	entries := logs.TakeAll()
	if len(entries) != 1 || entries[0].Level != zapcore.ErrorLevel {
		t.Fatalf("expected one error entry, got %+v", entries)
	}
	fields := entries[0].Context
	requireStringField(t, fields, "critical_event_name", "deadlock")
	requireStringField(t, fields, "critical_event_severity", "critical")
	requireStringField(t, fields, "event_type", "critical_event")
	requireBoolField(t, fields, "alert", true)
	requireStringField(t, fields, "severity", "page")
}
