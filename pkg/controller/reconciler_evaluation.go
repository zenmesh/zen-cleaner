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
	"fmt"
	"time"

	cleanerlog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	cleanererrors "github.com/zenmesh/zen-cleaner/pkg/errors"
)

// evaluationServiceKey builds a cache key for a policy's target resource.
// Each unique (apiVersion, kind, namespace) tuple gets its own service.
func evaluationServiceKey(policy *v1alpha1.ZenCleanerPolicy) string {
	return policy.Spec.TargetResource.APIVersion + "/" +
		policy.Spec.TargetResource.Kind + "/" +
		policy.Spec.TargetResource.Namespace
}

// getOrCreateEvaluationService builds or returns the cached PolicyEvaluationService
// for a given policy's target resource. Each unique (apiVersion, kind, namespace)
// tuple gets its own service with the correct resource lister.
// Thread-safe: double-checked locking under evaluationServiceMu.
func (r *PolicyReconciler) getOrCreateEvaluationService(ctx context.Context, policy *v1alpha1.ZenCleanerPolicy) (*PolicyEvaluationService, error) {
	key := evaluationServiceKey(policy)

	// Fast path: check with read lock
	r.evaluationServiceMu.RLock()
	if svc, ok := r.evaluationServices[key]; ok {
		r.evaluationServiceMu.RUnlock()
		return svc, nil
	}
	r.evaluationServiceMu.RUnlock()

	// Slow path: acquire write lock
	r.evaluationServiceMu.Lock()
	defer r.evaluationServiceMu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have created it)
	if svc, ok := r.evaluationServices[key]; ok {
		return svc, nil
	}

	// Create adapter
	adapter := NewPolicyReconcilerAdapter(r)

	// Get resource lister for this policy
	resourceLister, err := adapter.GetResourceListerForPolicy(ctx, policy)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource lister: %w", err)
	}

	// Create service with adapters
	// TTLCalculator is nil because PolicyEvaluationService uses shared functions internally
	svc := NewPolicyEvaluationService(
		resourceLister,
		adapter.GetSelectorMatcher(),
		adapter.GetConditionMatcher(),
		nil, // TTLCalculator (using shared function for now)
		adapter.GetRateLimiterProvider(),
		adapter.GetBatchDeleter(),
		r.statusUpdater,
		r.eventRecorder,
		r.config,
		r.logger,
	)
	// SUPPORT2-033 §2: evaluation-time safety gate (excluded objects never
	// join the deletion plan). The delete-time choke point runs regardless.
	svc.SetSafetyGate(r.safetyGate)

	r.evaluationServices[key] = svc
	return svc, nil
}

// evaluatePolicy runs one evaluation cycle for policy: PolicyEvaluationService
// (lister + DI) when available, otherwise the informer-store path in
// evaluate_policy_shared.go.
func (r *PolicyReconciler) evaluatePolicy(ctx context.Context, policy *v1alpha1.ZenCleanerPolicy) error {
	service, err := r.getOrCreateEvaluationService(ctx, policy)
	if err == nil {
		return service.EvaluatePolicy(ctx, policy)
	}
	r.logger.Debug("Evaluation service unavailable, using direct evaluation", cleanerlog.Operation("evaluate_policy"), cleanerlog.Error(err))
	// Use struct logger to avoid allocations
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		recordEvaluationDuration(policy.Namespace, policy.Name, duration)
	}()

	r.logger.Debug("Evaluating policy", cleanerlog.Operation("evaluate_policy"), cleanerlog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)))

	// Get or create resource informer for this policy
	informer, err := r.getOrCreateResourceInformer(ctx, policy)
	if err != nil {
		cleanerErr := cleanererrors.Wrap(err, "informer_creation_failed", "failed to get resource informer")
		cleanerErr = cleanerErr.WithContext("policy_namespace", policy.Namespace)
		cleanerErr = cleanerErr.WithContext("policy_name", policy.Name)
		recordError(policy.Namespace, policy.Name, "informer_creation_failed")
		r.logger.Error(cleanerErr, "Error creating resource informer for policy", cleanerlog.Operation("evaluate_policy"), cleanerlog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)), cleanerlog.ErrorCode("INFORMER_CREATION_FAILED"))
		return cleanerErr
	}

	// Evaluate resources and collect those to delete
	evalResult := evaluatePolicyResourcesShared(ctx, r, policy, informer)

	resourceAPIVersion := policy.Spec.TargetResource.APIVersion
	resourceKind := policy.Spec.TargetResource.Kind

	// Delete resources in batches
	deletedCount := deleteResourcesInBatchesShared(ctx, r, policy, evalResult.ResourcesToDelete, evalResult.ResourcesToDeleteReasons)
	evalResult.DeletedCount = deletedCount

	// Record pending resources metric
	if evalResult.PendingCount > 0 {
		recordResourcesPending(policy.Namespace, policy.Name, resourceAPIVersion, resourceKind, evalResult.PendingCount)
	}

	// Update policy status
	if err := updatePolicyStatusShared(ctx, r, policy, evalResult.MatchedCount, evalResult.DeletedCount, evalResult.PendingCount); err != nil {
		return err
	}

	// Record policy evaluation event
	if r.eventRecorder != nil {
		r.eventRecorder.RecordPolicyEvaluated(policy, evalResult.MatchedCount, evalResult.DeletedCount, evalResult.PendingCount)
	}

	return nil
}

// getStatusUpdater returns the status updater (implements PolicyEvaluator).
func (r *PolicyReconciler) getStatusUpdater() *StatusUpdater {
	return r.statusUpdater
}

// GetStatusUpdater returns the status updater (for testing).
func (r *PolicyReconciler) GetStatusUpdater() *StatusUpdater {
	return r.statusUpdater
}

// EvaluatePolicyForTesting allows injecting a PolicyEvaluationService for testing.
// This bypasses the normal getOrCreateEvaluationService flow.
func (r *PolicyReconciler) EvaluatePolicyForTesting(ctx context.Context, policy *v1alpha1.ZenCleanerPolicy, service *PolicyEvaluationService) error {
	key := evaluationServiceKey(policy)

	// Inject the service
	r.evaluationServiceMu.Lock()
	oldService := r.evaluationServices[key]
	r.evaluationServices[key] = service
	r.evaluationServiceMu.Unlock()

	// Evaluate using the injected service
	err := r.evaluatePolicy(ctx, policy)

	// Restore old service
	r.evaluationServiceMu.Lock()
	r.evaluationServices[key] = oldService
	r.evaluationServiceMu.Unlock()

	return err
}
