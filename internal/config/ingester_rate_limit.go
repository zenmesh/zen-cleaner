// Package config provides shared configuration utilities for Zen Platform services
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// IngesterRateLimitConfig represents rate limiting configuration for an ingester source
// This is used by both zen-ingester (webhook adapter) and zen-back (API rate limiting).
type IngesterRateLimitConfig struct {
	// RequestsPerMinute is the rate limit in requests per minute
	RequestsPerMinute int
	// Burst is the burst size (allows short bursts above the rate limit)
	Burst int
	// ConfigSource indicates where the config came from: "crd", "env_per_source", "env_default", "none"
	ConfigSource string
}

// CRDRateLimitConfig represents rate limit configuration from a CRD
// This interface allows different CRD types to provide rate limit configuration.
type CRDRateLimitConfig interface {
	// GetRequestsPerMinute returns the rate limit in requests per minute (0 if not configured)
	GetRequestsPerMinute() int
	// GetBurst returns the burst size (0 if not configured)
	GetBurst() int
}

// LoadIngesterRateLimitConfig loads rate limit configuration with priority:
// 1. CRD config (if provided and configured)
// 2. Per-source environment variable (RATE_LIMIT_INGESTER_{SOURCE}_RPM/BURST)
// 3. Default environment variable (RATE_LIMIT_INGESTER_DEFAULT_RPM/BURST)
// 4. No rate limit (returns nil)
//
// source: The ingester source name (e.g., "falco", "trivy", "kyverno")
// crdConfig: Optional CRD-based rate limit configuration (nil if not available)
//
// Returns the rate limit configuration and the source of the configuration.
func LoadIngesterRateLimitConfig(source string, crdConfig CRDRateLimitConfig) *IngesterRateLimitConfig {
	var rpm, burst int
	var configSource string

	// Priority 1: CRD config (highest priority)
	if crdConfig != nil {
		rpm = crdConfig.GetRequestsPerMinute()
		burst = crdConfig.GetBurst()
		if rpm > 0 {
			configSource = "crd"
		}
	}

	// Priority 2: Per-source environment variable
	if rpm == 0 {
		rpm = getIngesterRateLimitFromEnv(source, "RPM")
		burst = getIngesterRateLimitFromEnv(source, "BURST")
		if rpm > 0 {
			configSource = "env_per_source"
		}
	}

	// Priority 3: Default environment variable
	if rpm == 0 {
		rpm = getIngesterRateLimitFromEnv("DEFAULT", "RPM")
		if burst == 0 {
			burst = getIngesterRateLimitFromEnv("DEFAULT", "BURST")
		}
		if rpm > 0 {
			configSource = "env_default"
		}
	}

	// Priority 4: No rate limit configured
	if rpm == 0 {
		return nil
	}

	return &IngesterRateLimitConfig{
		RequestsPerMinute: rpm,
		Burst:             burst,
		ConfigSource:      configSource,
	}
}

// getIngesterRateLimitFromEnv gets rate limit value from environment variable
// Format: RATE_LIMIT_INGESTER_{SOURCE}_{TYPE} (e.g., RATE_LIMIT_INGESTER_FALCO_RPM)
// source: "falco", "trivy", "DEFAULT", etc.
// rateType: "RPM" or "BURST".
func getIngesterRateLimitFromEnv(source, rateType string) int {
	envKey := fmt.Sprintf("RATE_LIMIT_INGESTER_%s_%s", strings.ToUpper(source), rateType)
	envVal := os.Getenv(envKey)
	if envVal == "" {
		return 0
	}
	val, err := strconv.Atoi(envVal)
	if err != nil {
		// Log warning would require a logger, but we want to keep this package lightweight
		// Components can log warnings themselves if needed
		return 0
	}
	return val
}

// CRDRateLimitAdapter adapts different CRD rate limit config types to CRDRateLimitConfig interface
// This allows zen-ingester's SourceConfig.RateLimit to work with the shared loader

// SourceConfigRateLimitAdapter adapts zen-ingester's RateLimitConfig to CRDRateLimitConfig.
type SourceConfigRateLimitAdapter struct {
	ObservationsPerMinute int
	Burst                 int
}

// GetRequestsPerMinute returns the rate limit in requests per minute.
func (a *SourceConfigRateLimitAdapter) GetRequestsPerMinute() int {
	return a.ObservationsPerMinute
}

// GetBurst returns the burst size.
func (a *SourceConfigRateLimitAdapter) GetBurst() int {
	return a.Burst
}

// NewSourceConfigRateLimitAdapter creates an adapter from zen-ingester's RateLimitConfig.
func NewSourceConfigRateLimitAdapter(observationsPerMinute, burst int) CRDRateLimitConfig {
	return &SourceConfigRateLimitAdapter{
		ObservationsPerMinute: observationsPerMinute,
		Burst:                 burst,
	}
}
