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
	"context"
	"errors"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	sdklog "github.com/zenmesh/zen-cleaner/internal/logging"
	sdkttl "github.com/zenmesh/zen-cleaner/internal/ttl"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
	cleanererrors "github.com/zenmesh/zen-cleaner/pkg/errors"
	"github.com/zenmesh/zen-cleaner/pkg/safety"
	"github.com/zenmesh/zen-cleaner/pkg/validation"
)

// PolicyEvaluationService provides policy evaluation using injected dependencies.
// Uses interfaces for better testability.
type PolicyEvaluationService struct {
	resourceLister      ResourceLister
	selectorMatcher     SelectorMatcher
	conditionMatcher    ConditionMatcher
	ttlCalculator       TTLCalculator
	rateLimiterProvider RateLimiterProvider
	batchDeleter        BatchDeleterCore
	statusUpdater       *StatusUpdater
	safetyGate          *safety.Gate
	eventRecorder       *EventRecorder
	controllerConfig    *config.ControllerConfig
	logger              *sdklog.Logger
}

// SetSafetyGate wires the evaluation-time safety gate (SUPPORT2-033 §2).
// Nil clears the gate; the delete-time choke point in performResourceDeletion
// still runs regardless.
func (s *PolicyEvaluationService) SetSafetyGate(gate *safety.Gate) {
	s.safetyGate = gate
}

// NewPolicyEvaluationService creates a new PolicyEvaluationService with injected dependencies.
func NewPolicyEvaluationService(
	resourceLister ResourceLister,
	selectorMatcher SelectorMatcher,
	conditionMatcher ConditionMatcher,
	ttlCalculator TTLCalculator,
	rateLimiterProvider RateLimiterProvider,
	batchDeleter BatchDeleterCore,
	statusUpdater *StatusUpdater,
	eventRecorder *EventRecorder,
	controllerConfig *config.ControllerConfig,
	logger *sdklog.Logger,
) *PolicyEvaluationService {
	if logger == nil {
		logger = sdklog.NewLogger("zen-cleaner")
	}
	return &PolicyEvaluationService{
		resourceLister:      resourceLister,
		selectorMatcher:     selectorMatcher,
		conditionMatcher:    conditionMatcher,
		ttlCalculator:       ttlCalculator,
		rateLimiterProvider: rateLimiterProvider,
		batchDeleter:        batchDeleter,
		statusUpdater:       statusUpdater,
		eventRecorder:       eventRecorder,
		controllerConfig:    controllerConfig,
		logger:              logger,
	}
}

// EvaluatePolicy evaluates a policy using the injected dependencies.
// Uses dependency injection for testability.
func (s *PolicyEvaluationService) EvaluatePolicy(ctx context.Context, policy *v1alpha1.ZenCleanerPolicy) error {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		recordEvaluationDuration(policy.Namespace, policy.Name, duration)
	}()

	s.logger.Debug("Evaluating policy", sdklog.Operation("evaluate_policy"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)))

	// Parse GVR from policy (same validation as reconciler/informer path)
	gvr, err := validation.ParseGVR(policy.Spec.TargetResource.APIVersion, policy.Spec.TargetResource.Kind)
	if err != nil {
		cleanerErr := cleanererrors.Wrap(err, "invalid_gvr", "failed to parse GVR")
		cleanerErr = cleanerErr.WithContext("policy_namespace", policy.Namespace)
		cleanerErr = cleanerErr.WithContext("policy_name", policy.Name)
		recordError(policy.Namespace, policy.Name, "invalid_gvr")
		s.logger.Error(cleanerErr, "Invalid GVR in policy", sdklog.Operation("evaluate_policy"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)), sdklog.ErrorCode("INVALID_GVR"))
		return cleanerErr
	}

	// Get namespace (use "*" for all namespaces if empty)
	namespace := policy.Spec.TargetResource.Namespace
	if namespace == "" {
		namespace = "*"
	}

	// List resources using ResourceLister interface
	resources, err := s.resourceLister.ListResources(ctx, gvr, namespace)
	if err != nil {
		cleanerErr := cleanererrors.Wrap(err, "list_resources_failed", "failed to list resources")
		cleanerErr = cleanerErr.WithContext("policy_namespace", policy.Namespace)
		cleanerErr = cleanerErr.WithContext("policy_name", policy.Name)
		recordError(policy.Namespace, policy.Name, "list_resources_failed")
		s.logger.Error(cleanerErr, "Error listing resources", sdklog.Operation("evaluate_policy"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)), sdklog.ErrorCode("LIST_RESOURCES_FAILED"))
		return cleanerErr
	}

	var matchedCount, deletedCount, pendingCount int64

	// SUPPORT2-033 §8: per-cycle refusal summary surfaced on the policy
	// status so operators can answer "why was an object excluded?".
	refusals := make(map[string]int64)

	resourceAPIVersion := policy.Spec.TargetResource.APIVersion
	resourceKind := policy.Spec.TargetResource.Kind

	// Pre-allocate with estimated capacity
	estimatedDeletions := len(resources) / 10
	if estimatedDeletions < 10 {
		estimatedDeletions = 10
	}
	resourcesToDelete := make([]*unstructured.Unstructured, 0, estimatedDeletions)
	resourcesToDeleteReasons := make(map[string]string, estimatedDeletions)

	// Evaluate each resource
	matchedCount, pendingCount = s.evaluateResources(ctx, resources, policy, &resourcesToDelete, resourcesToDeleteReasons, refusals, resourceAPIVersion, resourceKind)

	// Delete resources in batches using BatchDeleterCore interface
	if len(resourcesToDelete) > 0 {
		deletedCount = s.deleteResourcesInBatches(ctx, policy, resourcesToDelete, resourcesToDeleteReasons)
	}

	// Record pending resources metric
	if pendingCount > 0 {
		recordResourcesPending(policy.Namespace, policy.Name, resourceAPIVersion, resourceKind, pendingCount)
	}

	// Update policy status
	if err := s.updatePolicyStatus(ctx, policy, matchedCount, deletedCount, pendingCount, refusals); err != nil {
		return err
	}

	// Record policy evaluation event
	if s.eventRecorder != nil {
		s.eventRecorder.RecordPolicyEvaluated(policy, matchedCount, deletedCount, pendingCount)
	}

	return nil
}

