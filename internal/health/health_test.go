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

package health

import (
	"net/http"
	"testing"
	"time"
)

func TestInformerSyncChecker_ReadinessCheck(t *testing.T) {
	tests := []struct {
		name      string
		informers map[string]func() bool
		wantErr   bool
	}{
		{
			name:      "all informers synced",
			informers: map[string]func() bool{"informer1": func() bool { return true }},
			wantErr:   false,
		},
		{
			name:      "some informers not synced",
			informers: map[string]func() bool{"informer1": func() bool { return false }},
			wantErr:   true,
		},
		{
			name:      "no informers",
			informers: map[string]func() bool{},
			wantErr:   false,
		},
		{
			name:      "nil getter",
			informers: nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var getter func() map[string]func() bool
			if tt.informers != nil {
				getter = func() map[string]func() bool { return tt.informers }
			}
			checker := NewInformerSyncChecker(getter)
			err := checker.ReadinessCheck(&http.Request{})
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadinessCheck() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestActivityChecker_ReadinessCheck(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name                 string
		lastActivity         time.Time
		maxTimeSinceActivity time.Duration
		wantErr              bool
	}{
		{
			name:                 "recent activity",
			lastActivity:         now.Add(-1 * time.Minute),
			maxTimeSinceActivity: 5 * time.Minute,
			wantErr:              false,
		},
		{
			name:                 "old activity",
			lastActivity:         now.Add(-10 * time.Minute),
			maxTimeSinceActivity: 5 * time.Minute,
			wantErr:              true,
		},
		{
			name:                 "zero time",
			lastActivity:         time.Time{},
			maxTimeSinceActivity: 5 * time.Minute,
			wantErr:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getter := func() time.Time { return tt.lastActivity }
			checker := NewActivityChecker(getter, tt.maxTimeSinceActivity)
			err := checker.ReadinessCheck(&http.Request{})
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadinessCheck() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCompositeChecker(t *testing.T) {
	checker1 := NewInformerSyncChecker(func() map[string]func() bool {
		return map[string]func() bool{"informer1": func() bool { return true }}
	})
	checker2 := NewActivityChecker(time.Now, 5*time.Minute)

	composite := NewCompositeChecker(checker1, checker2)

	// All checkers pass
	if err := composite.ReadinessCheck(&http.Request{}); err != nil {
		t.Errorf("ReadinessCheck() error = %v, want nil", err)
	}

	// Add a failing checker
	failingChecker := NewInformerSyncChecker(func() map[string]func() bool {
		return map[string]func() bool{"informer2": func() bool { return false }}
	})
	composite.AddChecker(failingChecker)

	// Should fail now
	if err := composite.ReadinessCheck(&http.Request{}); err == nil {
		t.Error("ReadinessCheck() error = nil, want error")
	}
}
