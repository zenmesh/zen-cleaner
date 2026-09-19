package config

import "errors"

// Sentinel errors for configuration validation (err113 / static error wrapping).
var (
	ErrEnvRequired            = errors.New("environment variable is required but not set")
	ErrEnvInvalidInt          = errors.New("environment variable must be an integer")
	ErrEnvInvalidBool         = errors.New("environment variable must be a boolean (true/false)")
	ErrEnvInvalidURL          = errors.New("environment variable must be a valid URL (http:// or https://)")
	ErrEnvValueNotAllowed     = errors.New("environment variable value is not allowed")
	ErrEnvSecretTooShort      = errors.New("environment variable secret is too short")
	ErrEnvSecretWeak          = errors.New("environment variable contains weak placeholder text")
	ErrEnvAtLeastOne          = errors.New("at least one of the listed environment variables must be set")
	ErrProductionDebug        = errors.New("DEBUG must be false in production")
	ErrProductionDBSSL        = errors.New("DATABASE_URL must use SSL in production (remove sslmode=disable)")
	ErrServiceConfigInvalid   = errors.New("service configuration validation failed")
	ErrEnvInvalidDuration     = errors.New("environment variable must be a valid duration")
	ErrEnvCSVEmpty            = errors.New("environment variable CSV list cannot be empty")
	ErrConfigValidationFailed = errors.New("configuration validation failed")
)
