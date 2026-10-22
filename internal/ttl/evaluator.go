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
	"errors"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Common TTL evaluation errors.
var (
	// ErrNoValidTTLConfiguration indicates no valid TTL configuration was provided.
	ErrNoValidTTLConfiguration = errors.New("no valid TTL configuration")

	// ErrFieldPathNotFound indicates the specified field path was not found in the resource.
	ErrFieldPathNotFound = errors.New("field path not found")

	// ErrNoMappingForFieldValue indicates no TTL mapping exists for the field value.
	ErrNoMappingForFieldValue = errors.New("no TTL mapping for field value")

	// ErrRelativeTimestampFieldNotFound indicates the RelativeTo timestamp field was not found.
	ErrRelativeTimestampFieldNotFound = errors.New("relative timestamp field not found")

	// ErrInvalidTimestampFormat indicates the timestamp field has an invalid format.
	ErrInvalidTimestampFormat = errors.New("invalid timestamp format (expected RFC3339)")

	// ErrRelativeTTLExpired indicates the relative TTL has already expired.
	ErrRelativeTTLExpired = errors.New("relative TTL already expired")
)

// CalculateExpirationTime calculates the absolute expiration time for a resource based on TTL spec.
// Returns zero time if TTL cannot be calculated or is invalid.
//
// Supported TTL modes:
// 1. Fixed TTL: SecondsAfterCreation (e.g., 3600 = 1 hour after creation)
// 2. Dynamic TTL: FieldPath pointing to int64 field (e.g., spec.ttlSeconds)
// 3. Mapped TTL: FieldPath pointing to string field + Mappings (e.g., severity -> TTL)
// 4. Relative TTL: RelativeTo timestamp field + SecondsAfter (e.g., 2 hours after last processed).
func CalculateExpirationTime(resource *unstructured.Unstructured, spec *Spec) (time.Time, error) {
	if spec == nil {
		return time.Time{}, ErrNoValidTTLConfiguration
	}

	// Option 1: Fixed TTL (seconds after creation)
	if expiration, handled := fixedTTLExpiration(resource, spec); handled {
		return expiration, nil
	}

	// Options 2 and 3: Dynamic TTL from a field (int64 direct or mapped string)
	if expiration, handled, err := fieldTTLExpiration(resource, spec); handled {
		return expiration, err
	}

	// Option 4: Relative TTL (relative to another timestamp field)
	if expiration, handled, err := relativeTTLExpiration(resource, spec); handled {
		return expiration, err
	}

	return time.Time{}, fmt.Errorf("%w", ErrNoValidTTLConfiguration)
}

// fixedTTLExpiration handles Option 1 (SecondsAfterCreation); handled reports
// whether this TTL mode applies to the spec.
func fixedTTLExpiration(resource *unstructured.Unstructured, spec *Spec) (time.Time, bool) {
	if spec.SecondsAfterCreation == nil {
		return time.Time{}, false
	}
	creationTime := resource.GetCreationTimestamp().Time
	return creationTime.Add(time.Duration(*spec.SecondsAfterCreation) * time.Second), true
}

// fieldTTLExpiration handles Options 2 (int64 field) and 3 (mapped string
// field); handled reports whether the FieldPath mode applies to the spec.
func fieldTTLExpiration(resource *unstructured.Unstructured, spec *Spec) (time.Time, bool, error) {
	if spec.FieldPath == "" {
		return time.Time{}, false, nil
	}
	fieldPath := parseFieldPath(spec.FieldPath)
	creationTime := resource.GetCreationTimestamp().Time

	// Try to get as int64 first
	value, found, err := unstructured.NestedInt64(resource.Object, fieldPath...)
	if err == nil && found {
		return creationTime.Add(time.Duration(value) * time.Second), true, nil
	}

	// Try as string for mappings
	strValue, found, err := unstructured.NestedString(resource.Object, fieldPath...)
	if err == nil && found && len(spec.Mappings) > 0 {
		// Option 3: Mapped TTL
		ttl, mapErr := mappedTTLForValue(spec, strValue)
		if mapErr != nil {
			return time.Time{}, true, mapErr
		}
		return creationTime.Add(time.Duration(ttl) * time.Second), true, nil
	}

	if !found && spec.Default != nil {
		return creationTime.Add(time.Duration(*spec.Default) * time.Second), true, nil
	}
	return time.Time{}, true, fmt.Errorf("%w: %s", ErrFieldPathNotFound, spec.FieldPath)
}

// mappedTTLForValue resolves the TTL for a mapped string field value,
// falling back to spec.Default when the value has no explicit mapping.
func mappedTTLForValue(spec *Spec, strValue string) (int64, error) {
	if mappedTTL, ok := spec.Mappings[strValue]; ok {
		return mappedTTL, nil
	}
	if spec.Default != nil {
		return *spec.Default, nil
	}
	return 0, fmt.Errorf("%w: %s", ErrNoMappingForFieldValue, strValue)
}

// relativeTTLExpiration handles Option 4 (RelativeTo + SecondsAfter);
// handled reports whether this TTL mode applies to the spec.
func relativeTTLExpiration(resource *unstructured.Unstructured, spec *Spec) (time.Time, bool, error) {
	if spec.RelativeTo == "" || spec.SecondsAfter == nil {
		return time.Time{}, false, nil
	}
	fieldPath := parseFieldPath(spec.RelativeTo)
	timestampStr, found, err := unstructured.NestedString(resource.Object, fieldPath...)
	if err != nil || !found {
		return time.Time{}, true, fmt.Errorf("%w: %s", ErrRelativeTimestampFieldNotFound, spec.RelativeTo)
	}

	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		return time.Time{}, true, fmt.Errorf("%w: %w", ErrInvalidTimestampFormat, err)
	}

	// Calculate absolute expiration time from the relative timestamp
	expirationTime := timestamp.Add(time.Duration(*spec.SecondsAfter) * time.Second)
	if time.Now().After(expirationTime) {
		return time.Time{}, true, fmt.Errorf("%w", ErrRelativeTTLExpired)
	}
	return expirationTime, true, nil
}

// IsExpired checks if a resource has expired based on TTL spec.
// Returns true if the resource should be deleted, false otherwise.
func IsExpired(resource *unstructured.Unstructured, spec *Spec) (bool, error) {
	expirationTime, err := CalculateExpirationTime(resource, spec)
	if err != nil {
		return false, err
	}

	if expirationTime.IsZero() {
		return false, nil
	}

	return time.Now().After(expirationTime), nil
}

// Parses a dot-separated field path into a slice for nested field access.
// Backslash-escaped dots are supported for annotation keys containing literal dots.
// Example: "spec.severity" -> ["spec", "severity"].
// Example: "status.lastProcessedAt" -> ["status", "lastProcessedAt"].
func parseFieldPath(path string) []string {
	if path == "" {
		return nil
	}
	var result []string
	var current strings.Builder
	for i := 0; i < len(path); i++ {
		switch {
		case path[i] == '\\' && i+1 < len(path) && path[i+1] == '.':
			current.WriteByte('.')
			i++
		case path[i] == '.':
			result = append(result, current.String())
			current.Reset()
		default:
			current.WriteByte(path[i])
		}
	}
	result = append(result, current.String())
	return result
}
