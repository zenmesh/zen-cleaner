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
	"fmt"
	"net"
	"net/http"
	"strings"
)

// ErrorClass categorizes errors for retry/fail/escalation decisions.
// Used across zen-* components when calling external APIs (HTTP, gRPC, etc.).
type ErrorClass int

const (
	// Transient indicates a temporary failure that should be retried
	// (HTTP 429, 502, 503, 504, 500, network timeout, connection reset).
	Transient ErrorClass = iota

	// Permanent indicates a non-recoverable failure that should not be retried
	// (HTTP 401, 403, 404, 400, 422, invalid request).
	Permanent

	// ResourceExhausted indicates a budget/quota limit was hit.
	// Retrying at the same level won't help — escalate or fail.
	ResourceExhausted
)

func (c ErrorClass) String() string {
	switch c {
	case Transient:
		return "transient"
	case Permanent:
		return "permanent"
	case ResourceExhausted:
		return "resource_exhausted"
	default:
		return "unknown"
	}
}

// ClassifiedError wraps an error with its classification for retry logic.
type ClassifiedError struct {
	// Err is the underlying error.
	Err error
	// Class is the error classification.
	Class ErrorClass
	// StatusCode is the HTTP status code, 0 if not an HTTP error.
	StatusCode int
}

// Error implements the error interface.
func (e *ClassifiedError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("[%s] status %d: %v", e.Class, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("[%s] %v", e.Class, e.Err)
}

// Unwrap returns the underlying error for errors.Is/As chains.
func (e *ClassifiedError) Unwrap() error { return e.Err }

// ClassifyError determines the retry class of an error by inspecting:
//   - Existing ClassifiedError wrappers (passthrough)
//   - HTTP status codes embedded in error messages
//   - Resource exhaustion patterns (token budget, context length)
//   - Network errors (timeout, connection refused, DNS failure)
//   - Defaults to Transient (safe for retry)
//
// The exhaustion patterns are configurable via ClassifyConfig.
func ClassifyError(err error) *ClassifiedError {
	return classifyWithConfig(err, defaultClassifyConfig)
}

// ClassifyConfig controls which error message patterns map to ResourceExhausted.
type ClassifyConfig struct {
	// ExhaustionPatterns are lowercase substrings that indicate resource/budget exhaustion.
	ExhaustionPatterns []string
}

var defaultClassifyConfig = ClassifyConfig{
	ExhaustionPatterns: []string{
		"token budget exhausted",
		"budget exceeded",
		"context_length_exceeded",
		"exceeds the available context",
		"maximum context length",
		"rate limit exceeded",
		"quota exceeded",
		"resource exhausted",
	},
}

// ClassifyErrorWithConfig is like ClassifyError but with custom exhaustion patterns.
func ClassifyErrorWithConfig(err error, cfg ClassifyConfig) *ClassifiedError {
	return classifyWithConfig(err, cfg)
}

func classifyWithConfig(err error, cfg ClassifyConfig) *ClassifiedError {
	if err == nil {
		return nil
	}

	// Unwrap wrapped errors to find existing ClassifiedError.
	unwrapped := err
	for {
		var ce *ClassifiedError
		if errors.As(unwrapped, &ce) {
			return ce
		}
		if n := errors.Unwrap(unwrapped); n != nil {
			unwrapped = n
		} else {
			break
		}
	}

	msg := strings.ToLower(unwrapped.Error())

	// Check for resource exhaustion patterns.
	for _, pattern := range cfg.ExhaustionPatterns {
		if strings.Contains(msg, pattern) {
			return &ClassifiedError{Err: err, Class: ResourceExhausted}
		}
	}

	// HTTP status code classification.
	if sc := extractHTTPStatus(msg); sc > 0 {
		switch {
		case isRetryableStatus(sc):
			return &ClassifiedError{Err: err, Class: Transient, StatusCode: sc}
		case sc == 401 || sc == 403:
			return &ClassifiedError{Err: err, Class: Permanent, StatusCode: sc}
		case sc == 404:
			return &ClassifiedError{Err: err, Class: Permanent, StatusCode: sc}
		case sc >= 400 && sc < 500:
			// Other 4xx = client error = permanent.
			return &ClassifiedError{Err: err, Class: Permanent, StatusCode: sc}
		case sc >= 500:
			return &ClassifiedError{Err: err, Class: Transient, StatusCode: sc}
		}
	}

	// Network errors — connection refused, timeout, DNS failure, reset.
	if isNetworkError(unwrapped) {
		return &ClassifiedError{Err: err, Class: Transient}
	}

	// Default: treat unknown errors as transient (safe for retry).
	return &ClassifiedError{Err: err, Class: Transient}
}

// extractHTTPStatus tries to parse an HTTP status code from error messages
// formatted as "status NNN" or "HTTP NNN".
func extractHTTPStatus(msg string) int {
	for _, prefix := range []string{"status ", "http "} {
		if idx := strings.Index(msg, prefix); idx >= 0 {
			rest := msg[idx+len(prefix):]
			var code int
			for i, ch := range rest {
				if ch < '0' || ch > '9' || i >= 3 {
					break
				}
				code = code*10 + int(ch-'0')
			}
			if code >= 100 && code < 600 {
				return code
			}
		}
	}
	return 0
}

func isNetworkError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") {
		return true
	}
	return false
}

// IsTransient returns true if the error is retryable.
func IsTransient(err error) bool {
	ce := ClassifyError(err)
	return ce != nil && ce.Class == Transient
}

// IsPermanent returns true if the error should not be retried.
func IsPermanent(err error) bool {
	ce := ClassifyError(err)
	return ce != nil && ce.Class == Permanent
}

// IsResourceExhausted returns true if the error indicates a budget/quota limit.
func IsResourceExhausted(err error) bool {
	ce := ClassifyError(err)
	return ce != nil && ce.Class == ResourceExhausted
}

// RetryableHTTPStatus returns true for HTTP status codes that warrant retry.
func RetryableHTTPStatus(code int) bool {
	return isRetryableStatus(code)
}

func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, // 429
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
		http.StatusInternalServerError: // 500
		return true
	}
	return false
}
