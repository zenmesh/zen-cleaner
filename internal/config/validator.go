package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// ValidationError represents a configuration validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validator validates environment configuration.
type Validator struct {
	errors []ValidationError
}

// NewValidator creates a new config validator.
func NewValidator() *Validator {
	return &Validator{errors: []ValidationError{}}
}

// RequireString validates a required string env var.
func (v *Validator) RequireString(key string) string {
	val := os.Getenv(key)
	if val == "" {
		v.errors = append(v.errors, ValidationError{
			Field:   key,
			Message: "required but not set",
		})
		return ""
	}
	return val
}

// OptionalString returns env var or default.
func (v *Validator) OptionalString(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

// RequireURL validates a required URL.
func (v *Validator) RequireURL(key string) string {
	val := v.RequireString(key)
	if val == "" {
		return ""
	}

	parsed, err := url.Parse(val)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		v.errors = append(v.errors, ValidationError{
			Field:   key,
			Message: fmt.Sprintf("invalid URL: %s", val),
		})
		return ""
	}
	return val
}

// OptionalURL returns env var as URL or default.
func (v *Validator) OptionalURL(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	parsed, err := url.Parse(val)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		v.errors = append(v.errors, ValidationError{
			Field:   key,
			Message: fmt.Sprintf("invalid URL: %s", val),
		})
		return defaultVal
	}
	return val
}

// RequireInt validates a required integer env var.
func (v *Validator) RequireInt(key string) int {
	val := v.RequireString(key)
	if val == "" {
		return 0
	}

	intVal, err := strconv.Atoi(val)
	if err != nil {
		v.errors = append(v.errors, ValidationError{
			Field:   key,
			Message: fmt.Sprintf("must be integer, got: %s", val),
		})
		return 0
	}
	return intVal
}

// OptionalInt returns env var as int or default.
func (v *Validator) OptionalInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	intVal, err := strconv.Atoi(val)
	if err != nil {
		v.errors = append(v.errors, ValidationError{
			Field:   key,
			Message: fmt.Sprintf("must be integer, got: %s", val),
		})
		return defaultVal
	}
	return intVal
}

// RequireBool validates a required boolean env var.
func (v *Validator) RequireBool(key string) bool {
	val := v.RequireString(key)
	if val == "" {
		return false
	}

	boolVal, err := strconv.ParseBool(val)
	if err != nil {
		v.errors = append(v.errors, ValidationError{
			Field:   key,
			Message: fmt.Sprintf("must be boolean (true/false), got: %s", val),
		})
		return false
	}
	return boolVal
}

// OptionalBool returns env var as bool or default.
func (v *Validator) OptionalBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	boolVal, err := strconv.ParseBool(val)
	if err != nil {
		v.errors = append(v.errors, ValidationError{
			Field:   key,
			Message: fmt.Sprintf("must be boolean (true/false), got: %s", val),
		})
		return defaultVal
	}
	return boolVal
}

// RequireDuration validates a required duration env var (e.g., "30s", "5m").
func (v *Validator) RequireDuration(key string) string {
	val := v.RequireString(key)
	if val == "" {
		return ""
	}

	// Basic validation - check if it looks like a duration
	// Full parsing should be done by caller using time.ParseDuration
	if !strings.HasSuffix(val, "s") && !strings.HasSuffix(val, "m") && !strings.HasSuffix(val, "h") {
		v.errors = append(v.errors, ValidationError{
			Field:   key,
			Message: fmt.Sprintf("must be duration (e.g., 30s, 5m, 1h), got: %s", val),
		})
		return ""
	}
	return val
}

// OptionalDuration returns env var as duration or default.
func (v *Validator) OptionalDuration(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	if !strings.HasSuffix(val, "s") && !strings.HasSuffix(val, "m") && !strings.HasSuffix(val, "h") {
		v.errors = append(v.errors, ValidationError{
			Field:   key,
			Message: fmt.Sprintf("must be duration (e.g., 30s, 5m, 1h), got: %s", val),
		})
		return defaultVal
	}
	return val
}

// RequireOneOf validates value is in allowed list.
func (v *Validator) RequireOneOf(key string, allowed []string) string {
	val := v.RequireString(key)
	if val == "" {
		return ""
	}

	for _, a := range allowed {
		if val == a {
			return val
		}
	}

	v.errors = append(v.errors, ValidationError{
		Field:   key,
		Message: fmt.Sprintf("must be one of %v, got: %s", allowed, val),
	})
	return ""
}

// RequireCSV validates comma-separated values.
func (v *Validator) RequireCSV(key string) []string {
	val := v.RequireString(key)
	if val == "" {
		return nil
	}

	parts := strings.Split(val, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		v.errors = append(v.errors, ValidationError{
			Field:   key,
			Message: "CSV list cannot be empty",
		})
	}
	return result
}

// OptionalCSV returns CSV or default.
func (v *Validator) OptionalCSV(key string, defaultVal []string) []string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}

	parts := strings.Split(val, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// ForbidInProduction ensures value is not set in production.
func (v *Validator) ForbidInProduction(key string) {
	env := os.Getenv("ENVIRONMENT")
	if env == "production" && os.Getenv(key) != "" {
		v.errors = append(v.errors, ValidationError{
			Field:   key,
			Message: "forbidden in production environment",
		})
	}
}

// Validate returns all validation errors.
func (v *Validator) Validate() error {
	if len(v.errors) == 0 {
		return nil
	}

	var msgs []string
	for _, e := range v.errors {
		msgs = append(msgs, e.Error())
	}
	return fmt.Errorf("%w:\n  - %s", ErrConfigValidationFailed, strings.Join(msgs, "\n  - "))
}

// HasErrors returns true if validation errors exist.
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// Errors returns all validation errors.
func (v *Validator) Errors() []ValidationError {
	return v.errors
}
