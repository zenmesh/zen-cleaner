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

// Package ratelimiter provides rate limiting primitives for cleanup operations.
// Extracted from zen-cleaner and zen-watcher to eliminate duplication.
package ratelimiter

import (
	"context"

	"golang.org/x/time/rate"
)

// RateLimiter implements rate limiting using token bucket algorithm.
// This is a shared implementation used by zen-cleaner and zen-watcher.
type RateLimiter struct {
	limiter *rate.Limiter
}

// DefaultMaxPerSecond is the default maximum operations per second.
const DefaultMaxPerSecond = 10

// NewRateLimiter creates a new rate limiter.
// MaxPerSecond specifies the maximum number of operations allowed per second.
// If maxPerSecond <= 0, DefaultMaxPerSecond is used.
func NewRateLimiter(maxPerSecond int) *RateLimiter {
	if maxPerSecond <= 0 {
		maxPerSecond = DefaultMaxPerSecond
	}

	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(maxPerSecond), maxPerSecond),
	}
}

// Wait waits until the next operation is allowed, respecting the rate limit.
// It returns an error if the context is canceled.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	return rl.limiter.Wait(ctx)
}

// Allow checks if an operation is allowed without waiting.
// Returns true if allowed, false if rate limit exceeded.
func (rl *RateLimiter) Allow() bool {
	return rl.limiter.Allow()
}

// SetRate updates the rate limit dynamically.
// If maxPerSecond <= 0, DefaultMaxPerSecond is used.
func (rl *RateLimiter) SetRate(maxPerSecond int) {
	if maxPerSecond <= 0 {
		maxPerSecond = DefaultMaxPerSecond
	}
	rl.limiter.SetLimit(rate.Limit(maxPerSecond))
	rl.limiter.SetBurst(maxPerSecond)
}

// GetRate returns the current rate limit (operations per second).
func (rl *RateLimiter) GetRate() float64 {
	return float64(rl.limiter.Limit())
}
