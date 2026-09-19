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

// Package ttl provides TTL (Time-To-Live) evaluation primitives for resource cleanup.
// Extracted from zen-cleaner to enable reuse across components.
package ttl

// Spec defines the TTL configuration for a resource.
// This is a platform-neutral representation extracted from zen-cleaner's TTLSpec.
type Spec struct {
	// SecondsAfterCreation is a fixed TTL in seconds after resource creation.
	// Example: 3600 (1 hour after creation)
	SecondsAfterCreation *int64

	// FieldPath is a dot-separated path to a field containing the TTL value.
	// The field can be:
	// - int64: TTL in seconds (e.g., spec.ttlSeconds: 3600)
	// - string: Used with Mappings for mapped TTL
	// Example: "spec.ttlSeconds"
	FieldPath string

	// Mappings define different TTL values based on field value (when FieldPath is string).
	// Example: {"critical": 86400, "normal": 3600, "low": 1800}
	Mappings map[string]int64

	// Default is the fallback TTL when:
	// - FieldPath field is not found, OR
	// - Field value has no mapping and Default is set
	Default *int64

	// RelativeTo is a field path to a timestamp field (RFC3339 format).
	// Used with SecondsAfter to calculate TTL relative to a specific timestamp.
	// Example: "status.lastProcessedAt"
	RelativeTo string

	// SecondsAfter is the TTL in seconds after the RelativeTo timestamp.
	// Only used when RelativeTo is set.
	// Example: 7200 (2 hours after status.lastProcessedAt)
	SecondsAfter *int64
}

// Config is an alias for Spec for backward compatibility.
type Config = Spec
