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
	stderrors "errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	cleanerlog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"
	cleanerttl "github.com/zenmesh/zen-cleaner/internal/ttl"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

// matchesSelectors checks if a resource matches the target resource selectors.
func (r *PolicyReconciler) matchesSelectors(resource *unstructured.Unstructured, target *v1alpha1.TargetResourceSpec) bool {
	return matchesSelectorsShared(resource, target)
}

// shouldDelete determines if a resource should be deleted based on TTL and conditions.
func (r *PolicyReconciler) shouldDelete(resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy) (shouldDelete bool, reason string) {
	// Check conditions first
	if policy.Spec.Conditions != nil {
		if !r.meetsConditions(resource, policy.Spec.Conditions) {
			return false, ReasonConditionNotMet
		}
	}

	// Calculate expiration time
	expirationTime, err := r.calculateExpirationTime(resource, &policy.Spec.TTL)
	if err != nil {
		// ErrRelativeTTLExpired means the relative TTL is already in the past —
		// the resource is expired and should be deleted now.
		if stderrors.Is(err, cleanerttl.ErrRelativeTTLExpired) {
			return true, ReasonTTLExpired
		}
		// Use struct logger to avoid allocations
		r.logger.Debug("Could not calculate expiration time for resource", cleanerlog.Operation("should_delete"), cleanerlog.String("resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName())), cleanerlog.Error(err))
		return false, ReasonNoTTL
	}

	if expirationTime.IsZero() {
		return false, ReasonNoTTL
	}

	// Check if expired
	if time.Now().After(expirationTime) {
		return true, ReasonTTLExpired
	}

	return false, ReasonNotExpired
}

// calculateExpirationTime calculates the absolute expiration time for a resource based on policy.
// Returns zero time if TTL cannot be calculated or is invalid.
func (r *PolicyReconciler) calculateExpirationTime(resource *unstructured.Unstructured, ttlSpec *v1alpha1.TTLSpec) (time.Time, error) {
	return calculateExpirationTimeShared(resource, ttlSpec)
}

// meetsConditions checks if a resource meets the deletion conditions.
func (r *PolicyReconciler) meetsConditions(resource *unstructured.Unstructured, conditions *v1alpha1.ConditionsSpec) bool {
	return meetsConditionsShared(resource, conditions)
}

// deleteResource deletes a resource based on policy behavior.
func (r *PolicyReconciler) deleteResource(ctx context.Context, resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter) error {
	// Rate limiting
	if err := rateLimiter.Wait(ctx); err != nil {
		return err
	}

	// Dry run check
	if policy.Spec.Behavior.DryRun {
		RecordDryRunCandidate()
		r.logger.Info("[DRY RUN] Would delete resource", cleanerlog.Operation("delete_resource"), cleanerlog.String("resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName())))
		return nil
	}

	// Resolve GVR for deletion
	gvr := r.resolveGVRForDeletion(resource)

	// Build delete options
	deleteOptions := buildDeleteOptions(policy)

	// Perform deletion (safety gate + UID precondition live inside).
	return r.performResourceDeletion(ctx, resource, gvr, deleteOptions, policy.Spec.Behavior)
}

// getBatchSize returns the batch size for a policy.
func (r *PolicyReconciler) getBatchSize(policy *v1alpha1.ZenCleanerPolicy) int {
	return resolveBatchSize(policy, r.config)
}

// deleteBatch deletes a batch of resources and emits an execution receipt
// bound to the governing policy and plan digest (HELPER-H258+ H2). Dry-run
// emits no receipt (no fake successful effect receipt).
// Returns the number of successfully deleted resources and any errors encountered.
func (r *PolicyReconciler) deleteBatch(
	ctx context.Context,
	batch []*unstructured.Unstructured,
	policy *v1alpha1.ZenCleanerPolicy,
	rateLimiter *ratelimiter.RateLimiter,
	reasons map[string]string,
) (int64, []error) {
	n, errs, receipt := r.ExecuteWithReceipt(ctx, batch, policy, rateLimiter, reasons)
	if receipt != nil && r.eventRecorder != nil && r.Client != nil {
		ref := policy.DeepCopy()
		r.GetEventRecorder().Eventf(ref, "Normal", "PolicyExecutionReceipt",
			"plan=%s overall=%s effects=%d", receipt.PlanDigest, receipt.OverallStatus, len(receipt.Results))
	}
	return n, errs
}

// DeleteResourceWithBackoff deletes a resource with exponential backoff (implements BatchDeleter).
func (r *PolicyReconciler) DeleteResourceWithBackoff(ctx context.Context, resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter) error {
	return r.deleteResourceWithBackoff(ctx, resource, policy, rateLimiter)
}

// GetEventRecorder returns the event recorder (implements BatchDeleter).
func (r *PolicyReconciler) GetEventRecorder() *EventRecorder {
	return r.eventRecorder
}

// deleteResourceWithBackoff deletes a resource with exponential backoff retry logic.
func (r *PolicyReconciler) deleteResourceWithBackoff(ctx context.Context, resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter) error {
	return deleteResourceWithBackoffShared(ctx, resource, policy, rateLimiter, r, nil)
}

// DeleteResourceWithContext deletes a resource with context (implements ResourceDeleterWithContext).
func (r *PolicyReconciler) DeleteResourceWithContext(ctx context.Context, resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter) error {
	return r.deleteResource(ctx, resource, policy, rateLimiter)
}
