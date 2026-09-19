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

var (
	testErrorType = "test_error"
	errUnderlying = errors.New("underlying error")
	testNS        = "test-ns"
)

func TestCleanerError_Error(t *testing.T) {
	tests := []struct {
		name       string
		cleanerErr *CleanerError
		wantErr    string
	}{
		{
			name: "error with message only",
			cleanerErr: &CleanerError{
				Type:    testErrorType,
				Message: "test message",
			},
			wantErr: "test message",
		},
		{
			name: "error with underlying error",
			cleanerErr: &CleanerError{
				Type:    testErrorType,
				Message: "test message",
				Err:     errUnderlying,
			},
			wantErr: "test message: underlying error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cleanerErr.Error(); got != tt.wantErr {
				t.Errorf("CleanerError.Error() = %v, want %v", got, tt.wantErr)
			}
		})
	}
}

func TestCleanerError_Unwrap(t *testing.T) {
	cleanerErr := &CleanerError{
		Type:    testErrorType,
		Message: "test message",
		Err:     errUnderlying,
	}

	if got := cleanerErr.Unwrap(); !errors.Is(got, errUnderlying) {
		t.Errorf("CleanerError.Unwrap() = %v, want %v", got, errUnderlying)
	}
}

func TestWithPolicy(t *testing.T) {
	cleanerErr := WithPolicy(errUnderlying, testNS, "test-policy")

	if cleanerErr.GetContext("policy_namespace") != testNS {
		t.Errorf("Expected policy_namespace=%s, got %s", testNS, cleanerErr.GetContext("policy_namespace"))
	}
	if cleanerErr.GetContext("policy_name") != "test-policy" {
		t.Errorf("Expected policy_name=test-policy, got %s", cleanerErr.GetContext("policy_name"))
	}
}

func TestWithPolicy_AlreadyCleanerError(t *testing.T) {
	existingGCErr := New(testErrorType, "existing error")
	cleanerErr := WithPolicy(existingGCErr, testNS, "test-policy")

	if cleanerErr.GetContext("policy_namespace") != testNS {
		t.Errorf("Expected policy_namespace=%s, got %s", testNS, cleanerErr.GetContext("policy_namespace"))
	}
	if cleanerErr.GetContext("policy_name") != "test-policy" {
		t.Errorf("Expected policy_name=test-policy, got %s", cleanerErr.GetContext("policy_name"))
	}
}

func TestWithResource(t *testing.T) {
	cleanerErr := WithResource(errUnderlying, testNS, "test-resource")

	if cleanerErr.GetContext("resource_namespace") != testNS {
		t.Errorf("Expected resource_namespace=%s, got %s", testNS, cleanerErr.GetContext("resource_namespace"))
	}
	if cleanerErr.GetContext("resource_name") != "test-resource" {
		t.Errorf("Expected resource_name=test-resource, got %s", cleanerErr.GetContext("resource_name"))
	}
}

func TestWithResource_AlreadyCleanerError(t *testing.T) {
	existingGCErr := New(testErrorType, "existing error")
	cleanerErr := WithResource(existingGCErr, testNS, "test-resource")

	if cleanerErr.GetContext("resource_namespace") != testNS {
		t.Errorf("Expected resource_namespace=%s, got %s", testNS, cleanerErr.GetContext("resource_namespace"))
	}
	if cleanerErr.GetContext("resource_name") != "test-resource" {
		t.Errorf("Expected resource_name=test-resource, got %s", cleanerErr.GetContext("resource_name"))
	}
}

func TestNew(t *testing.T) {
	cleanerErr := New(testErrorType, "test message")

	if cleanerErr.Type != testErrorType {
		t.Errorf("Expected Type=%s, got %s", testErrorType, cleanerErr.Type)
	}
	if cleanerErr.Message != "test message" {
		t.Errorf("Expected Message=test message, got %s", cleanerErr.Message)
	}
}

func TestWrap(t *testing.T) {
	cleanerErr := Wrap(errUnderlying, testErrorType, "test message")

	if cleanerErr.Type != testErrorType {
		t.Errorf("Expected Type=%s, got %s", testErrorType, cleanerErr.Type)
	}
	if cleanerErr.Message != "test message" {
		t.Errorf("Expected Message=test message, got %s", cleanerErr.Message)
	}
	if !errors.Is(cleanerErr.Err, errUnderlying) {
		t.Errorf("Expected Err to wrap underlying error, got %v", cleanerErr.Err)
	}
}

func TestWrapf(t *testing.T) {
	cleanerErr := Wrapf(errUnderlying, testErrorType, "test message: %s", "formatted")

	if cleanerErr.Type != testErrorType {
		t.Errorf("Expected Type=%s, got %s", testErrorType, cleanerErr.Type)
	}
	if cleanerErr.Message != "test message: formatted" {
		t.Errorf("Expected Message=test message: formatted, got %s", cleanerErr.Message)
	}
	if !errors.Is(cleanerErr.Err, errUnderlying) {
		t.Errorf("Expected Err to wrap underlying error, got %v", cleanerErr.Err)
	}
}
