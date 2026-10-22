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
	"math"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// findField returns the field with the given key from a zap field slice.
func findField(fields []zap.Field, key string) (zap.Field, bool) {
	for _, f := range fields {
		if f.Key == key {
			return f, true
		}
	}
	return zap.Field{}, false
}

// requireField fails the test if the key is missing and returns the field.
func requireField(t *testing.T, fields []zap.Field, key string) zap.Field {
	t.Helper()
	f, ok := findField(fields, key)
	if !ok {
		t.Fatalf("expected field %q in fields %+v", key, fields)
	}
	return f
}

// requireNoField fails the test if the key is present.
func requireNoField(t *testing.T, fields []zap.Field, key string) {
	t.Helper()
	if _, ok := findField(fields, key); ok {
		t.Fatalf("did not expect field %q in fields %+v", key, fields)
	}
}

// requireStringField asserts a string-typed field with the given value exists.
func requireStringField(t *testing.T, fields []zap.Field, key, want string) {
	t.Helper()
	f := requireField(t, fields, key)
	if f.Type != zapcore.StringType {
		t.Errorf("field %q: got type %v, want StringType", key, f.Type)
	}
	if f.String != want {
		t.Errorf("field %q: got %q, want %q", key, f.String, want)
	}
}

// requireInt64Field asserts an int/int64-typed field with the given value exists.
func requireInt64Field(t *testing.T, fields []zap.Field, key string, want int64) {
	t.Helper()
	f := requireField(t, fields, key)
	if f.Type != zapcore.Int64Type {
		t.Errorf("field %q: got type %v, want Int64Type", key, f.Type)
	}
	if f.Integer != want {
		t.Errorf("field %q: got %d, want %d", key, f.Integer, want)
	}
}

// requireBoolField asserts a bool-typed field with the given value exists.
func requireBoolField(t *testing.T, fields []zap.Field, key string, want bool) {
	t.Helper()
	f := requireField(t, fields, key)
	if f.Type != zapcore.BoolType {
		t.Errorf("field %q: got type %v, want BoolType", key, f.Type)
	}
	if got, wantInt := f.Integer, boolToInt64(want); got != wantInt {
		t.Errorf("field %q: got %d, want %d", key, got, wantInt)
	}
}

// requireFloat64Field asserts a float64-typed field with the given value exists.
func requireFloat64Field(t *testing.T, fields []zap.Field, key string, want float64) {
	t.Helper()
	f := requireField(t, fields, key)
	if f.Type != zapcore.Float64Type {
		t.Errorf("field %q: got type %v, want Float64Type", key, f.Type)
	}
	if got := math.Float64frombits(uint64(f.Integer)); got != want {
		t.Errorf("field %q: got %v, want %v", key, got, want)
	}
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// newObservedLogger returns a Logger backed by an in-memory observer core,
// so tests can assert on emitted entries without touching stdout.
func newObservedLogger() (*Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return &Logger{Logger: zap.New(core), componentName: "test-component"}, logs
}

// lastEntryFields returns the fields of the most recent entry, failing if none.
func lastEntryFields(t *testing.T, logs *observer.ObservedLogs) []zap.Field {
	t.Helper()
	entries := logs.TakeAll()
	if len(entries) == 0 {
		t.Fatal("expected at least one log entry, got none")
	}
	return entries[len(entries)-1].Context
}
