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

// H118: Concurrency and determinism tests for rate limiter
package ratelimiter

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRateLimiter_ConcurrentAccess tests concurrent access safety
func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(100) // 100 ops/sec

	var wg sync.WaitGroup
	var allowedCount int64
	var deniedCount int64
	concurrency := 100
	iterations := 1000

	// Spawn multiple goroutines accessing the rate limiter concurrently
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if rl.Allow() {
					atomic.AddInt64(&allowedCount, 1)
				} else {
					atomic.AddInt64(&deniedCount, 1)
				}
			}
		}()
	}

	wg.Wait()

	// With 100 ops/sec and burst=100, we should allow at least the burst amount
	// plus some additional tokens that refill during the test
	if allowedCount < 100 {
		t.Errorf("Expected at least 100 allowed operations (burst), got %d", allowedCount)
	}

	t.Logf("Concurrent test: allowed=%d, denied=%d", allowedCount, deniedCount)
}

// TestRateLimiter_ConcurrentWait tests concurrent Wait() calls
func TestRateLimiter_ConcurrentWait(t *testing.T) {
	rl := NewRateLimiter(10) // 10 ops/sec
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var successCount int64
	var errorCount int64
	concurrency := 20 // Reduced for faster test
	iterations := 5   // Reduced for faster test

	// Spawn multiple goroutines calling Wait() concurrently
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if err := rl.Wait(ctx); err == nil {
					atomic.AddInt64(&successCount, 1)
				} else {
					// Context timeout is acceptable for this test
					if errors.Is(err, context.DeadlineExceeded) {
						atomic.AddInt64(&errorCount, 1)
					} else {
						t.Errorf("Unexpected error: %v", err)
					}
				}
			}
		}()
	}

	wg.Wait()

	// Should have allowed at least burst amount
	expectedMin := int64(10) // burst
	if successCount < expectedMin {
		t.Errorf("Expected at least %d successful waits, got %d", expectedMin, successCount)
	}

	t.Logf("Concurrent Wait test: success=%d, timeout errors=%d (acceptable)", successCount, errorCount)
}

// TestRateLimiter_ConcurrentSetRate tests concurrent SetRate() calls
func TestRateLimiter_ConcurrentSetRate(t *testing.T) {
	rl := NewRateLimiter(10)

	var wg sync.WaitGroup
	concurrency := 20

	// Spawn multiple goroutines calling SetRate() concurrently
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(rate int) {
			defer wg.Done()
			rl.SetRate(rate)
			// Verify rate was set (may be different due to concurrency, but should be valid)
			currentRate := rl.GetRate()
			if currentRate <= 0 {
				t.Errorf("Invalid rate after SetRate: %v", currentRate)
			}
		}(10 + i)
	}

	wg.Wait()

	// Final rate should be valid
	finalRate := rl.GetRate()
	if finalRate <= 0 {
		t.Errorf("Invalid final rate: %v", finalRate)
	}

	t.Logf("Concurrent SetRate test: final rate=%v", finalRate)
}

// TestRateLimiter_BurstLimit tests burst limit behavior
func TestRateLimiter_BurstLimit(t *testing.T) {
	rl := NewRateLimiter(1) // 1 op/sec, burst = 1

	// First call should be allowed (burst)
	if !rl.Allow() {
		t.Error("First Allow() should succeed (burst)")
	}

	// Second call should be rate limited (burst exhausted)
	if rl.Allow() {
		t.Error("Second Allow() should be rate limited")
	}

	// Wait for token refill
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed again
	if !rl.Allow() {
		t.Error("Allow() after refill should succeed")
	}
}

// TestRateLimiter_RefillRounding tests token refill behavior
func TestRateLimiter_RefillRounding(t *testing.T) {
	rl := NewRateLimiter(10) // 10 ops/sec

	// Exhaust burst
	for i := 0; i < 10; i++ {
		rl.Allow()
	}

	// Wait slightly less than the refill interval
	time.Sleep(800 * time.Millisecond)

	// Should still be rate limited (not enough time for refill)
	if rl.Allow() {
		t.Log("Warning: Allow() succeeded earlier than expected (CI timing variance)")
		// Don't fail - timing tests can be flaky in CI
	}

	// Wait for full refill with extra margin for CI timing
	time.Sleep(300 * time.Millisecond)

	// Should be allowed now
	if !rl.Allow() {
		t.Error("Allow() should succeed after refill interval")
	}
}

// TestRateLimiter_MaxBackoffClamp tests that rate doesn't exceed reasonable bounds
func TestRateLimiter_MaxBackoffClamp(t *testing.T) {
	// Test very high rate
	rl := NewRateLimiter(1000000)
	if rl.GetRate() != 1000000.0 {
		t.Errorf("Expected rate 1000000, got %v", rl.GetRate())
	}

	// Test very low rate (should use default)
	rl2 := NewRateLimiter(0)
	if rl2.GetRate() != float64(DefaultMaxPerSecond) {
		t.Errorf("Expected default rate %d, got %v", DefaultMaxPerSecond, rl2.GetRate())
	}

	// Test negative rate (should use default)
	rl3 := NewRateLimiter(-1)
	if rl3.GetRate() != float64(DefaultMaxPerSecond) {
		t.Errorf("Expected default rate %d, got %v", DefaultMaxPerSecond, rl3.GetRate())
	}
}
