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

package logging

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
)

func TestDefaultSamplerConfig(t *testing.T) {
	cfg := DefaultSamplerConfig()
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"info rate", cfg.InfoSamplingRate, 0.1},
		{"debug rate", cfg.DebugSamplingRate, 0.01},
		{"success rate", cfg.SuccessSamplingRate, 0.01},
		{"error rate", cfg.ErrorSamplingRate, 1.0},
		{"warn rate", cfg.WarnSamplingRate, 1.0},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestShouldLogDeterministicRates(t *testing.T) {
	tests := []struct {
		name      string
		config    SamplerConfig
		level     zapcore.Level
		isSuccess bool
		want      bool
	}{
		{
			"error rate 1.0 always logs",
			SamplerConfig{ErrorSamplingRate: 1.0}, zapcore.ErrorLevel, false, true,
		},
		{
			"error rate 0.0 never logs",
			SamplerConfig{ErrorSamplingRate: 0.0}, zapcore.ErrorLevel, false, false,
		},
		{
			"warn rate 1.0 always logs",
			SamplerConfig{WarnSamplingRate: 1.0}, zapcore.WarnLevel, false, true,
		},
		{
			"warn rate 0.0 never logs",
			SamplerConfig{WarnSamplingRate: 0.0}, zapcore.WarnLevel, false, false,
		},
		{
			"success rate 1.0 always logs",
			SamplerConfig{SuccessSamplingRate: 1.0}, zapcore.InfoLevel, true, true,
		},
		{
			"success rate 0.0 never logs",
			SamplerConfig{SuccessSamplingRate: 0.0}, zapcore.InfoLevel, true, false,
		},
		{
			"info rate 1.0 always logs",
			SamplerConfig{InfoSamplingRate: 1.0}, zapcore.InfoLevel, false, true,
		},
		{
			"info rate 0.0 never logs",
			SamplerConfig{InfoSamplingRate: 0.0}, zapcore.InfoLevel, false, false,
		},
		{
			"debug rate 1.0 always logs",
			SamplerConfig{DebugSamplingRate: 1.0}, zapcore.DebugLevel, false, true,
		},
		{
			"debug rate 0.0 never logs",
			SamplerConfig{DebugSamplingRate: 0.0}, zapcore.DebugLevel, false, false,
		},
		{
			"unknown levels always log",
			SamplerConfig{InfoSamplingRate: 0.0}, zapcore.FatalLevel, false, true,
		},
		{
			"dpanic levels always log",
			SamplerConfig{DebugSamplingRate: 0.0}, zapcore.DPanicLevel, false, true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSampler(tt.config)
			for i := range 3 {
				if got := s.ShouldLog(tt.level, tt.isSuccess); got != tt.want {
					t.Fatalf("ShouldLog call %d = %v, want %v", i+1, got, tt.want)
				}
			}
		})
	}
}

func TestShouldLogCounterDeterministicSequence(t *testing.T) {
	// Info/debug sampling is counter-based and fully deterministic. Note the
	// deliberate double increment: ShouldLog bumps the counter itself and
	// shouldSampleWithCounter bumps it again, so with rate 0.5 (threshold 2)
	// every call lands on an even counter and is logged.
	s := NewSampler(SamplerConfig{SuccessSamplingRate: 0.5})
	for i := range 4 {
		if got := s.ShouldLog(zapcore.InfoLevel, true); !got {
			t.Fatalf("ShouldLog call %d = false, want true", i+1)
		}
	}

	// With rate 0.3 (threshold 3) only every third call is logged: the shared
	// counter advances by 2 per call, so calls 3 and 6 hit the modulo.
	s = NewSampler(SamplerConfig{SuccessSamplingRate: 0.3})
	want := []bool{false, false, true, false, false, true}
	for i, w := range want {
		if got := s.ShouldLog(zapcore.InfoLevel, true); got != w {
			t.Fatalf("ShouldLog call %d = %v, want %v", i+1, got, w)
		}
	}
}

