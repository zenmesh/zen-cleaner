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
	if spec.SecondsAfterCreation != nil {
		creationTime := resource.GetCreationTimestamp().Time
		return creationTime.Add(time.Duration(*spec.SecondsAfterCreation) * time.Second), nil
	}

	// Option 2: Dynamic TTL from field
	if spec.FieldPath != "" {
		fieldPath := parseFieldPath(spec.FieldPath)

		// Try to get as int64 first
		value, found, err := unstructured.NestedInt64(resource.Object, fieldPath...)
		if err == nil && found {
			creationTime := resource.GetCreationTimestamp().Time
			return creationTime.Add(time.Duration(value) * time.Second), nil
		}

		// Try as string for mappings
		strValue, found, err := unstructured.NestedString(resource.Object, fieldPath...)
		if err == nil && found {
			// Option 3: Mapped TTL
			if len(spec.Mappings) > 0 {
				var ttl int64
				if mappedTTL, ok := spec.Mappings[strValue]; ok {
					ttl = mappedTTL
				} else if spec.Default != nil {
					ttl = *spec.Default
				} else {
					return time.Time{}, fmt.Errorf("%w: %s", ErrNoMappingForFieldValue, strValue)
				}
				creationTime := resource.GetCreationTimestamp().Time
				return creationTime.Add(time.Duration(ttl) * time.Second), nil
			}
		}

		if !found && spec.Default != nil {
			creationTime := resource.GetCreationTimestamp().Time
			return creationTime.Add(time.Duration(*spec.Default) * time.Second), nil
		}
		return time.Time{}, fmt.Errorf("%w: %s", ErrFieldPathNotFound, spec.FieldPath)
	}

	// Option 4: Relative TTL (relative to another timestamp field)
	if spec.RelativeTo != "" && spec.SecondsAfter != nil {
		fieldPath := parseFieldPath(spec.RelativeTo)
		timestampStr, found, err := unstructured.NestedString(resource.Object, fieldPath...)
		if err != nil || !found {
			return time.Time{}, fmt.Errorf("%w: %s", ErrRelativeTimestampFieldNotFound, spec.RelativeTo)
		}

		timestamp, err := time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("%w: %w", ErrInvalidTimestampFormat, err)
		}

		// Calculate absolute expiration time from the relative timestamp
		expirationTime := timestamp.Add(time.Duration(*spec.SecondsAfter) * time.Second)
		if time.Now().After(expirationTime) {
			return time.Time{}, fmt.Errorf("%w", ErrRelativeTTLExpired)
		}
		return expirationTime, nil
	}

	return time.Time{}, fmt.Errorf("%w", ErrNoValidTTLConfiguration)
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
