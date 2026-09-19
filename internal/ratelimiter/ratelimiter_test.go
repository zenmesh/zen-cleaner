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

package ratelimiter

import (
	"context"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	tests := []struct {
		name         string
		maxPerSecond int
		wantRate     float64
	}{
		{
			name:         "valid rate",
			maxPerSecond: 5,
			wantRate:     5.0,
		},
		{
			name:         "zero rate uses default",
			maxPerSecond: 0,
			wantRate:     float64(DefaultMaxPerSecond),
		},
		{
			name:         "negative rate uses default",
			maxPerSecond: -1,
			wantRate:     float64(DefaultMaxPerSecond),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.maxPerSecond)
			if got := rl.GetRate(); got != tt.wantRate {
				t.Errorf("NewRateLimiter().GetRate() = %v, want %v", got, tt.wantRate)
			}
		})
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	rl := NewRateLimiter(10) // 10 ops/sec

	ctx := context.Background()

	// First call should succeed immediately (burst allows it)
	start := time.Now()
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("Wait() error = %v, want nil", err)
	}
	duration := time.Since(start)
	if duration > 50*time.Millisecond {
		t.Errorf("First Wait() took %v, expected < 50ms", duration)
	}

	// Subsequent calls should be rate limited
	// With 10 ops/sec, we should be able to do 10 quickly (burst), then wait
	for i := 0; i < 10; i++ {
		if err := rl.Wait(ctx); err != nil {
			t.Fatalf("Wait() error = %v, want nil", err)
		}
	}

	// Next call should be rate limited
	start = time.Now()
	if err := rl.Wait(ctx); err != nil {
		t.Fatalf("Wait() error = %v, want nil", err)
	}
	duration = time.Since(start)
	if duration < 90*time.Millisecond || duration > 150*time.Millisecond {
		t.Errorf("Rate-limited Wait() took %v, expected ~100ms", duration)
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(1) // 1 op/sec, burst = 1

	// First call should be allowed (burst)
	if !rl.Allow() {
		t.Error("Allow() = false, want true (first call)")
	}

	// Second call should be rate limited (burst exhausted)
	if rl.Allow() {
		t.Error("Allow() = true, want false (rate limited)")
	}

	// Wait a bit and try again (token should refill)
	time.Sleep(1100 * time.Millisecond)
	if !rl.Allow() {
		t.Error("Allow() = false after wait, want true")
	}
}

func TestRateLimiter_SetRate(t *testing.T) {
	rl := NewRateLimiter(5)
	if got := rl.GetRate(); got != 5.0 {
		t.Errorf("Initial rate = %v, want 5.0", got)
	}

	rl.SetRate(10)
	if got := rl.GetRate(); got != 10.0 {
		t.Errorf("After SetRate(10), rate = %v, want 10.0", got)
	}

	rl.SetRate(0)
	if got := rl.GetRate(); got != float64(DefaultMaxPerSecond) {
		t.Errorf("After SetRate(0), rate = %v, want %v", got, DefaultMaxPerSecond)
	}
}

func TestRateLimiter_ContextCancel(t *testing.T) {
	rl := NewRateLimiter(1) // 1 op/sec

	// Exhaust burst
	rl.Allow()
	rl.Allow()

	// Create canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Wait should return immediately with context error
	if err := rl.Wait(ctx); err == nil {
		t.Error("Wait() with canceled context = nil, want error")
	}
}