func TestShouldSampleWithCounter(t *testing.T) {
	t.Run("rate 0.5 logs every second call", func(t *testing.T) {
		var counter uint64
		s := NewSampler(SamplerConfig{})
		want := []bool{false, true, false, true, false, true}
		for i, w := range want {
			if got := s.shouldSampleWithCounter(0.5, &counter); got != w {
				t.Fatalf("call %d = %v, want %v", i+1, got, w)
			}
		}
	})

	t.Run("rate 0.3 logs every third call", func(t *testing.T) {
		var counter uint64
		s := NewSampler(SamplerConfig{})
		want := []bool{false, false, true, false, false, true}
		for i, w := range want {
			if got := s.shouldSampleWithCounter(0.3, &counter); got != w {
				t.Fatalf("call %d = %v, want %v", i+1, got, w)
			}
		}
	})

	t.Run("rate at or above 1.0 skips counter", func(t *testing.T) {
		var counter uint64
		s := NewSampler(SamplerConfig{})
		for i := range 3 {
			if !s.shouldSampleWithCounter(1.0, &counter) {
				t.Fatalf("call %d = false, want true", i+1)
			}
		}
		if counter != 0 {
			t.Errorf("counter = %d, want 0 (short-circuited)", counter)
		}
	})

	t.Run("rate at or below 0.0 skips counter", func(t *testing.T) {
		var counter uint64
		s := NewSampler(SamplerConfig{})
		for _, rate := range []float64{0.0, -1.0} {
			if s.shouldSampleWithCounter(rate, &counter) {
				t.Errorf("rate %v = true, want false", rate)
			}
		}
		if counter != 0 {
			t.Errorf("counter = %d, want 0", counter)
		}
	})
}

func TestShouldSampleBoundaryRates(t *testing.T) {
	s := NewSampler(SamplerConfig{})
	tests := []struct {
		name string
		rate float64
		want bool
	}{
		{"rate 1.0 always true", 1.0, true},
		{"rate above 1.0 always true", 1.5, true},
		{"rate 0.0 always false", 0.0, false},
		{"negative rate always false", -0.5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range 5 {
				if got := s.shouldSample(tt.rate); got != tt.want {
					t.Fatalf("call %d = %v, want %v", i+1, got, tt.want)
				}
			}
		})
	}
}

func TestRateLimiterAllow(t *testing.T) {
	t.Run("allows up to max per window then denies", func(t *testing.T) {
		rl := NewRateLimiter(RateLimiterConfig{
			MaxLogsPerSecond: 3,
			WindowSize:       time.Second,
			CleanupInterval:  time.Minute,
		})
		for i := range 3 {
			if !rl.Allow("key") {
				t.Fatalf("call %d for key denied, want allowed", i+1)
			}
		}
		if rl.Allow("key") {
			t.Error("4th call for key allowed, want denied")
		}
	})

	t.Run("keys are rate limited independently", func(t *testing.T) {
		rl := NewRateLimiter(RateLimiterConfig{
			MaxLogsPerSecond: 1,
			WindowSize:       time.Second,
			CleanupInterval:  time.Minute,
		})
		if !rl.Allow("a") {
			t.Fatal("first call for key a denied")
		}
		if rl.Allow("a") {
			t.Error("second call for key a allowed, want denied")
		}
		if !rl.Allow("b") {
			t.Error("first call for key b denied, want allowed")
		}
	})

	t.Run("disabled when max is zero or negative", func(t *testing.T) {
		for _, max := range []int{0, -1} {
			rl := NewRateLimiter(RateLimiterConfig{MaxLogsPerSecond: max})
			for i := range 5 {
				if !rl.Allow("key") {
					t.Fatalf("max=%d call %d denied, want allowed", max, i+1)
				}
			}
		}
	})
}

func TestRateLimiterCleanup(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		MaxLogsPerSecond: 2,
		WindowSize:       time.Second,
		CleanupInterval:  time.Minute,
	})
	old := time.Now().Add(-time.Hour)
	rl.entries["stale"] = []time.Time{old, old}
	rl.entries["fresh"] = []time.Time{time.Now()}

	rl.cleanup(time.Now())

	if _, ok := rl.entries["stale"]; ok {
		t.Error("stale key should have been deleted by cleanup")
	}
	if _, ok := rl.entries["fresh"]; !ok {
		t.Error("fresh key should survive cleanup")
	}
}

func TestRateLimiterAllowTriggersPeriodicCleanup(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		MaxLogsPerSecond: 2,
		WindowSize:       time.Second,
		CleanupInterval:  time.Millisecond,
	})
	rl.entries["stale"] = []time.Time{time.Now().Add(-time.Hour)}
	rl.lastCleanup = time.Now().Add(-time.Hour)

	if !rl.Allow("new") {
		t.Fatal("Allow for new key denied, want allowed")
	}
	if _, ok := rl.entries["stale"]; ok {
		t.Error("periodic cleanup should have removed the stale key")
	}
}

func TestDefaultRateLimiterConfig(t *testing.T) {
	cfg := DefaultRateLimiterConfig()
	if cfg.MaxLogsPerSecond != 10 {
		t.Errorf("MaxLogsPerSecond = %d, want 10", cfg.MaxLogsPerSecond)
	}
	if cfg.WindowSize != time.Second {
		t.Errorf("WindowSize = %v, want 1s", cfg.WindowSize)
	}
	if cfg.CleanupInterval != time.Minute {
		t.Errorf("CleanupInterval = %v, want 1m", cfg.CleanupInterval)
	}
}

