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

package ttl

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCalculateExpirationTime_FixedTTL(t *testing.T) {
	creationTime := time.Now().Add(-2 * time.Hour)
	resource := &unstructured.Unstructured{}
	resource.SetCreationTimestamp(metav1.Time{Time: creationTime})

	ttlSeconds := int64(3600) // 1 hour
	spec := &Spec{
		SecondsAfterCreation: &ttlSeconds,
	}

	expirationTime, err := CalculateExpirationTime(resource, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := creationTime.Add(1 * time.Hour)
	if !expirationTime.Truncate(time.Second).Equal(expected.Truncate(time.Second)) {
		t.Errorf("expected %v, got %v", expected, expirationTime)
	}
}

func TestCalculateExpirationTime_DynamicTTL(t *testing.T) {
	creationTime := time.Now().Add(-30 * time.Minute)
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"ttlSeconds": int64(3600),
			},
		},
	}
	resource.SetCreationTimestamp(metav1.Time{Time: creationTime})

	spec := &Spec{
		FieldPath: "spec.ttlSeconds",
	}

	expirationTime, err := CalculateExpirationTime(resource, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := creationTime.Add(1 * time.Hour)
	if !expirationTime.Truncate(time.Second).Equal(expected.Truncate(time.Second)) {
		t.Errorf("expected %v, got %v", expected, expirationTime)
	}
}

func TestCalculateExpirationTime_MappedTTL(t *testing.T) {
	creationTime := time.Now().Add(-2 * time.Hour)
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"severity": "critical",
			},
		},
	}
	resource.SetCreationTimestamp(metav1.Time{Time: creationTime})

	spec := &Spec{
		FieldPath: "spec.severity",
		Mappings: map[string]int64{
			"critical": 86400, // 24 hours
			"normal":   3600,  // 1 hour
		},
	}

	expirationTime, err := CalculateExpirationTime(resource, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := creationTime.Add(24 * time.Hour)
	if !expirationTime.Truncate(time.Second).Equal(expected.Truncate(time.Second)) {
		t.Errorf("expected %v, got %v", expected, expirationTime)
	}
}

func TestCalculateExpirationTime_MappedTTL_WithDefault(t *testing.T) {
	creationTime := time.Now().Add(-30 * time.Minute)
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"severity": "unknown",
			},
		},
	}
	resource.SetCreationTimestamp(metav1.Time{Time: creationTime})

	defaultTTL := int64(1800) // 30 minutes
	spec := &Spec{
		FieldPath: "spec.severity",
		Mappings: map[string]int64{
			"critical": 86400,
			"normal":   3600,
		},
		Default: &defaultTTL,
	}

	expirationTime, err := CalculateExpirationTime(resource, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := creationTime.Add(30 * time.Minute)
	if !expirationTime.Truncate(time.Second).Equal(expected.Truncate(time.Second)) {
		t.Errorf("expected %v, got %v", expected, expirationTime)
	}
}

func TestCalculateExpirationTime_RelativeTTL(t *testing.T) {
	lastProcessed := time.Now().Add(-30 * time.Minute)
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"lastProcessedAt": lastProcessed.Format(time.RFC3339),
			},
		},
	}

	secondsAfter := int64(3600) // 1 hour after lastProcessedAt
	spec := &Spec{
		RelativeTo:   "status.lastProcessedAt",
		SecondsAfter: &secondsAfter,
	}

	expirationTime, err := CalculateExpirationTime(resource, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := lastProcessed.Add(1 * time.Hour)
	if !expirationTime.Truncate(time.Second).Equal(expected.Truncate(time.Second)) {
		t.Errorf("expected %v, got %v", expected, expirationTime)
	}
}

func TestCalculateExpirationTime_RelativeTTL_Expired(t *testing.T) {
	lastProcessed := time.Now().Add(-2 * time.Hour)
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"lastProcessedAt": lastProcessed.Format(time.RFC3339),
			},
		},
	}

	secondsAfter := int64(3600) // 1 hour after lastProcessedAt (already expired)
	spec := &Spec{
		RelativeTo:   "status.lastProcessedAt",
		SecondsAfter: &secondsAfter,
	}

	_, err := CalculateExpirationTime(resource, spec)
	if err == nil || err.Error() != ErrRelativeTTLExpired.Error() {
		t.Errorf("expected ErrRelativeTTLExpired, got %v", err)
	}
}