// evaluateResources evaluates all resources and builds the deletion list.
func (s *PolicyEvaluationService) evaluateResources(
	ctx context.Context,
	resources []*unstructured.Unstructured,
	policy *v1alpha1.ZenCleanerPolicy,
	resourcesToDelete *[]*unstructured.Unstructured,
	resourcesToDeleteReasons map[string]string,
	refusals map[string]int64,
	resourceAPIVersion, resourceKind string,
) (matchedCount, pendingCount int64) {
	// Check context cancellation at start to avoid unnecessary work
	select {
	case <-ctx.Done():
		s.logger.Debug("Stopping policy evaluation: context canceled", sdklog.Operation("evaluate_policy"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)))
		return int64(0), int64(0)
	default:
	}

	const contextCheckInterval = 100
	for i, resource := range resources {
		// Check context cancellation periodically
		if i%contextCheckInterval == 0 {
			select {
			case <-ctx.Done():
				s.logger.Debug("Stopping policy evaluation: context canceled", sdklog.Operation("evaluate_policy"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)))
				return matchedCount, pendingCount
			default:
			}
		}

		// Check if resource matches selectors using SelectorMatcher interface
		if !s.selectorMatcher.MatchesSelectors(resource, &policy.Spec.TargetResource) {
			continue
		}

		matchedCount++
		recordResourceMatched(policy.Namespace, policy.Name, resourceAPIVersion, resourceKind)

		// SUPPORT2-033 §2: evaluation-time safety gate. Refused objects are
		// excluded from the deletion plan entirely and surfaced by refusal
		// class (metrics + policy status).
		if s.safetyGate != nil {
			if allowed, refusal := s.safetyGate.EvaluateObject(resource, policy.Spec.Behavior); !allowed {
				refusals[refusal]++
				RecordSafetyRefusal(refusal)
				continue
			}
		}

		// Check conditions using ConditionMatcher interface
		if policy.Spec.Conditions != nil {
			if !s.conditionMatcher.MeetsConditions(resource, policy.Spec.Conditions) {
				pendingCount++
				continue
			}
		}

		// SUPPORT2-033 §7: the object reached the deletion decision point.
		RecordCandidateConsidered()

		// Check TTL using shared function (TTLCalculator interface is for future use)
		shouldDelete, reason := s.shouldDelete(resource, policy)
		if !shouldDelete {
			pendingCount++
			continue
		}

		// Add to deletion list
		*resourcesToDelete = append(*resourcesToDelete, resource)
		resourcesToDeleteReasons[string(resource.GetUID())] = reason
	}
	return matchedCount, pendingCount
}