func TestSampledLogger(t *testing.T) {
	alwaysLog := SamplerConfig{
		InfoSamplingRate:    1.0,
		DebugSamplingRate:   1.0,
		SuccessSamplingRate: 1.0,
		ErrorSamplingRate:   1.0,
		WarnSamplingRate:    1.0,
	}

	t.Run("info logged when sampled in", func(t *testing.T) {
		logger, logs := newObservedLogger()
		sl := NewSampledLogger(logger, alwaysLog, RateLimiterConfig{MaxLogsPerSecond: 10})
		sl.Info("hello", true, "key", HTTPStatus(200))
		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Message != "hello" {
			t.Fatalf("got %d entries, want 1 with message hello", len(entries))
		}
		if entries[0].Level != zapcore.InfoLevel {
			t.Errorf("level = %v, want info", entries[0].Level)
		}
	})

	t.Run("info sampled out when rate is zero", func(t *testing.T) {
		logger, logs := newObservedLogger()
		sl := NewSampledLogger(logger, SamplerConfig{SuccessSamplingRate: 0.0},
			RateLimiterConfig{MaxLogsPerSecond: 10})
		sl.Info("hello", true, "key")
		sl.Info("hello", false, "key")
		if got := len(logs.TakeAll()); got != 0 {
			t.Errorf("got %d entries, want 0", got)
		}
	})

	t.Run("debug sampling", func(t *testing.T) {
		logger, logs := newObservedLogger()
		sl := NewSampledLogger(logger, SamplerConfig{DebugSamplingRate: 1.0},
			RateLimiterConfig{MaxLogsPerSecond: 10})
		sl.Debug("dbg", "key")
		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Level != zapcore.DebugLevel {
			t.Fatalf("got %d entries, want 1 debug entry", len(entries))
		}
	})

	t.Run("rate limiting applies before sampling", func(t *testing.T) {
		logger, logs := newObservedLogger()
		sl := NewSampledLogger(logger, alwaysLog,
			RateLimiterConfig{MaxLogsPerSecond: 1, WindowSize: time.Second})
		sl.Info("first", true, "key")
		sl.Info("second", true, "key")
		entries := logs.TakeAll()
		if len(entries) != 1 || entries[0].Message != "first" {
			t.Fatalf("got %d entries, want only the first one", len(entries))
		}
	})

	t.Run("warn is never sampled but is rate limited", func(t *testing.T) {
		logger, logs := newObservedLogger()
		sl := NewSampledLogger(logger, SamplerConfig{WarnSamplingRate: 0.0},
			RateLimiterConfig{MaxLogsPerSecond: 2, WindowSize: time.Second})
		sl.Warn("w1", "key")
		sl.Warn("w2", "key")
		sl.Warn("w3", "key")
		entries := logs.TakeAll()
		if len(entries) != 2 {
			t.Fatalf("got %d entries, want 2", len(entries))
		}
		if entries[0].Level != zapcore.WarnLevel || entries[1].Level != zapcore.WarnLevel {
			t.Error("entries should be warn level")
		}
	})

	t.Run("error is never sampled but is rate limited", func(t *testing.T) {
		logger, logs := newObservedLogger()
		sl := NewSampledLogger(logger, SamplerConfig{ErrorSamplingRate: 0.0},
			RateLimiterConfig{MaxLogsPerSecond: 2, WindowSize: time.Second})
		sl.Error(errors.New("boom"), "e1", "key")
		sl.Error(errors.New("boom"), "e2", "key")
		sl.Error(errors.New("boom"), "e3", "key")
		entries := logs.TakeAll()
		if len(entries) != 2 {
			t.Fatalf("got %d entries, want 2", len(entries))
		}
		if entries[0].Level != zapcore.ErrorLevel {
			t.Errorf("level = %v, want error", entries[0].Level)
		}
		if _, ok := findField(entries[0].Context, "error"); !ok {
			t.Error("error entry should contain the error field")
		}
	})

	t.Run("nil error still logs at error level", func(t *testing.T) {
		logger, logs := newObservedLogger()
		sl := NewSampledLogger(logger, alwaysLog, RateLimiterConfig{MaxLogsPerSecond: 10})
		sl.Error(nil, "oops", "key")
		entries := logs.TakeAll()
		if len(entries) != 1 {
			t.Fatalf("got %d entries, want 1", len(entries))
		}
		if entries[0].Level != zapcore.ErrorLevel {
			t.Errorf("level = %v, want error", entries[0].Level)
		}
		requireNoField(t, entries[0].Context, "error")
	})
}