func TestIsExpired_NotExpired(t *testing.T) {
	creationTime := time.Now().Add(-30 * time.Minute)
	resource := &unstructured.Unstructured{}
	resource.SetCreationTimestamp(metav1.Time{Time: creationTime})

	ttlSeconds := int64(3600) // 1 hour
	spec := &Spec{
		SecondsAfterCreation: &ttlSeconds,
	}

	expired, err := IsExpired(resource, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if expired {
		t.Error("expected resource to not be expired")
	}
}

func TestIsExpired_Expired(t *testing.T) {
	creationTime := time.Now().Add(-2 * time.Hour)
	resource := &unstructured.Unstructured{}
	resource.SetCreationTimestamp(metav1.Time{Time: creationTime})

	ttlSeconds := int64(3600) // 1 hour
	spec := &Spec{
		SecondsAfterCreation: &ttlSeconds,
	}

	expired, err := IsExpired(resource, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !expired {
		t.Error("expected resource to be expired")
	}
}

func TestCalculateExpirationTime_NoValidConfig(t *testing.T) {
	resource := &unstructured.Unstructured{}

	spec := &Spec{} // No TTL configuration

	_, err := CalculateExpirationTime(resource, spec)
	if err == nil || err.Error() != ErrNoValidTTLConfiguration.Error() {
		t.Errorf("expected ErrNoValidTTLConfiguration, got %v", err)
	}
}

// TestParseFieldPath_EscapedDots verifies that parseFieldPath correctly handles
// backslash-escaped dots in annotation keys. This is the regression guard for BUG-003
// where naive strings.Split(".") broke annotation keys containing dots.
func TestParseFieldPath_EscapedDots(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple dotted annotation key",
			input:    `metadata.annotations.cleaner\.zen-mesh\.io/ttl-seconds`,
			expected: []string{"metadata", "annotations", "cleaner.zen-mesh.io/ttl-seconds"},
		},
		{
			name:     "regular label path (no dots in segments)",
			input:    "metadata.labels.environment",
			expected: []string{"metadata", "labels", "environment"},
		},
		{
			name:     "single segment",
			input:    "spec",
			expected: []string{"spec"},
		},
		{
			name:     "nested field with dot in last segment",
			input:    `spec.owner\.name`,
			expected: []string{"spec", "owner.name"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "trailing dot",
			input:    "spec.severity.",
			expected: []string{"spec", "severity", ""},
		},
		{
			name:     "escaped dot after regular segments",
			input:    `metadata.annotations.example\.com/some-key`,
			expected: []string{"metadata", "annotations", "example.com/some-key"},
		},
		{
			name:     "backslash at end (malformed, no following char)",
			input:    `spec.trailing\`,
			expected: []string{"spec", "trailing\\"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFieldPath(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseFieldPath(%q) len=%d, want len=%d", tt.input, len(result), len(tt.expected))
				t.Errorf("  got:  %v", result)
				t.Errorf("  want: %v", tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("parseFieldPath(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

// TestCalculateExpirationTime_DynamicTTL_DottedAnnotation verifies that dynamic TTL mode works
// with annotation field paths containing escaped dots. Regression guard for BUG-003.
func TestCalculateExpirationTime_DynamicTTL_DottedAnnotation(t *testing.T) {
	creationTime := time.Now().Add(-30 * time.Minute)
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"cleaner.zen-mesh.io/ttl-seconds": int64(3600),
				},
			},
		},
	}
	resource.SetCreationTimestamp(metav1.Time{Time: creationTime})

	spec := &Spec{
		FieldPath: `metadata.annotations.cleaner\.zen-mesh\.io/ttl-seconds`,
	}

	expirationTime, err := CalculateExpirationTime(resource, spec)
	if err != nil {
		t.Fatalf("BUG-003: unexpected error with escaped dotted annotation key: %v", err)
	}

	expected := creationTime.Add(1 * time.Hour)
	if !expirationTime.Truncate(time.Second).Equal(expected.Truncate(time.Second)) {
		t.Errorf("expected %v, got %v", expected, expirationTime)
	}
}

// TestCalculateExpirationTime_RelativeTTL_DottedAnnotation verifies that relative TTL mode works
// with annotation field paths containing escaped dots. Regression guard for BUG-003.
func TestCalculateExpirationTime_RelativeTTL_DottedAnnotation(t *testing.T) {
	lastProcessed := time.Now().Add(-30 * time.Minute)
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"zen-mesh.io/last-processed": lastProcessed.Format(time.RFC3339),
				},
			},
		},
	}

	secondsAfter := int64(7200) // 2 hours — not expired
	spec := &Spec{
		RelativeTo:   `metadata.annotations.zen-mesh\.io/last-processed`,
		SecondsAfter: &secondsAfter,
	}

	expirationTime, err := CalculateExpirationTime(resource, spec)
	if err != nil {
		t.Fatalf("BUG-003: unexpected error with escaped dotted relativeTo: %v", err)
	}

	expected := lastProcessed.Add(2 * time.Hour)
	if !expirationTime.Truncate(time.Second).Equal(expected.Truncate(time.Second)) {
		t.Errorf("expected %v, got %v", expected, expirationTime)
	}
}

func TestCalculateExpirationTime_FieldPathNotFound(t *testing.T) {
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{},
		},
	}

	spec := &Spec{
		FieldPath: "spec.ttlSeconds",
	}

	_, err := CalculateExpirationTime(resource, spec)
	if err == nil {
		t.Error("expected error for field path not found")
	}
}
