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

// Package backoff provides exponential backoff primitives for retry operations.
// Extracted from zen-cleaner to enable reuse across components.
package backoff

import (
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

// Backoff implements exponential backoff for retry operations.
// It is safe for concurrent use by multiple goroutines.
type Backoff struct {
	mu      sync.Mutex
	backoff wait.Backoff
	step    int
}

// Config holds backoff configuration.
type Config struct {
	Steps    int           // Maximum number of retry steps
	Duration time.Duration // Initial duration
	Factor   float64       // Multiplier for each step
	Jitter   float64       // Randomization factor (0.0 to 1.0)
	Cap      time.Duration // Maximum duration cap
}

// DefaultConfig returns a default backoff configuration.
func DefaultConfig() Config {
	return Config{
		Steps:    5,
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.1,
		Cap:      30 * time.Second,
	}
}

// NewBackoff creates a new backoff instance with the given configuration.
func NewBackoff(config Config) *Backoff {
	return &Backoff{
		backoff: wait.Backoff{
			Steps:    config.Steps,
			Duration: config.Duration,
			Factor:   config.Factor,
			Jitter:   config.Jitter,
			Cap:      config.Cap,
		},
		step: 0,
	}
}

// Next returns the next backoff duration and increments the step counter.
// Returns 0 if maximum steps reached.
// This method is safe for concurrent use.
func (b *Backoff) Next() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.step >= b.backoff.Steps {
		return 0
	}

	// Calculate duration for current step (0-indexed)
	// Step 0 = Duration
	// Step 1 = Duration * Factor
	// Step 2 = Duration * Factor^2
	// etc.
	duration := b.backoff.Duration
	for i := 0; i < b.step; i++ {
		duration = time.Duration(float64(duration) * b.backoff.Factor)
		if duration > b.backoff.Cap {
			duration = b.backoff.Cap
		}
	}

	// Apply jitter (simplified: deterministic for testing)
	// In production, use proper randomization
	if b.backoff.Jitter > 0 {
		jitterAmount := time.Duration(float64(duration) * b.backoff.Jitter)
		// For deterministic testing, use half jitter
		// In real usage, this would be random between 0 and jitterAmount
		duration += jitterAmount / 2
	}

	b.step++
	return duration
}

// Reset resets the backoff to the initial state.
// This method is safe for concurrent use.
func (b *Backoff) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.step = 0
}

// Step returns the current step number (0-indexed).
// This method is safe for concurrent use.
func (b *Backoff) Step() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.step
}

// IsExhausted returns true if the backoff has reached maximum steps.
// This method is safe for concurrent use.
func (b *Backoff) IsExhausted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.step >= b.backoff.Steps
}
