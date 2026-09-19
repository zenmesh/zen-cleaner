# Config Package

**Package:** `github.com/zenmesh/zen-cleaner/internal/config`

**Purpose:** Environment variable validation and configuration helpers

## Overview

The `config` package provides utilities for validating and parsing environment variables with consistent error handling and type safety.

## Features

- ✅ **Batch Validation** - Collect multiple validation errors before failing
- ✅ **Type Safety** - Strongly typed helpers for common types
- ✅ **Production Safety** - Helpers to prevent dangerous config in production
- ✅ **Consistent Errors** - Standardized error messages

## Quick Start

```go
import "github.com/zenmesh/zen-cleaner/internal/config"

// Create validator
v := config.NewValidator()

// Validate required values
dbHost := v.RequireString("DB_HOST")
dbPort := v.RequireInt("DB_PORT")
apiURL := v.RequireURL("API_URL")

// Validate optional values with defaults
timeout := v.OptionalInt("TIMEOUT_SECONDS", 30)
debug := v.OptionalBool("DEBUG", false)

// Check for production safety
v.ForbidInProduction("DEBUG_MODE")

// Validate all at once
if err := v.Validate(); err != nil {
    log.Fatal(err)
}
```

## API Reference

### Validator (Batch Validation)

Use `Validator` when you want to collect multiple validation errors before failing:

```go
v := config.NewValidator()

// Validate multiple fields
dbHost := v.RequireString("DB_HOST")
dbPort := v.RequireInt("DB_PORT")
apiURL := v.RequireURL("API_URL")

// Check if any errors occurred
if v.HasErrors() {
    // Get all errors
    for _, err := range v.Errors() {
        log.Printf("Validation error: %s", err)
    }
    // Or get formatted error message
    if err := v.Validate(); err != nil {
        log.Fatal(err)
    }
}
```

**Methods:**
- `RequireString(key)` - Required string env var
- `OptionalString(key, default)` - Optional string with default
- `RequireInt(key)` - Required integer env var
- `OptionalInt(key, default)` - Optional integer with default
- `RequireBool(key)` - Required boolean env var
- `OptionalBool(key, default)` - Optional boolean with default
- `RequireURL(key)` - Required URL env var (http/https)
- `OptionalURL(key, default)` - Optional URL with default
- `RequireDuration(key)` - Required duration string (e.g., "30s")
- `OptionalDuration(key, default)` - Optional duration with default
- `RequireCSV(key)` - Required comma-separated values
- `OptionalCSV(key, default)` - Optional CSV with default
- `ForbidInProduction(key)` - Ensure value not set in production
- `Validate()` - Returns error if any validation failed
- `HasErrors()` - Check if errors exist
- `Errors()` - Get all validation errors

### Direct Helpers (Immediate Errors)

Use direct helpers when you want immediate error handling:

```go
import "github.com/zenmesh/zen-cleaner/internal/config"

// Returns error immediately if missing
dbHost, err := config.RequireEnv("DB_HOST")
if err != nil {
    log.Fatal(err)
}

// With default value
timeout := config.RequireEnvIntWithDefault("TIMEOUT", 30)
```

**Functions:**
- `RequireEnv(key)` - Required string (returns error)
- `RequireEnvWithDefault(key, default)` - String with default
- `RequireEnvInt(key)` - Required integer (returns error)
- `RequireEnvIntWithDefault(key, default)` - Integer with default
- `RequireEnvBool(key)` - Required boolean (returns error)
- `RequireEnvBoolWithDefault(key, default)` - Boolean with default
- `RequireEnvURL(key)` - Required URL (returns error)
- `RequireEnvURLWithDefault(key, default)` - URL with default
- `RequireEnvDuration(key)` - Required duration (returns error)
- `RequireEnvDurationWithDefault(key, default)` - Duration with default
- `RequireEnvCSV(key)` - Required CSV (returns error)
- `RequireEnvCSVWithDefault(key, default)` - CSV with default

## Examples

### Example 1: Batch Validation

```go
func LoadConfig() (*Config, error) {
    v := config.NewValidator()

    cfg := &Config{
        DBHost:     v.RequireString("DB_HOST"),
        DBPort:     v.RequireInt("DB_PORT"),
        APIURL:     v.RequireURL("API_URL"),
        Timeout:    v.OptionalInt("TIMEOUT_SECONDS", 30),
        Debug:      v.OptionalBool("DEBUG", false),
        AllowedIPs: v.OptionalCSV("ALLOWED_IPS", []string{"127.0.0.1"}),
    }

    // Production safety check
    v.ForbidInProduction("DEBUG_MODE")

    // Validate all at once
    if err := v.Validate(); err != nil {
        return nil, fmt.Errorf("configuration validation failed: %w", err)
    }

    return cfg, nil
}
```

### Example 2: Immediate Error Handling