// deleteResourcesInBatches deletes resources in batches.
func (s *PolicyEvaluationService) deleteResourcesInBatches(
	ctx context.Context,
	policy *v1alpha1.ZenCleanerPolicy,
	resourcesToDelete []*unstructured.Unstructured,
	resourcesToDeleteReasons map[string]string,
) int64 {
	// Check context cancellation at start
	select {
	case <-ctx.Done():
		s.logger.Debug("Stopping batch deletion: context canceled", sdklog.Operation("delete_batch"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)))
		return 0
	default:
	}

	rateLimiter := s.rateLimiterProvider.GetOrCreateRateLimiter(policy)
	if rateLimiter == nil {
		s.logger.Error(nil, "Rate limiter is nil, cannot proceed with deletions", sdklog.Operation("delete_batch"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)), sdklog.ErrorCode("RATE_LIMITER_NIL"))
		return 0
	}
	batchSize := s.getBatchSize(policy)
	deletedCount := int64(0)

	// Process deletions in batches
	for i := 0; i < len(resourcesToDelete); i += batchSize {
		// Check context cancellation between batches
		select {
		case <-ctx.Done():
			s.logger.Debug("Stopping batch deletion: context canceled", sdklog.Operation("delete_batch"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)))
			return deletedCount
		default:
		}

		end := i + batchSize
		if end > len(resourcesToDelete) {
			end = len(resourcesToDelete)
		}
		batch := resourcesToDelete[i:end]

		// Delete batch using BatchDeleterCore interface
		batchDeleted, batchErrors := s.batchDeleter.DeleteBatch(ctx, batch, policy, rateLimiter, resourcesToDeleteReasons)
		deletedCount += batchDeleted

		// Track deletion failures
		if len(batchErrors) > 0 {
			recordError(policy.Namespace, policy.Name, "deletion_failed")
		}

		// Log errors
		for _, err := range batchErrors {
			if s.eventRecorder != nil {
				s.eventRecorder.RecordEvaluationFailed(policy, err)
			}
			s.logger.Error(err, "Error deleting batch for policy", sdklog.Operation("delete_batch"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)), sdklog.ErrorCode("DELETE_BATCH_FAILED"))
		}
	}
	return deletedCount
}

// updatePolicyStatus updates the policy status.
func (s *PolicyEvaluationService) updatePolicyStatus(
	ctx context.Context,
	policy *v1alpha1.ZenCleanerPolicy,
	matchedCount, deletedCount, pendingCount int64,
	refusals map[string]int64,
) error {
	if s.statusUpdater == nil {
		return nil
	}

	statusCtx, statusCancel := context.WithTimeout(ctx, 10*time.Second)
	defer statusCancel()

	if err := s.statusUpdater.UpdateStatus(statusCtx, policy, matchedCount, deletedCount, pendingCount, refusals); err != nil {
		if statusCtx.Err() != nil {
			s.logger.Debug("Status update canceled or timed out", sdklog.Operation("update_status"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)), sdklog.Error(statusCtx.Err()))
			return nil
		}
		cleanerErr := cleanererrors.Wrap(err, "status_update_failed", "failed to update policy status")
		cleanerErr = cleanerErr.WithContext("policy_namespace", policy.Namespace)
		cleanerErr = cleanerErr.WithContext("policy_name", policy.Name)
		recordError(policy.Namespace, policy.Name, "status_update_failed")
		if s.eventRecorder != nil {
			s.eventRecorder.RecordStatusUpdateFailed(policy, cleanerErr)
		}
		s.logger.Error(cleanerErr, "Error updating policy status", sdklog.Operation("update_status"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)), sdklog.ErrorCode("UPDATE_STATUS_FAILED"))
		return cleanerErr
	}
	return nil
}

// shouldDelete determines if a resource should be deleted based on TTL.
func (s *PolicyEvaluationService) shouldDelete(resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy) (shouldDelete bool, reason string) {
	// Calculate expiration time using shared function
	expirationTime, err := calculateExpirationTimeShared(resource, &policy.Spec.TTL)
	if err != nil {
		// ErrRelativeTTLExpired means the relative TTL is already in the past —
		// the resource is expired and should be deleted now.
		if errors.Is(err, sdkttl.ErrRelativeTTLExpired) {
			return true, ReasonTTLExpired
		}
		s.logger.Debug("Could not calculate expiration time for resource", sdklog.Operation("should_delete"), sdklog.String("resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName())), sdklog.Error(err))
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

// getBatchSize returns the batch size for deletions (aligned with PolicyReconciler.getBatchSize).
func (s *PolicyEvaluationService) getBatchSize(policy *v1alpha1.ZenCleanerPolicy) int {
	return resolveBatchSize(policy, s.controllerConfig)
}
