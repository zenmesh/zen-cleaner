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

// H118: Concurrency and determinism tests for backoff
package backoff

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestBackoff_ConcurrentAccess tests concurrent access safety
func TestBackoff_ConcurrentAccess(t *testing.T) {
	config := DefaultConfig()
	b := NewBackoff(config)

	var wg sync.WaitGroup
	var totalDurations int64
	var exhaustedCount int64
	concurrency := 50
	iterations := 100

	// Spawn multiple goroutines accessing the backoff concurrently
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				duration := b.Next()
				if duration == 0 {
					atomic.AddInt64(&exhaustedCount, 1)
				} else {
					atomic.AddInt64(&totalDurations, 1)
				}
			}
		}()
	}

	wg.Wait()

	// Should have some durations returned
	if totalDurations == 0 {
		t.Error("Expected some durations to be returned")
	}

	t.Logf("Concurrent test: durations=%d, exhausted=%d", totalDurations, exhaustedCount)
}

// TestBackoff_ConcurrentReset tests concurrent Reset() calls
func TestBackoff_ConcurrentReset(t *testing.T) {
	config := DefaultConfig()
	b := NewBackoff(config)

	var wg sync.WaitGroup
	concurrency := 20

	// Spawn multiple goroutines calling Reset() concurrently
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Reset()
			// Verify state is valid after reset
			if b.IsExhausted() {
				t.Error("Backoff should not be exhausted after reset")
			}
		}()
	}

	wg.Wait()

	// Final state should be valid
	if b.IsExhausted() {
		t.Error("Backoff should not be exhausted after concurrent resets")
	}
}

// TestBackoff_DeterministicDurations tests deterministic duration calculation
func TestBackoff_DeterministicDurations(t *testing.T) {
	config := Config{
		Steps:    5,
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.0, // No jitter for deterministic testing
		Cap:      1 * time.Second,
	}

	b1 := NewBackoff(config)
	b2 := NewBackoff(config)

	// Both should produce same durations
	for i := 0; i < config.Steps; i++ {
		d1 := b1.Next()
		d2 := b2.Next()

		if d1 != d2 {
			t.Errorf("Step %d: durations differ: %v vs %v", i, d1, d2)
		}
	}
}

// TestBackoff_MaxBackoffClamp tests that durations respect the cap
func TestBackoff_MaxBackoffClamp(t *testing.T) {
	config := Config{
		Steps:    20, // More steps than needed to exceed cap
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.0,
		Cap:      500 * time.Millisecond,
	}

	b := NewBackoff(config)

	maxDuration := time.Duration(0)
	for !b.IsExhausted() {
		duration := b.Next()
		if duration == 0 {
			break
		}
		if duration > maxDuration {
			maxDuration = duration
		}
		// All durations should be <= cap
		if duration > config.Cap {
			t.Errorf("Duration %v exceeds cap %v", duration, config.Cap)
		}
	}

	// Max duration should be at or near the cap
	if maxDuration > config.Cap {
		t.Errorf("Max duration %v exceeds cap %v", maxDuration, config.Cap)
	}

	t.Logf("Max duration: %v (cap: %v)", maxDuration, config.Cap)
}

// TestBackoff_EdgeBehavior tests edge cases
func TestBackoff_EdgeBehavior(t *testing.T) {
	// Test with Steps=0
	config1 := Config{
		Steps:    0,
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.0,
		Cap:      1 * time.Second,
	}
	b1 := NewBackoff(config1)
	if !b1.IsExhausted() {
		t.Error("Backoff with Steps=0 should be exhausted")
	}
	if b1.Next() != 0 {
		t.Error("Next() should return 0 when exhausted")
	}

	// Test with Factor=1.0 (no growth)
	config2 := Config{
		Steps:    5,
		Duration: 100 * time.Millisecond,
		Factor:   1.0,
		Jitter:   0.0,
		Cap:      1 * time.Second,
	}
	b2 := NewBackoff(config2)
	d1 := b2.Next()
	d2 := b2.Next()
	if d1 != d2 {
		t.Errorf("With Factor=1.0, durations should be equal: %v vs %v", d1, d2)
	}
}
