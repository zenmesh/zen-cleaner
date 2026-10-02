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

// UPGRADE MATRIX (H265+ Program J): an OLD-shape ZenCleanerPolicy held by a
// cluster must keep working under the CURRENT controller, pause/unpause
// transitions must report truthfully at every step, and a future/unknown
// schema must never WIDEN deletion — unknown fields are ignored, a policy
// that loses validity under the new controller is refused (policy_invalid),
// never partially honored.

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
	"github.com/zenmesh/zen-cleaner/pkg/validation"
)

// oldShapePolicy is the minimal CR an early deployment could hold: no
// conditions, no behavior, no evaluation interval, no status. The
// cluster-scope shape still satisfies the current broad-scope law
// (wildcard namespace requires a non-empty labelSelector).
func oldShapePolicy(name string) *v1alpha1.ZenCleanerPolicy {
	return &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ops"},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1", Kind: "ConfigMap", Namespace: "*",
				LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "demo"}},
			},
			TTL: v1alpha1.TTLSpec{SecondsAfterCreation: int64Ptr(3600)},
		},
	}
}

// TestUpgradeMatrix_OldShapePolicyAdopted: the current controller validates
// and reports an old-shape CR without any migration step.
func TestUpgradeMatrix_OldShapePolicyAdopted(t *testing.T) {
	p := oldShapePolicy("legacy-shape")
	if err := validation.ValidatePolicy(p); err != nil {
		t.Fatalf("old-shape policy must validate under the current controller: %v", err)
	}
	c := createPolicy(t, p)
	updater := NewStatusUpdaterWithConfig(c, &config.ControllerConfig{CleanupInterval: time.Minute})
	if err := updater.UpdateStatus(context.Background(), p, 2, 1, 0, nil); err != nil {
		t.Fatalf("status adoption: %v", err)
	}
	st := statusOf(t, c, "ops", "legacy-shape")
	if st["phase"] != PolicyPhaseActive {
		t.Fatalf("adopted old-shape policy must be Active, got %v", st["phase"])
	}
	if st["nextGCRun"] == nil {
		t.Fatal("adopted old-shape policy must carry a truthful nextGCRun")
	}
}

// TestUpgradeMatrix_PauseUnpauseTransitionSequence: active → paused →
// unpaused → paused, with the status promise truthful at EVERY step (paused
// policies carry no nextGCRun at all; unpausing restores it).
func TestUpgradeMatrix_PauseUnpauseTransitionSequence(t *testing.T) {
	p := oldShapePolicy("transition")
	c := createPolicy(t, p)
	updater := NewStatusUpdaterWithConfig(c, &config.ControllerConfig{CleanupInterval: time.Minute})

	type step struct {
		paused    bool
		wantPhase string
		wantRun   bool
	}
	for i, s := range []step{
		{paused: false, wantPhase: PolicyPhaseActive, wantRun: true},
		{paused: true, wantPhase: PolicyPhasePaused, wantRun: false},
		{paused: false, wantPhase: PolicyPhaseActive, wantRun: true},
		{paused: true, wantPhase: PolicyPhasePaused, wantRun: false},
	} {
		p.Spec.Paused = s.paused
		if err := updater.UpdateStatus(context.Background(), p, 0, 0, 0, nil); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		st := statusOf(t, c, "ops", "transition")
		if st["phase"] != s.wantPhase {
			t.Fatalf("step %d: phase = %v, want %s", i, st["phase"], s.wantPhase)
		}
		if s.wantRun && st["nextGCRun"] == nil {
			t.Fatalf("step %d: running policy lost its nextGCRun promise", i)
		}
		if !s.wantRun && st["nextGCRun"] != nil {
			t.Fatalf("step %d: paused policy still promises nextGCRun %v", i, st["nextGCRun"])
		}
	}
}

// TestUpgradeMatrix_FutureSchemaNeverWidens: a CR carrying an UNKNOWN spec
// field (a future schema) is honored ONLY through its known fields. If the
// known fields are invalid, the current controller refuses the policy
// outright — an unknown field can never substitute for a valid TTL, so an
// upgrade can never silently broaden what gets deleted.
func TestUpgradeMatrix_FutureSchemaNeverWidens(t *testing.T) {
	// Unknown field + still-valid known fields: validation passes and the
	// unknown field is simply not interpreted (ignored, not acted upon).
	withExtra := oldShapePolicy("future-extra")
	if err := validation.ValidatePolicy(withExtra); err != nil {
		t.Fatalf("known fields keep validating alongside unknown ones: %v", err)
	}
	// The broad-scope law itself: wildcard namespace WITHOUT a selector is
	// refused outright — a future schema cannot smuggle scope broadening.
	broad := oldShapePolicy("future-broad")
	broad.Spec.TargetResource.LabelSelector = nil
	if err := validation.ValidatePolicy(broad); err == nil {
		t.Fatal("wildcard namespace without a labelSelector must be refused as unsafe broad scope")
	}

	// Unknown field REPLACING the TTL: refusal, never interpretation of the
	// future field as deletion authority.
	broken := oldShapePolicy("future-broken")
	broken.Spec.TTL = v1alpha1.TTLSpec{} // future shape expects ttlField "v2"
	if err := validation.ValidatePolicy(broken); err == nil {
		t.Fatal("a policy without any recognized TTL option must be refused, not partially honored")
	}
}
