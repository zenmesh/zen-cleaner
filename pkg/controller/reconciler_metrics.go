/*
Copyright 2025 Zen Mesh

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
	"context"

	cleanerlog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

// recordPolicyPhaseMetrics records metrics for policy phases.
// Uses controller-runtime cache to list all policies.
func (r *PolicyReconciler) recordPolicyPhaseMetrics(ctx context.Context) {
	// List all policies using the client cache
	policyList := &v1alpha1.ZenCleanerPolicyList{}
	if err := r.List(ctx, policyList); err != nil {
		// Use struct logger to avoid allocations
		r.logger.Debug("Failed to list policies for metrics", cleanerlog.Operation("record_policy_phase_metrics"), cleanerlog.Error(err))
		return
	}

	phaseCounts := make(map[string]float64)

	for i := range policyList.Items {
		policy := &policyList.Items[i]
		phase := policy.Status.Phase
		if phase == "" {
			// Determine phase from spec
			if policy.Spec.Paused {
				phase = PolicyPhasePaused
			} else {
				phase = PolicyPhaseActive
			}
		}
		phaseCounts[phase]++
	}

	// Update metrics for each phase
	for phase, count := range phaseCounts {
		recordPolicyPhase(phase, count)
	}

	// Reset phases that are no longer present
	knownPhases := []string{PolicyPhaseActive, PolicyPhasePaused, PolicyPhaseError}
	for _, phase := range knownPhases {
		if _, exists := phaseCounts[phase]; !exists {
			recordPolicyPhase(phase, 0)
		}
	}
}
