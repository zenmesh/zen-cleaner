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

package errors

import (
	"errors"
	"testing"
)

func TestContextError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *ContextError
		wantMsg string
	}{
		{
			name:    "error with message only",
			err:     New("test_type", "test message"),
			wantMsg: "test message",
		},
		{
			name:    "error with underlying error",
			err:     Wrap(errors.New("underlying error"), "test_type", "wrapped message"),
			wantMsg: "wrapped message: underlying error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.wantMsg {
				t.Errorf("ContextError.Error() = %v, want %v", got, tt.wantMsg)
			}
		})
	}
}

func TestContextError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	err := Wrap(underlying, "test_type", "wrapped message")

	if !errors.Is(err, underlying) {
		t.Errorf("ContextError should wrap underlying: %v", underlying)
	}
}

func TestContextError_WithContext(t *testing.T) {
	err := New("test_type", "test message")
	err = err.WithContext("policy", "test-policy")
	err = err.WithContext("namespace", "default")

	if got := err.GetContext("policy"); got != "test-policy" {
		t.Errorf("GetContext(\"policy\") = %v, want %v", got, "test-policy")
	}
	if got := err.GetContext("namespace"); got != "default" {
		t.Errorf("GetContext(\"namespace\") = %v, want %v", got, "default")
	}
}

func TestWithContext(t *testing.T) {
	underlying := errors.New("underlying error")
	err := WithContext(underlying, "policy", "test-policy")

	var ctxErr *ContextError
	if !errors.As(err, &ctxErr) {
		t.Fatal("WithContext should return a ContextError")
	}

	if got := ctxErr.GetContext("policy"); got != "test-policy" {
		t.Errorf("GetContext(\"policy\") = %v, want %v", got, "test-policy")
	}
}

func TestWithMultipleContext(t *testing.T) {
	underlying := errors.New("underlying error")
	context := map[string]string{
		"policy":    "test-policy",
		"namespace": "default",
		"resource":  "test-resource",
	}
	err := WithMultipleContext(underlying, context)

	var ctxErr *ContextError
	if !errors.As(err, &ctxErr) {
		t.Fatal("WithMultipleContext should return a ContextError")
	}

	for k, v := range context {
		if got := ctxErr.GetContext(k); got != v {
			t.Errorf("GetContext(%q) = %v, want %v", k, got, v)
		}
	}
}

func TestWrapf(t *testing.T) {
	underlying := errors.New("underlying error")
	err := Wrapf(underlying, "test_type", "formatted message: %s", "value")

	if err.Message != "formatted message: value" {
		t.Errorf("Wrapf message = %v, want %v", err.Message, "formatted message: value")
	}
	if !errors.Is(err, underlying) {
		t.Errorf("Wrapf should wrap underlying error: %v", underlying)
	}
}
