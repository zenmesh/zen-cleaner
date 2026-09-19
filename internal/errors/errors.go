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

// Package errors provides structured error types with context for Kubernetes controllers.
// This package enables consistent error handling across zen-cleaner, zen-lock, zen-watcher, and other components.
package errors

import (
	"errors"
	"fmt"
)

// ContextError represents an error with structured context information.
// This is a generic error type that can be extended by components for specific use cases.
type ContextError struct {
	// Type categorizes the error (e.g., "informer_creation_failed", "deletion_failed")
	Type string

	// Message is the error message
	Message string

	// Err is the underlying error
	Err error

	// Context holds arbitrary context fields (e.g., policy namespace, resource name)
	Context map[string]string
}

// Error implements the error interface.
func (e *ContextError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Unwrap returns the underlying error.
func (e *ContextError) Unwrap() error {
	return e.Err
}

// WithContext adds a context field to the error.
func (e *ContextError) WithContext(key, value string) *ContextError {
	if e.Context == nil {
		e.Context = make(map[string]string)
	}
	e.Context[key] = value
	return e
}

// GetContext returns the value of a context field.
func (e *ContextError) GetContext(key string) string {
	if e.Context == nil {
		return ""
	}
	return e.Context[key]
}

// New creates a new ContextError.
func New(errType, message string) *ContextError {
	return &ContextError{
		Type:    errType,
		Message: message,
		Context: make(map[string]string),
	}
}

// Wrap wraps an error with a message and type.
func Wrap(err error, errType, message string) *ContextError {
	return &ContextError{
		Type:    errType,
		Message: message,
		Err:     err,
		Context: make(map[string]string),
	}
}

// Wrapf wraps an error with a formatted message and type.
func Wrapf(err error, errType, format string, args ...interface{}) *ContextError {
	return &ContextError{
		Type:    errType,
		Message: fmt.Sprintf(format, args...),
		Err:     err,
		Context: make(map[string]string),
	}
}

// WithContext adds context to an error, creating a ContextError if needed.
func WithContext(err error, key, value string) *ContextError {
	var ctxErr *ContextError
	if errors.As(err, &ctxErr) && ctxErr != nil {
		return ctxErr.WithContext(key, value)
	}
	return &ContextError{
		Message: err.Error(),
		Err:     err,
		Context: map[string]string{key: value},
	}
}

// WithMultipleContext adds multiple context fields to an error.
func WithMultipleContext(err error, context map[string]string) *ContextError {
	var ctxErr *ContextError
	if errors.As(err, &ctxErr) && ctxErr != nil {
		if ctxErr.Context == nil {
			ctxErr.Context = make(map[string]string)
		}
		for k, v := range context {
			ctxErr.Context[k] = v
		}
		return ctxErr
	}
	return &ContextError{
		Message: err.Error(),
		Err:     err,
		Context: context,
	}
}
