# Errors Package

**Structured error types with context for Kubernetes controllers**

## Overview

This package provides generic structured error types that can be used across zen-cleaner, zen-lock, zen-watcher, and other Kubernetes controller components. It enables consistent error handling with support for arbitrary context fields.

## Features

- ✅ **Structured Errors**: `ContextError` with type, message, and context
- ✅ **Error Wrapping**: Wrap underlying errors with additional context
- ✅ **Context Fields**: Add arbitrary key-value context (policy, resource, etc.)
- ✅ **Error Unwrapping**: Standard `Unwrap()` support for error chains

## Quick Start

```go
import "github.com/zenmesh/zen-cleaner/internal/pkg/errors"

// Create a new error
err := errors.New("informer_creation_failed", "failed to create informer")

// Add context
err = err.WithContext("policy", "my-policy")
err = err.WithContext("namespace", "default")

// Wrap an existing error
underlying := fmt.Errorf("connection failed")
err = errors.Wrap(underlying, "deletion_failed", "failed to delete resource")

// Wrap with formatted message
err = errors.Wrapf(underlying, "deletion_failed", "failed to delete resource %s", resourceName)

// Add context to any error
err = errors.WithContext(underlying, "policy", "my-policy")
err = errors.WithMultipleContext(underlying, map[string]string{
    "policy":    "my-policy",
    "namespace": "default",
    "resource":  "my-resource",
})
```

## Usage Examples

### Component-Specific Error Types

Components can extend `ContextError` for their specific needs:

```go
// In zen-cleaner
type CleanerError = errors.ContextError

func WithPolicy(err error, namespace, name string) *CleanerError {
    return errors.WithMultipleContext(err, map[string]string{
        "policy_namespace": namespace,
        "policy_name":      name,
    })
}

func WithResource(err error, namespace, name string) *CleanerError {
    return errors.WithMultipleContext(err, map[string]string{
        "resource_namespace": namespace,
        "resource_name":      name,
    })
}
```

### Error Handling

```go
// Check error type
var ctxErr *errors.ContextError
if errors.As(err, &ctxErr) {
    policy := ctxErr.GetContext("policy")
    namespace := ctxErr.GetContext("namespace")
    // Handle error with context
}

// Unwrap underlying error
underlying := errors.Unwrap(err)
```

## API Reference

### Types

#### `ContextError`

```go
type ContextError struct {
    Type    string            // Error type/category
    Message string            // Error message
    Err     error             // Underlying error
    Context map[string]string // Arbitrary context fields
}
```

### Functions

- `New(errType, message string) *ContextError` - Create a new error
- `Wrap(err error, errType, message string) *ContextError` - Wrap an error
- `Wrapf(err error, errType, format string, args ...interface{}) *ContextError` - Wrap with formatted message
- `WithContext(err error, key, value string) *ContextError` - Add context to any error
- `WithMultipleContext(err error, context map[string]string) *ContextError` - Add multiple context fields

### Methods

- `Error() string` - Implement error interface
- `Unwrap() error` - Return underlying error
- `WithContext(key, value string) *ContextError` - Add context field
- `GetContext(key string) string` - Get context field value

## Migration Guide

### From Component-Specific Errors

If you have component-specific error types (like `CleanerError`), you can migrate to `ContextError`:

**Before:**
```go
type CleanerError struct {
    Type              string
    PolicyNamespace   string
    PolicyName        string
    ResourceNamespace string
    ResourceName      string
    Message           string
    Err               error
}
```

**After:**
```go
type CleanerError = errors.ContextError

func WithPolicy(err error, namespace, name string) *CleanerError {
    return errors.WithMultipleContext(err, map[string]string{
        "policy_namespace": namespace,
        "policy_name":      name,
    })
}
```

## Related

- [internal/logging](../logging/README.md) - Error logging utilities
- [internal/backoff](../backoff/README.md) - Backoff logic for retries

