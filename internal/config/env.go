// Package config provides environment variable validation and configuration helpers
// for Kubernetes controllers. It simplifies required/optional env var handling
// with clear error messages and type conversion utilities.
//
// Usage:
//
//	v := config.NewValidator()
//	dbHost := v.RequireString("DB_HOST")
//	dbPort := v.RequireInt("DB_PORT")
//	if err := v.Validate(); err != nil {
//	    log.Fatal(err)
//	}
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// RequireEnv validates and returns a required environment variable
// Returns error if not set or empty.
func RequireEnv(key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", fmt.Errorf("%s: %w", key, ErrEnvRequired)
	}
	return val, nil
}

// RequireEnvWithDefault returns env var or default if not set.
func RequireEnvWithDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

// RequireEnvInt validates and returns a required integer environment variable.
func RequireEnvInt(key string) (int, error) {
	val, err := RequireEnv(key)
	if err != nil {
		return 0, err
	}
	intVal, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("%s: %w: %s", key, ErrEnvInvalidInt, val)
	}
	return intVal, nil
}

// RequireEnvIntWithDefault returns env var as int or default.
func RequireEnvIntWithDefault(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// RequireEnvBool validates and returns a required boolean environment variable.
func RequireEnvBool(key string) (bool, error) {
	val, err := RequireEnv(key)
	if err != nil {
		return false, err
	}
	boolVal, err := strconv.ParseBool(val)
	if err != nil {
		return false, fmt.Errorf("%s: %w: %s", key, ErrEnvInvalidBool, val)
	}
	return boolVal, nil
}

// RequireEnvBoolWithDefault returns env var as bool or default.
func RequireEnvBoolWithDefault(key string, defaultValue bool) bool {
	if val := os.Getenv(key); val != "" {
		if boolVal, err := strconv.ParseBool(val); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

// RequireEnvURL validates and returns a required URL environment variable.
func RequireEnvURL(key string) (string, error) {
	val, err := RequireEnv(key)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(val)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("%s: %w: %s", key, ErrEnvInvalidURL, val)
	}
	return val, nil
}

// RequireEnvURLWithDefault returns env var as URL or default.
func RequireEnvURLWithDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		parsed, err := url.Parse(val)
		if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
			return val
		}
	}
	return defaultValue
}

// RequireEnvOneOf validates that env var is one of allowed values.
func RequireEnvOneOf(key string, allowed []string) (string, error) {
	val, err := RequireEnv(key)
	if err != nil {
		return "", err
	}
	for _, a := range allowed {
		if val == a {
			return val, nil
		}
	}
	return "", fmt.Errorf("%s: %w: allowed %v, got %s", key, ErrEnvValueNotAllowed, allowed, val)
}

// RequireEnvSecret validates that a secret env var is set and meets minimum length.
func RequireEnvSecret(key string, minLength int) (string, error) {
	val, err := RequireEnv(key)
	if err != nil {
		return "", err
	}
	if len(val) < minLength {
		return "", fmt.Errorf("%s: %w: need at least %d characters (current %d)", key, ErrEnvSecretTooShort, minLength, len(val))
	}
	if strings.Contains(strings.ToLower(val), "change-me") {
		return "", fmt.Errorf("%s: %w", key, ErrEnvSecretWeak)
	}
	return val, nil
}

// RequireAtLeastOne validates that at least one of the provided env vars is set.
func RequireAtLeastOne(keys []string) error {
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			return nil
		}
	}
	return fmt.Errorf("%w: %v", ErrEnvAtLeastOne, keys)
}

// ValidateProduction checks production-specific security requirements.
func ValidateProduction() error {
	env := os.Getenv("ENVIRONMENT")
	if env != "production" {
		return nil // Skip validation in non-production
	}

	// Check DEBUG is disabled
	if os.Getenv("DEBUG") == "true" {
		return fmt.Errorf("%w", ErrProductionDebug)
	}

	// Check database SSL
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("CRDB_DSN")
	}
	if dbURL != "" && strings.Contains(dbURL, "sslmode=disable") {
		return fmt.Errorf("%w", ErrProductionDBSSL)
	}

	return nil
}

