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

package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
)

func TestResolveBatchSize(t *testing.T) {
	policyWithBehavior := func(batch int) *v1alpha1.ZenCleanerPolicy {
		return &v1alpha1.ZenCleanerPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
			Spec: v1alpha1.ZenCleanerPolicySpec{
				Behavior: v1alpha1.BehaviorSpec{BatchSize: batch},
			},
		}
	}
	policyDefault := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
	}

	t.Run("policy_behavior_wins_over_controller_config", func(t *testing.T) {
		cfg := config.NewControllerConfig().WithBatchSize(25)
		got := resolveBatchSize(policyWithBehavior(100), cfg)
		if got != 100 {
			t.Fatalf("got %d, want 100", got)
		}
	})

	t.Run("controller_config_when_no_policy_batch", func(t *testing.T) {
		cfg := config.NewControllerConfig().WithBatchSize(42)
		got := resolveBatchSize(policyDefault, cfg)
		if got != 42 {
			t.Fatalf("got %d, want 42", got)
		}
	})

	t.Run("default_when_nil_config_and_no_policy_batch", func(t *testing.T) {
		got := resolveBatchSize(policyDefault, nil)
		if got != DefaultBatchSize {
			t.Fatalf("got %d, want DefaultBatchSize %d", got, DefaultBatchSize)
		}
	})

	t.Run("default_controller_config_uses_pkg_default_batch", func(t *testing.T) {
		cfg := config.NewControllerConfig()
		got := resolveBatchSize(policyDefault, cfg)
		if got != cfg.BatchSize {
			t.Fatalf("got %d, want controller default batch %d", got, cfg.BatchSize)
		}
	})
}
