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
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestStringFieldHelpers(t *testing.T) {
	tests := []struct {
		name  string
		field zap.Field
		key   string
		value string
	}{
		{"RequestID", RequestID("req-1"), "request_id", "req-1"},
		{"TraceID", TraceID("tr-1"), "trace_id", "tr-1"},
		{"SpanID", SpanID("sp-1"), "span_id", "sp-1"},
		{"ClusterID", ClusterID("c-1"), "cluster_id", "c-1"},
		{"ResourceID", ResourceID("r-1"), "resource_id", "r-1"},
		{"ResourceType", ResourceType("pod"), "resource_type", "pod"},
		{"Operation", Operation("list"), "operation", "list"},
		{"HTTPMethod", HTTPMethod("GET"), "http_method", "GET"},
		{"HTTPPath", HTTPPath("/api/v1"), "http_path", "/api/v1"},
		{"ErrorCode", ErrorCode("E_1"), "error_code", "E_1"},
		{"RemoteAddr", RemoteAddr("10.0.0.1"), "remote_addr", "10.0.0.1"},
		{"UserAgent", UserAgent("ua"), "user_agent", "ua"},
		{"Component", Component("comp"), "component", "comp"},
		{"Namespace", Namespace("default"), "namespace", "default"},
		{"Pod", Pod("pod-1"), "pod", "pod-1"},
		{"Node", Node("node-1"), "node", "node-1"},
		{"Kind", Kind("Pod"), "kind", "Pod"},
		{"Name", Name("my-pod"), "name", "my-pod"},
		{"String", String("custom", "v"), "custom", "v"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireStringField(t, []zap.Field{tt.field}, tt.key, tt.value)
		})
	}
}

func TestIntFieldHelpers(t *testing.T) {
	tests := []struct {
		name  string
		field zap.Field
		key   string
		value int64
	}{
		{"HTTPStatus", HTTPStatus(200), "http_status", 200},
		{"RetryCount", RetryCount(3), "retry_count", 3},
		{"Int", Int("n", 7), "n", 7},
		{"Int64", Int64("n64", 1<<40), "n64", 1 << 40},
		{"Latency", Latency(1500 * time.Millisecond), "latency_ms", 1500},
		{"LatencyMs", LatencyMs(42), "latency_ms", 42},
		{"DBDurationMs", DBDurationMs(9), "db_duration_ms", 9},
		{"MessageCount", MessageCount(5), "message_count", 5},
		{"ConcurrentOperations", ConcurrentOperations(12), "concurrent_operations", 12},
		{"RequestSizeBytes", RequestSizeBytes(100), "request_size_bytes", 100},
		{"ResponseSizeBytes", ResponseSizeBytes(200), "response_size_bytes", 200},
		{"RowsAffected", RowsAffected(1), "rows_affected", 1},
		{"FileSizeBytes", FileSizeBytes(4096), "file_size_bytes", 4096},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireInt64Field(t, []zap.Field{tt.field}, tt.key, tt.value)
		})
	}
}

func TestLatencyTruncatesToMilliseconds(t *testing.T) {
	f := Latency(250 * time.Microsecond)
	if f.Integer != 0 {
		t.Errorf("Latency(250us) = %d ms, want 0", f.Integer)
	}
}

func TestBoolFieldHelpers(t *testing.T) {
	tests := []struct {
		name  string
		field zap.Field
		key   string
		value bool
	}{
		{"CacheHit", CacheHit(true), "cache_hit", true},
		{"Bool", Bool("flag", false), "flag", false},
		{"AlertFlag", AlertFlag(true), "alert", true},
		{"HealthCheckHealthy", HealthCheckHealthy(false), "health_check_healthy", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireBoolField(t, []zap.Field{tt.field}, tt.key, tt.value)
		})
	}
}

func TestFloat64FieldHelpers(t *testing.T) {
	tests := []struct {
		name  string
		field zap.Field
		key   string
		value float64
	}{
		{"Float64", Float64("ratio", 0.5), "ratio", 0.5},
		{"MetricValue", MetricValue(3.14), "metric_value", 3.14},
		{"ThroughputBytesPerSecond", ThroughputBytesPerSecond(1024.5), "throughput_bytes_per_second", 1024.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireFloat64Field(t, []zap.Field{tt.field}, tt.key, tt.value)
		})
	}
}

func TestMaskedIDFields(t *testing.T) {
	tests := []struct {
		name  string
		field zap.Field
		key   string
		value string
	}{
		{"TenantID unmasked", TenantID("tenant-1", false), "tenant_id", "tenant-1"},
		{
			"TenantID masked",
			TenantID("550e8400-e29b-41d4-a716-446655440000", true),
			"tenant_id", "550e8400...",
		},
		{"TenantID masked empty stays empty", TenantID("", true), "tenant_id", ""},
		{"UserID unmasked", UserID("user-1", false), "user_id", "user-1"},
		{
			"UserID masked",
			UserID("12345678-90ab-41d4-a716-446655440000", true),
			"user_id", "12345678...",
		},
		{"UserID masked empty stays empty", UserID("", true), "user_id", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireStringField(t, []zap.Field{tt.field}, tt.key, tt.value)
		})
	}
}

func TestDurationFieldStoresMilliseconds(t *testing.T) {
	f := Duration("elapsed", 2*time.Second)
	if f.Key != "elapsed" || f.Type != zapcore.Int64Type || f.Integer != 2000 {
		t.Errorf("Duration = {key=%q type=%v int=%d}, want elapsed/Int64/2000", f.Key, f.Type, f.Integer)
	}
}

func TestStringsField(t *testing.T) {
	f := Strings("tags", []string{"a", "b"})
	if f.Key != "tags" || f.Type != zapcore.ArrayMarshalerType {
		t.Fatalf("Strings field = {key=%q type=%v}, want tags/ArrayMarshalerType", f.Key, f.Type)
	}
	if _, ok := f.Interface.(zapcore.ArrayMarshaler); !ok {
		t.Errorf("Strings field interface %T does not implement ArrayMarshaler", f.Interface)
	}
}

func TestDBQueryFieldSanitizes(t *testing.T) {
	f := DBQuery("SELECT * FROM users WHERE name = 'bob'")
	requireStringField(t, []zap.Field{f}, "db_query",
		"SELECT * FROM users WHERE name = '?'")
}

func TestDBQueryFieldEmpty(t *testing.T) {
	requireStringField(t, []zap.Field{DBQuery("")}, "db_query", "")
}

func TestErrorField(t *testing.T) {
	err := errors.New("boom")
	f := Error(err)
	if f.Key != "error" || f.Type != zapcore.ErrorType {
		t.Fatalf("Error field = {key=%q type=%v}, want error/ErrorType", f.Key, f.Type)
	}
	if f.Interface.(error).Error() != "boom" {
		t.Errorf("Error field value = %v, want boom", f.Interface)
	}
}

// fieldKey accepts the package alias, proving it is identical to zap.Field.
func fieldKey(f Field) string {
	return f.Key
}

func TestFieldAliasMatchesZapField(t *testing.T) {
	if got := fieldKey(zap.String("k", "v")); got != "k" {
		t.Errorf("Field alias did not behave like zap.Field: key %q", got)
	}
}
