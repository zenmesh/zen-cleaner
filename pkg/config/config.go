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

// Package config provides centralized configuration management for the Zen Cleaner controller.
// This package now uses zen-cleaner/internal/pkg/config for validation.
package config

import (
	"os"
	"strings"
	"time"

	sdkconfig "github.com/zenmesh/zen-cleaner/internal/config"
)

// Default values for controller configuration.
const (
	// DefaultCleanupInterval is the default interval for cleanup runs.
	DefaultCleanupInterval = 1 * time.Minute

	// DefaultMaxDeletionsPerSecond is the default rate limit for deletions.
	DefaultMaxDeletionsPerSecond = 10

	// DefaultBatchSize is the default batch size for deletions.
	DefaultBatchSize = 50

	// DefaultMaxConcurrentEvaluations is the default number of concurrent policy evaluations.
	DefaultMaxConcurrentEvaluations = 5
)

// ControllerConfig holds configuration for the Zen Cleaner controller.
type ControllerConfig struct {
	// CleanupInterval is the interval between cleanup evaluation runs.
	CleanupInterval time.Duration

	// MaxDeletionsPerSecond is the default maximum deletions per second.
	// Individual policies can override this.
	MaxDeletionsPerSecond int

	// BatchSize is the default batch size for deletions.
	// Individual policies can override this.
	BatchSize int

	// MaxConcurrentEvaluations is the maximum number of policies to evaluate concurrently.
	// Defaults to 5 if not set.
	MaxConcurrentEvaluations int

	// DeleteEnabled is the emergency stop (SUPPORT2-033 Â§2/Â§19). When
	// false, nothing is deleted: candidates are logged and counted with
	// refusal reason "deletes_disabled". Defaults to true.
	DeleteEnabled bool

	// OwnNamespace is the namespace the controller runs in; it is always
	// protected. Empty in unit tests.
	OwnNamespace string

	// ProtectedNamespaces are denied in addition to the built-in protected
	// set (kube-system, kube-public, kube-node-lease, ...).
	ProtectedNamespaces []string
}

// NewControllerConfig creates a new controller config with defaults.
func NewControllerConfig() *ControllerConfig {
	return &ControllerConfig{
		CleanupInterval:          DefaultCleanupInterval,
		MaxDeletionsPerSecond:    DefaultMaxDeletionsPerSecond,
		BatchSize:                DefaultBatchSize,
		MaxConcurrentEvaluations: DefaultMaxConcurrentEvaluations,
	}
}

// LoadFromEnv loads configuration from environment variables.
// Environment variables override defaults if set.
// This implementation uses zen-cleaner/internal/pkg/config for validation.
func (c *ControllerConfig) LoadFromEnv() error {
	validator := sdkconfig.NewValidator()

	// ZEN_CLEANER_INTERVAL - duration string (e.g., "1m", "30s", "2h")
	if val := validator.OptionalDuration("ZEN_CLEANER_INTERVAL", ""); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			c.CleanupInterval = d
		}
		// If parsing fails, validator already validated format, so keep default
	}

	// ZEN_CLEANER_MAX_DELETIONS_PER_SECOND - integer
	if val := validator.OptionalInt("ZEN_CLEANER_MAX_DELETIONS_PER_SECOND", 0); val > 0 {
		c.MaxDeletionsPerSecond = val
	}

	// ZEN_CLEANER_BATCH_SIZE - integer
	if val := validator.OptionalInt("ZEN_CLEANER_BATCH_SIZE", 0); val > 0 {
		c.BatchSize = val
	}

	// ZEN_CLEANER_MAX_CONCURRENT_EVALUATIONS - integer
	if val := validator.OptionalInt("ZEN_CLEANER_MAX_CONCURRENT_EVALUATIONS", 0); val > 0 {
		c.MaxConcurrentEvaluations = val
	}

	// ZEN_CLEANER_DELETE_ENABLED - emergency stop (SUPPORT2-033 Â§19).
	// "false"/"0" disables ALL deletions (fail-safe default is enabled).
	if val := os.Getenv("ZEN_CLEANER_DELETE_ENABLED"); val != "" {
		switch val {
		case "false", "0", "no":
			c.DeleteEnabled = false
		case "true", "1", "yes":
			c.DeleteEnabled = true
		}
	} else {
		c.DeleteEnabled = true
	}

	// ZEN_CLEANER_PROTECTED_NAMESPACES - extra protected namespaces (CSV).
	if val := os.Getenv("ZEN_CLEANER_PROTECTED_NAMESPACES"); val != "" {
		for _, ns := range strings.Split(val, ",") {
			if ns = strings.TrimSpace(ns); ns != "" {
				c.ProtectedNamespaces = append(c.ProtectedNamespaces, ns)
			}
		}
	}

	// POD_NAMESPACE - the controller's own namespace (always protected).
	if val := os.Getenv("POD_NAMESPACE"); val != "" {
		c.OwnNamespace = val
	}

	// Return validation errors if any
	return validator.Validate()
}

// WithCleanupInterval sets the cleanup interval.
func (c *ControllerConfig) WithCleanupInterval(interval time.Duration) *ControllerConfig {
	c.CleanupInterval = interval
	return c
}

// WithMaxDeletionsPerSecond sets the max deletions per second.
func (c *ControllerConfig) WithMaxDeletionsPerSecond(rate int) *ControllerConfig {
	c.MaxDeletionsPerSecond = rate
	return c
}

// WithBatchSize sets the batch size.
func (c *ControllerConfig) WithBatchSize(size int) *ControllerConfig {
	c.BatchSize = size
	return c
}

// WithMaxConcurrentEvaluations sets the maximum concurrent evaluations.
func (c *ControllerConfig) WithMaxConcurrentEvaluations(maxConcurrent int) *ControllerConfig {
	c.MaxConcurrentEvaluations = maxConcurrent
	return c
}