```go
func LoadConfig() (*Config, error) {
    dbHost, err := config.RequireEnv("DB_HOST")
    if err != nil {
        return nil, err
    }

    dbPort, err := config.RequireEnvInt("DB_PORT")
    if err != nil {
        return nil, err
    }

    apiURL, err := config.RequireEnvURL("API_URL")
    if err != nil {
        return nil, err
    }

    timeout := config.RequireEnvIntWithDefault("TIMEOUT_SECONDS", 30)
    debug := config.RequireEnvBoolWithDefault("DEBUG", false)

    return &Config{
        DBHost:  dbHost,
        DBPort:  dbPort,
        APIURL:  apiURL,
        Timeout: timeout,
        Debug:   debug,
    }, nil
}
```

### Example 3: Production Safety

```go
func LoadConfig() (*Config, error) {
    v := config.NewValidator()

    cfg := &Config{
        DBHost: v.RequireString("DB_HOST"),
        // ... other config
    }

    // Ensure dangerous settings are not enabled in production
    v.ForbidInProduction("SKIP_AUTH")
    v.ForbidInProduction("DISABLE_TLS")
    v.ForbidInProduction("DEBUG_MODE")

    if err := v.Validate(); err != nil {
        return nil, err
    }

    return cfg, nil
}
```

## Migration Guide

### Before (Local Implementation)

```go
// Local validator
val := os.Getenv("DB_HOST")
if val == "" {
    return fmt.Errorf("DB_HOST is required")
}
```

### After (internal/config)

```go
import "github.com/zenmesh/zen-cleaner/internal/config"

v := config.NewValidator()
dbHost := v.RequireString("DB_HOST")
if err := v.Validate(); err != nil {
    return err
}
```

## Benefits

- ✅ **Consistency** - Same validation logic across all components
- ✅ **Type Safety** - Strongly typed helpers prevent errors
- ✅ **Better Errors** - Standardized, user-friendly error messages
- ✅ **Production Safety** - Built-in checks for dangerous config
- ✅ **Less Code** - No need to write validation logic

## Ingester Rate Limit Configuration

The `config` package provides a shared rate limit configuration loader for ingester components, harmonizing rate limiting across `zen-ingester` and `zen-back`:

### LoadIngesterRateLimitConfig

Loads rate limit configuration with priority order:
1. **CRD Configuration** (highest priority) - From Ingester CRD `spec.rateLimit`
2. **Per-Source Environment Variable** - `RATE_LIMIT_INGESTER_{SOURCE}_RPM/BURST`
3. **Default Environment Variable** - `RATE_LIMIT_INGESTER_DEFAULT_RPM/BURST`
4. **No Rate Limit** (returns `nil` if none configured)

**Usage:**
```go
import sdkconfig "github.com/zenmesh/zen-cleaner/internal/config"

// From CRD (zen-ingester)
var crdConfig sdkconfig.CRDRateLimitConfig
if config.RateLimit != nil {
    crdConfig = sdkconfig.NewSourceConfigRateLimitAdapter(
        config.RateLimit.ObservationsPerMinute,
        config.RateLimit.Burst,
    )
}

// Load configuration (harmonized across components)
rateLimitConfig := sdkconfig.LoadIngesterRateLimitConfig("falco", crdConfig)
if rateLimitConfig != nil {
    rps := float64(rateLimitConfig.RequestsPerMinute) / 60.0
    burst := rateLimitConfig.Burst
    // Use rateLimitConfig.ConfigSource to log where config came from
}
```

**Environment Variables:**
- `RATE_LIMIT_INGESTER_DEFAULT_RPM` - Default requests per minute (fallback)
- `RATE_LIMIT_INGESTER_DEFAULT_BURST` - Default burst size (fallback)
- `RATE_LIMIT_INGESTER_{SOURCE}_RPM` - Per-source RPM (e.g., `RATE_LIMIT_INGESTER_FALCO_RPM=100`)
- `RATE_LIMIT_INGESTER_{SOURCE}_BURST` - Per-source burst (e.g., `RATE_LIMIT_INGESTER_FALCO_BURST=200`)

**CRDRateLimitConfig Interface:**
```go
type CRDRateLimitConfig interface {
    GetRequestsPerMinute() int
    GetBurst() int
}
```

**SourceConfigRateLimitAdapter:**
Adapter for `zen-ingester`'s `RateLimitConfig` (with `ObservationsPerMinute` field):
```go
adapter := sdkconfig.NewSourceConfigRateLimitAdapter(observationsPerMinute, burst)
```

**Benefits:**
- ✅ **Consistency** - Same rate limit logic across `zen-ingester` and `zen-back`
- ✅ **Flexibility** - CRD config (per-ingester) with env var fallbacks (global defaults)
- ✅ **Operator Control** - Helm chart values map to environment variables
- ✅ **Observability** - `ConfigSource` field indicates where config came from (crd, env_per_source, env_default, none)

## See Also

- [Zen SDK README](../../README.md)
- [Migration Guide](../../docs/MIGRATION_GUIDE.md)

