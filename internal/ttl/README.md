# TTL Package

TTL (Time-To-Live) evaluation primitives extracted from zen-cleaner.

## Overview

This package provides a platform-neutral TTL evaluation API that can be used by any component needing time-based resource cleanup without deploying the full zen-cleaner controller.

## Features

- **Fixed TTL**: Delete resources N seconds after creation
- **Dynamic TTL**: Read TTL from resource field (e.g., `spec.ttlSeconds`)
- **Mapped TTL**: Different TTLs based on field value (e.g., `critical: 86400, normal: 3600`)
- **Relative TTL**: Calculate TTL relative to a timestamp field (e.g., 2 hours after `status.lastProcessedAt`)
- **Fallback defaults**: Use default TTL when field is missing

## Usage

### Fixed TTL (Simple)

```go
import "github.com/zenmesh/zen-cleaner/internal/pkg/gc/ttl"

// Delete resources 1 hour after creation
ttlSeconds := int64(3600)
spec := &ttl.Spec{
    SecondsAfterCreation: &ttlSeconds,
}

expired, err := ttl.IsExpired(resource, spec)
if err == nil && expired {
    // Delete the resource
    client.Delete(ctx, resource)
}
```

### Dynamic TTL (Read from Resource)

```go
// Delete resources based on spec.ttlSeconds field
spec := &ttl.Spec{
    FieldPath: "spec.ttlSeconds",
}

expired, err := ttl.IsExpired(resource, spec)
```

### Mapped TTL (Different TTLs per Severity)

```go
// Different TTL based on spec.severity field
spec := &ttl.Spec{
    FieldPath: "spec.severity",
    Mappings: map[string]int64{
        "critical": 86400, // 24 hours
        "normal":   3600,  // 1 hour
        "low":      1800,  // 30 minutes
    },
}

expired, err := ttl.IsExpired(resource, spec)
```

### Relative TTL (Time Since Last Processed)

```go
// Delete 2 hours after status.lastProcessedAt timestamp
secondsAfter := int64(7200)
spec := &ttl.Spec{
    RelativeTo:   "status.lastProcessedAt",
    SecondsAfter: &secondsAfter,
}

expired, err := ttl.IsExpired(resource, spec)
```

### With Default Fallback

```go
// Use default if field is missing
defaultTTL := int64(3600)
spec := &ttl.Spec{
    FieldPath: "spec.ttlSeconds",
    Default:   &defaultTTL,
}

expired, err := ttl.IsExpired(resource, spec)
```

## API Reference

### Types

#### `Spec`

TTL configuration for a resource.

```go
type Spec struct {
    SecondsAfterCreation *int64           // Fixed TTL
    FieldPath            string           // Dynamic TTL field path
    Mappings             map[string]int64 // Mapped TTLs
    Default              *int64           // Fallback TTL
    RelativeTo           string           // Relative timestamp field
    SecondsAfter         *int64           // Seconds after RelativeTo
}
```

### Functions

#### `CalculateExpirationTime`

```go
func CalculateExpirationTime(resource *unstructured.Unstructured, spec *Spec) (time.Time, error)
```

Calculates the absolute expiration time for a resource.

#### `IsExpired`

```go
func IsExpired(resource *unstructured.Unstructured, spec *Spec) (bool, error)
```

Checks if a resource has expired based on TTL spec.

### Errors

- `ErrNoValidTTLConfiguration`: No valid TTL configuration provided
- `ErrFieldPathNotFound`: Specified field path not found in resource
- `ErrNoMappingForFieldValue`: No TTL mapping for field value
- `ErrRelativeTimestampFieldNotFound`: RelativeTo field not found
- `ErrInvalidTimestampFormat`: Timestamp not in RFC3339 format
- `ErrRelativeTTLExpired`: Relative TTL already expired

## Integration with zen-cleaner

zen-cleaner uses this package internally for all TTL evaluation. If you need more advanced features (conditions, batch deletion, metrics), use zen-cleaner controller.

## Integration with zen-watcher

zen-watcher uses this package for observation resource cleanup based on age.

## Testing

```bash
cd pkg/gc/ttl
go test -v ./...
```

## Extraction Status

✅ **Extracted** (H112 Phase 2)
- Core TTL evaluation logic
- All TTL modes (fixed, dynamic, mapped, relative)
- Error types and validation

