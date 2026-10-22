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
	"sync"

	"k8s.io/apimachinery/pkg/types"

	cleanerlog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
)

// getOrCreateRateLimiter gets or creates a rate limiter for a policy.
func (r *PolicyReconciler) getOrCreateRateLimiter(policy *v1alpha1.ZenCleanerPolicy) *ratelimiter.RateLimiter {
	return getOrCreateRateLimiterShared(r, policy)
}

// getRateLimiters returns the rate limiters map (implements RateLimiterManager).
func (r *PolicyReconciler) getRateLimiters() map[types.UID]*ratelimiter.RateLimiter {
	return r.rateLimiters
}

// getRateLimitersMu returns the rate limiters mutex (implements RateLimiterManager).
func (r *PolicyReconciler) getRateLimitersMu() *sync.RWMutex {
	return &r.rateLimitersMu
}

// getConfig returns the controller config (implements RateLimiterManager).
func (r *PolicyReconciler) getConfig() *config.ControllerConfig {
	return r.config
}

// trackPolicyUID tracks a policy UID by NamespacedName for cleanup on deletion.
func (r *PolicyReconciler) trackPolicyUID(nn types.NamespacedName, uid types.UID) {
	r.policyUIDsMu.Lock()
	defer r.policyUIDsMu.Unlock()
	r.policyUIDs[nn] = uid
}

// trackPolicySpec tracks a policy spec for change detection.
func (r *PolicyReconciler) trackPolicySpec(uid types.UID, spec *v1alpha1.ZenCleanerPolicySpec) {
	r.policySpecsMu.Lock()
	defer r.policySpecsMu.Unlock()
	// Deep copy the spec to avoid reference issues
	specCopy := spec.DeepCopy()
	r.policySpecs[uid] = specCopy
}

// cleanupPolicyResources cleans up all resources associated with a policy by NamespacedName.
func (r *PolicyReconciler) cleanupPolicyResources(nn types.NamespacedName) {
	r.policyUIDsMu.Lock()
	uid, exists := r.policyUIDs[nn]
	if exists {
		delete(r.policyUIDs, nn)
	}
	r.policyUIDsMu.Unlock()

	if !exists {
		// Policy UID not tracked, nothing to clean up
		return
	}

	// Use struct logger to avoid allocations
	r.logger.Info("Cleaning up resources for policy", cleanerlog.Operation("cleanup_policy_resources"), cleanerlog.String("policy", nn.Namespace+"/"+nn.Name), cleanerlog.String("uid", string(uid)))

	// Clean up resource informer
	r.cleanupResourceInformer(uid)

	// Clean up rate limiter
	r.cleanupRateLimiter(uid)

	// Clean up tracked spec
	r.policySpecsMu.Lock()
	delete(r.policySpecs, uid)
	r.policySpecsMu.Unlock()
}

// cleanupResourceInformer retains shared per-target informers: informers are
// observation infrastructure shared by every policy with the same target
// GVR/namespace, so a single policy deletion must not tear down observation
// for the remaining policies. Unreferenced informers are bounded by the
// distinct target set (small); lifecycle review tracked as a follow-up.
func (r *PolicyReconciler) cleanupResourceInformer(policyUID types.UID) {
	// Intentionally retained: shared-informer lifecycle is not policy-owned.
	_ = policyUID
}

// cleanupRateLimiter cleans up a rate limiter for a given policy UID.
func (r *PolicyReconciler) cleanupRateLimiter(policyUID types.UID) {
	r.rateLimitersMu.Lock()
	defer r.rateLimitersMu.Unlock()

	if _, exists := r.rateLimiters[policyUID]; exists {
		delete(r.rateLimiters, policyUID)
		// Use struct logger to avoid allocations
		r.logger.Debug("Cleaned up rate limiter for policy", cleanerlog.Operation("cleanup_rate_limiter"), cleanerlog.String("uid", string(policyUID)))
	}

	// Update metrics
	recordRateLimiterCount(len(r.rateLimiters))
}