// ServiceConfigValidator validates service-specific configuration
// This is a helper that can be used by services to validate their config.
type ServiceConfigValidator struct {
	serviceName string
	errors      []string
}

// NewServiceConfigValidator creates a new service config validator.
func NewServiceConfigValidator(serviceName string) *ServiceConfigValidator {
	return &ServiceConfigValidator{
		serviceName: serviceName,
		errors:      []string{},
	}
}

// Require adds a required env var check.
func (v *ServiceConfigValidator) Require(key string) string {
	val, err := RequireEnv(key)
	if err != nil {
		v.errors = append(v.errors, err.Error())
		return ""
	}
	return val
}

// RequireWithDefault adds an optional env var with default.
func (v *ServiceConfigValidator) RequireWithDefault(key, defaultValue string) string {
	return RequireEnvWithDefault(key, defaultValue)
}

// RequireInt adds a required int env var check.
func (v *ServiceConfigValidator) RequireInt(key string) int {
	val, err := RequireEnvInt(key)
	if err != nil {
		v.errors = append(v.errors, err.Error())
		return 0
	}
	return val
}

// RequireIntWithDefault adds an optional int env var with default.
func (v *ServiceConfigValidator) RequireIntWithDefault(key string, defaultValue int) int {
	return RequireEnvIntWithDefault(key, defaultValue)
}

// RequireURL adds a required URL env var check.
func (v *ServiceConfigValidator) RequireURL(key string) string {
	val, err := RequireEnvURL(key)
	if err != nil {
		v.errors = append(v.errors, err.Error())
		return ""
	}
	return val
}

// RequireSecret adds a required secret env var check with minimum length.
func (v *ServiceConfigValidator) RequireSecret(key string, minLength int) string {
	val, err := RequireEnvSecret(key, minLength)
	if err != nil {
		v.errors = append(v.errors, err.Error())
		return ""
	}
	return val
}

// RequireAtLeastOne adds a check that at least one of the keys is set.
func (v *ServiceConfigValidator) RequireAtLeastOne(keys []string) {
	if err := RequireAtLeastOne(keys); err != nil {
		v.errors = append(v.errors, err.Error())
	}
}

// Validate returns all validation errors.
func (v *ServiceConfigValidator) Validate() error {
	if len(v.errors) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %w:\n  - %s", v.serviceName, ErrServiceConfigInvalid, strings.Join(v.errors, "\n  - "))
}

// HasErrors returns true if there are validation errors.
func (v *ServiceConfigValidator) HasErrors() bool {
	return len(v.errors) > 0
}

// Errors returns all validation errors.
func (v *ServiceConfigValidator) Errors() []string {
	return v.errors
}

// RequireEnvDuration validates and returns a required duration environment variable.
func RequireEnvDuration(key string) (time.Duration, error) {
	val, err := RequireEnv(key)
	if err != nil {
		return 0, err
	}
	duration, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("%s: %w: %s", key, ErrEnvInvalidDuration, val)
	}
	return duration, nil
}

// RequireEnvDurationWithDefault returns env var as duration or default.
func RequireEnvDurationWithDefault(key string, defaultValue time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			return duration
		}
	}
	return defaultValue
}

// RequireEnvCSV validates and returns a required CSV environment variable.
func RequireEnvCSV(key string) ([]string, error) {
	val, err := RequireEnv(key)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("%s: %w", key, ErrEnvCSVEmpty)
	}
	return result, nil
}

// RequireEnvCSVWithDefault returns env var as CSV or default.
func RequireEnvCSVWithDefault(key string, defaultValue []string) []string {
	if val := os.Getenv(key); val != "" {
		parts := strings.Split(val, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}
