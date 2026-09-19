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

package backoff

import (
	"testing"
	"time"
)

func TestNewBackoff(t *testing.T) {
	config := Config{
		Steps:    3,
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.0,
		Cap:      1 * time.Second,
	}

	b := NewBackoff(config)

	if b.Step() != 0 {
		t.Errorf("Initial step = %d, want 0", b.Step())
	}

	if b.IsExhausted() {
		t.Error("IsExhausted() = true, want false (initial state)")
	}
}

func TestBackoff_Next(t *testing.T) {
	config := Config{
		Steps:    3,
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.0, // No jitter for predictable testing
		Cap:      1 * time.Second,
	}

	b := NewBackoff(config)

	// First step: 100ms
	d1 := b.Next()
	if d1 < 90*time.Millisecond || d1 > 110*time.Millisecond {
		t.Errorf("First Next() = %v, want ~100ms", d1)
	}
	if b.Step() != 1 {
		t.Errorf("Step() after first Next() = %d, want 1", b.Step())
	}

	// Second step: 200ms (100ms * 2.0)
	d2 := b.Next()
	if d2 < 190*time.Millisecond || d2 > 210*time.Millisecond {
		t.Errorf("Second Next() = %v, want ~200ms", d2)
	}
	if b.Step() != 2 {
		t.Errorf("Step() after second Next() = %d, want 2", b.Step())
	}

	// Third step: 400ms (200ms * 2.0)
	d3 := b.Next()
	if d3 < 390*time.Millisecond || d3 > 410*time.Millisecond {
		t.Errorf("Third Next() = %v, want ~400ms", d3)
	}
	if b.Step() != 3 {
		t.Errorf("Step() after third Next() = %d, want 3", b.Step())
	}

	// Fourth step: should return 0 (exhausted)
	d4 := b.Next()
	if d4 != 0 {
		t.Errorf("Fourth Next() = %v, want 0 (exhausted)", d4)
	}
	if !b.IsExhausted() {
		t.Error("IsExhausted() = false, want true (after max steps)")
	}
}

func TestBackoff_Reset(t *testing.T) {
	config := DefaultConfig()
	b := NewBackoff(config)

	// Exhaust backoff
	for i := 0; i < config.Steps; i++ {
		b.Next()
	}

	if !b.IsExhausted() {
		t.Error("IsExhausted() = false, want true (after exhausting)")
	}

	// Reset
	b.Reset()

	if b.Step() != 0 {
		t.Errorf("Step() after Reset() = %d, want 0", b.Step())
	}
	if b.IsExhausted() {
		t.Error("IsExhausted() = true, want false (after reset)")
	}

	// Should be able to use again
	d := b.Next()
	if d == 0 {
		t.Error("Next() after Reset() = 0, want > 0")
	}
}

func TestBackoff_Cap(t *testing.T) {
	config := Config{
		Steps:    10,
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.0,
		Cap:      500 * time.Millisecond, // Cap at 500ms
	}

	b := NewBackoff(config)

	// First few steps should increase
	durations := []time.Duration{}
	for i := 0; i < 5; i++ {
		d := b.Next()
		durations = append(durations, d)
	}

	// Verify durations increase (with cap)
	for i := 1; i < len(durations); i++ {
		if durations[i] > config.Cap {
			t.Errorf("Duration[%d] = %v, exceeds cap %v", i, durations[i], config.Cap)
		}
	}
}
