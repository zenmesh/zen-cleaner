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
	"sync"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sdklog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"
	sdkttl "github.com/zenmesh/zen-cleaner/internal/ttl"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
	cleanererrors "github.com/zenmesh/zen-cleaner/pkg/errors"
)

// PolicyReconciler reconciles ZenCleanerPolicy resources.
// It implements the controller-runtime Reconciler interface.
type PolicyReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	dynamicClient dynamic.Interface

	// Controller configuration.
	config *config.ControllerConfig

	// shouldReconcile is a function that returns true if reconciliation should proceed.
	// Leader election is handled by controller-runtime Manager, so this always returns true.
	shouldReconcile func() bool

	// Resource informers (one per policy).
	// Protected by resourceInformersMu mutex.
	resourceInformers map[string]cache.SharedInformer

	// Resource informer factories (one per policy).
	// Protected by resourceInformersMu mutex.
	resourceInformerFactories map[string]dynamicinformer.DynamicSharedInformerFactory

	// Mutex to protect resourceInformers and resourceInformerFactories maps.
	resourceInformersMu sync.RWMutex

	// Per-policy rate limiters (one per policy).
	// Protected by rateLimitersMu mutex.
	rateLimiters map[types.UID]*ratelimiter.RateLimiter

	// Mutex to protect rateLimiters map.
	rateLimitersMu sync.RWMutex

	// Track policy UIDs by NamespacedName for cleanup on deletion.
	// Protected by policyUIDsMu mutex.
	policyUIDs map[types.NamespacedName]types.UID

	// Mutex to protect policyUIDs map.
	policyUIDsMu sync.RWMutex

	// Track last known policy spec for update detection.
	// Protected by policySpecsMu mutex.
	policySpecs map[types.UID]*v1alpha1.ZenCleanerPolicySpec

	// Mutex to protect policySpecs map.
	policySpecsMu sync.RWMutex

	// Status updater.
	statusUpdater *StatusUpdater

	// Event recorder.
	eventRecorder *EventRecorder

	// Logger instance (reused to avoid allocations).
	logger *sdklog.Logger

	// RESTMapper for GVR resolution (optional, improves reliability for irregular CRDs).
	// If nil, falls back to pluralization-based resolution.
	restMapper meta.RESTMapper

	// GVRResolver for resolving GroupVersionResource from GroupVersionKind.
	// Uses RESTMapper if available, otherwise falls back to pluralization.
	gvrResolver *GVRResolver

	// PolicyEvaluationServices for the primary (lister + DI) evaluation path.
	// Keyed by target resource identifier (apiVersion/kind/namespace) so each
	// GVR gets its own lister. Lazily created; see getOrCreateEvaluationService.
	// Protected by evaluationServiceMu.
	evaluationServices map[string]*PolicyEvaluationService

	// Mutex to protect evaluationServices.
	evaluationServiceMu sync.RWMutex
}

// NewPolicyReconciler creates a new policy reconciler.
func NewPolicyReconciler(
	kubeClient client.Client,
	scheme *runtime.Scheme,
	dynamicClient dynamic.Interface,
	statusUpdater *StatusUpdater,
	eventRecorder *EventRecorder,
	cfg *config.ControllerConfig,
) *PolicyReconciler {
	return NewPolicyReconcilerWithRESTMapper(kubeClient, scheme, dynamicClient, nil, statusUpdater, eventRecorder, cfg)
}

// NewPolicyReconcilerWithRESTMapper creates a new policy reconciler with RESTMapper.
// RESTMapper is optional - if nil, falls back to pluralization-based resolution.
func NewPolicyReconcilerWithRESTMapper(
	kubeClient client.Client,
	scheme *runtime.Scheme,
	dynamicClient dynamic.Interface,
	restMapper meta.RESTMapper,
	statusUpdater *StatusUpdater,
	eventRecorder *EventRecorder,
	cfg *config.ControllerConfig,
) *PolicyReconciler {
	// Use default config if nil
	if cfg == nil {
		cfg = config.NewControllerConfig()
	}

	// Create GVRResolver with RESTMapper (nil is OK, will use pluralization fallback)
	gvrResolver := NewGVRResolver(restMapper)

	return &PolicyReconciler{
		Client:                    kubeClient,
		Scheme:                    scheme,
		dynamicClient:             dynamicClient,
		config:                    cfg,
		shouldReconcile:           func() bool { return true }, // Default: always reconcile
		resourceInformers:         make(map[string]cache.SharedInformer),
		resourceInformerFactories: make(map[string]dynamicinformer.DynamicSharedInformerFactory),
		rateLimiters:              make(map[types.UID]*ratelimiter.RateLimiter),
		policyUIDs:                make(map[types.NamespacedName]types.UID),
		policySpecs:               make(map[types.UID]*v1alpha1.ZenCleanerPolicySpec),
		statusUpdater:             statusUpdater,
		eventRecorder:             eventRecorder,
		logger:                    sdklog.NewLogger("zen-cleaner"),
		restMapper:                restMapper,
		gvrResolver:               gvrResolver,
		evaluationServices:        make(map[string]*PolicyEvaluationService),
	}
}

// NewPolicyReconcilerWithLeaderCheck creates a new policy reconciler with leader check function.
// Leader election is handled by controller-runtime Manager, so shouldReconcile is ignored (always returns true).
func NewPolicyReconcilerWithLeaderCheck(
	kubeClient client.Client,
	scheme *runtime.Scheme,
	dynamicClient dynamic.Interface,
	statusUpdater *StatusUpdater,
	eventRecorder *EventRecorder,
	cfg *config.ControllerConfig,
	shouldReconcile func() bool,
) *PolicyReconciler {
	// Use default config if nil
	if cfg == nil {
		cfg = config.NewControllerConfig()
	}

	// Leader election is handled by controller-runtime Manager.
	// Manager only calls Reconcile on the leader.
	return &PolicyReconciler{
		Client:                    kubeClient,
		Scheme:                    scheme,
		dynamicClient:             dynamicClient,
		config:                    cfg,
		shouldReconcile:           func() bool { return true }, // Always true (Manager handles leader election)
		resourceInformers:         make(map[string]cache.SharedInformer),
		resourceInformerFactories: make(map[string]dynamicinformer.DynamicSharedInformerFactory),
		rateLimiters:              make(map[types.UID]*ratelimiter.RateLimiter),
		policyUIDs:                make(map[types.NamespacedName]types.UID),
		policySpecs:               make(map[types.UID]*v1alpha1.ZenCleanerPolicySpec),
		statusUpdater:             statusUpdater,
		eventRecorder:             eventRecorder,
		logger:                    sdklog.NewLogger("zen-cleaner"),
		evaluationServices:        make(map[string]*PolicyEvaluationService),
	}
}

// Reconcile is the main reconciliation function called by controller-runtime.
// It is triggered by changes to ZenCleanerPolicy resources.
func (r *PolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Leader election is handled by controller-runtime Manager.
	// Manager only calls Reconcile on the leader pod, so shouldReconcile always returns true.
	_ = r.shouldReconcile()

	// Fetch the ZenCleanerPolicy instance
	policy := &v1alpha1.ZenCleanerPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if k8serrors.IsNotFound(err) {
			return r.handlePolicyDeletion(ctx, req)
		}
		return r.handlePolicyFetchError(err)
	}

	// Track policy UID for cleanup on deletion
	r.trackPolicyUID(req.NamespacedName, policy.UID)

	// Handle informer recreation if policy spec changed
	r.handleInformerRecreation(policy)

	// Store current spec for future comparison
	r.trackPolicySpec(policy.UID, &policy.Spec)

	// Skip paused policies
	if policy.Spec.Paused {
		return r.handlePausedPolicy()
	}

	// Evaluate the policy
	if err := r.evaluatePolicy(ctx, policy); err != nil {
		return r.handleEvaluationError(err, policy)
	}

	// Record policy phase metrics periodically
	r.recordPolicyPhaseMetrics(ctx)

	// Determine requeue interval based on policy evaluation interval or default
	requeueAfter := r.getRequeueIntervalForPolicy(policy)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// getRequeueInterval returns the requeue interval for a policy.
// Uses policy-specific evaluation interval if configured, otherwise uses default.
func (r *PolicyReconciler) getRequeueInterval() time.Duration {
	// Use policy-specific evaluation interval if configured
	// This allows per-policy control over evaluation frequency
	interval := DefaultCleanupInterval
	if r.config != nil {
		interval = r.config.CleanupInterval
	}
	return interval
}

// getRequeueIntervalForPolicy returns the requeue interval for a specific policy.
// Uses policy-specific evaluation interval if configured, otherwise uses default.
func (r *PolicyReconciler) getRequeueIntervalForPolicy(policy *v1alpha1.ZenCleanerPolicy) time.Duration {
	// Use policy-specific evaluation interval if configured
	if policy.Spec.EvaluationInterval != nil && policy.Spec.EvaluationInterval.Duration > 0 {
		return policy.Spec.EvaluationInterval.Duration
	}

	// Fall back to default cleanup interval from config
	interval := DefaultCleanupInterval
	if r.config != nil {
		interval = r.config.CleanupInterval
	}
	return interval
}

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
	r.logger.Debug("Evaluation service unavailable, using direct evaluation", sdklog.Operation("evaluate_policy"), sdklog.Error(err))
	// Use struct logger to avoid allocations
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		recordEvaluationDuration(policy.Namespace, policy.Name, duration)
	}()

	r.logger.Debug("Evaluating policy", sdklog.Operation("evaluate_policy"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)))

	// Get or create resource informer for this policy
	informer, err := r.getOrCreateResourceInformer(ctx, policy)
	if err != nil {
		cleanerErr := cleanererrors.Wrap(err, "informer_creation_failed", "failed to get resource informer")
		cleanerErr = cleanerErr.WithContext("policy_namespace", policy.Namespace)
		cleanerErr = cleanerErr.WithContext("policy_name", policy.Name)
		recordError(policy.Namespace, policy.Name, "informer_creation_failed")
		r.logger.Error(cleanerErr, "Error creating resource informer for policy", sdklog.Operation("evaluate_policy"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)), sdklog.ErrorCode("INFORMER_CREATION_FAILED"))
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
		if stderrors.Is(err, sdkttl.ErrRelativeTTLExpired) {
			return true, ReasonTTLExpired
		}
		// Use struct logger to avoid allocations
		r.logger.Debug("Could not calculate expiration time for resource", sdklog.Operation("should_delete"), sdklog.String("resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName())), sdklog.Error(err))
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
		r.logger.Info("[DRY RUN] Would delete resource", sdklog.Operation("delete_resource"), sdklog.String("resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName())))
		return nil
	}

	// Resolve GVR for deletion
	gvr := r.resolveGVRForDeletion(resource)

	// Build delete options
	deleteOptions := buildDeleteOptions(policy)

	// Perform deletion
	return r.performResourceDeletion(ctx, resource, gvr, deleteOptions)
}

// getOrCreateResourceInformer gets or creates a resource informer for a policy.
func (r *PolicyReconciler) getOrCreateResourceInformer(ctx context.Context, policy *v1alpha1.ZenCleanerPolicy) (cache.SharedInformer, error) {
	// Check if informer already exists (with read lock)
	// Shared-informer lookup: keyed by target GVR+namespace, not policy UID.
	informerKey := evaluationServiceKey(policy)
	r.resourceInformersMu.RLock()
	if informer, ok := r.resourceInformers[informerKey]; ok {
		r.resourceInformersMu.RUnlock()
		return informer, nil
	}
	r.resourceInformersMu.RUnlock()

	// Acquire write lock for creating new informer
	r.resourceInformersMu.Lock()
	defer r.resourceInformersMu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have created it)
	if informer, ok := r.resourceInformers[informerKey]; ok {
		return informer, nil
	}

	// Resolve GVR through the RESTMapper-backed resolver (handles irregular
	// plurals/CRDs; pluralization remains an uncached last-resort fallback).
	sample := &unstructured.Unstructured{}
	sample.SetAPIVersion(policy.Spec.TargetResource.APIVersion)
	sample.SetKind(policy.Spec.TargetResource.Kind)
	gvr, err := r.gvrResolver.ResolveGVR(sample)
	if err != nil {
		return nil, fmt.Errorf("invalid target resource: %w", err)
	}

	// Normalize namespace for informer creation
	namespace := normalizeNamespace(policy.Spec.TargetResource.Namespace)

	// Get configured interval
	interval := DefaultCleanupInterval
	if r.config != nil {
		interval = r.config.CleanupInterval
	}

	// Shared observation infrastructure: ONE informer per target
	// (apiVersion/kind/namespace), deliberately UNFILTERED by policy label
	// selectors. Policy-specific selection (labels, fields, conditions, TTL)
	// is applied per-policy at evaluation time. Baking a policy's selector
	// into the shared informer would leak policy semantics across policies
	// that target the same GVR (observed as a dry-run policy suppressing a
	// later destructive policy's deletions).
	if informer, exists := r.resourceInformers[informerKey]; exists {
		return informer, nil
	}

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		r.dynamicClient,
		interval,
		namespace,
		nil,
	)

	// Create informer
	createdInformer := factory.ForResource(gvr).Informer()

	// Store informer and factory
	r.resourceInformers[informerKey] = createdInformer
	r.resourceInformerFactories[informerKey] = factory

	// Update metrics
	recordInformerCount(len(r.resourceInformers))

	// Start informer factory
	factory.Start(ctx.Done())

	// Wait for cache sync with timeout
	syncCtx, syncCancel := context.WithTimeout(ctx, DefaultCacheSyncTimeout)
	defer syncCancel()

	if !cache.WaitForCacheSync(syncCtx.Done(), createdInformer.HasSynced) {
		// Clean up on failure
		delete(r.resourceInformers, informerKey)
		delete(r.resourceInformerFactories, informerKey)
		if syncCtx.Err() != nil {
			return nil, fmt.Errorf("resource informer cache sync timed out: %w", syncCtx.Err())
		}
		return nil, fmt.Errorf("%w", ErrResourceInformerCacheSyncFailed)
	}

	// Use struct logger to avoid allocations
	r.logger.Debug("Created resource informer for policy", sdklog.Operation("get_or_create_informer"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)), sdklog.String("uid", string(policy.UID)))
	return createdInformer, nil
}

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

// getBatchSize returns the batch size for a policy.
func (r *PolicyReconciler) getBatchSize(policy *v1alpha1.ZenCleanerPolicy) int {
	return resolveBatchSize(policy, r.config)
}

// deleteBatch deletes a batch of resources.
// Returns the number of successfully deleted resources and any errors encountered.
func (r *PolicyReconciler) deleteBatch(
	ctx context.Context,
	batch []*unstructured.Unstructured,
	policy *v1alpha1.ZenCleanerPolicy,
	rateLimiter *ratelimiter.RateLimiter,
	reasons map[string]string,
) (int64, []error) {
	return deleteBatchShared(ctx, batch, policy, rateLimiter, reasons, r)
}

// DeleteResourceWithBackoff deletes a resource with exponential backoff (implements BatchDeleter).
func (r *PolicyReconciler) DeleteResourceWithBackoff(ctx context.Context, resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter) error {
	return r.deleteResourceWithBackoff(ctx, resource, policy, rateLimiter)
}

// GetEventRecorder returns the event recorder (implements BatchDeleter).
func (r *PolicyReconciler) GetEventRecorder() *EventRecorder {
	return r.eventRecorder
}

// GetStatusUpdater returns the status updater (for testing).
func (r *PolicyReconciler) GetStatusUpdater() *StatusUpdater {
	return r.statusUpdater
}

// GetLogger returns the logger (for testing).
func (r *PolicyReconciler) GetLogger() *sdklog.Logger {
	return r.logger
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

// deleteResourceWithBackoff deletes a resource with exponential backoff retry logic.
func (r *PolicyReconciler) deleteResourceWithBackoff(ctx context.Context, resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter) error {
	return deleteResourceWithBackoffShared(ctx, resource, policy, rateLimiter, r, nil)
}

// DeleteResourceWithContext deletes a resource with context (implements ResourceDeleterWithContext).
func (r *PolicyReconciler) DeleteResourceWithContext(ctx context.Context, resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter) error {
	return r.deleteResource(ctx, resource, policy, rateLimiter)
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

// shouldRecreateInformer checks if policy spec changed in a way that requires informer recreation.
func (r *PolicyReconciler) shouldRecreateInformer(policy *v1alpha1.ZenCleanerPolicy) bool {
	r.policySpecsMu.RLock()
	defer r.policySpecsMu.RUnlock()

	oldSpec, exists := r.policySpecs[policy.UID]
	if !exists {
		// First time seeing this policy, no need to recreate
		return false
	}

	// Compare key fields that affect informer creation
	newSpec := policy.Spec.TargetResource
	oldTarget := oldSpec.TargetResource

	if oldTarget.APIVersion != newSpec.APIVersion ||
		oldTarget.Kind != newSpec.Kind ||
		oldTarget.Namespace != newSpec.Namespace ||
		!labelSelectorsEqual(oldTarget.LabelSelector, newSpec.LabelSelector) {
		return true
	}

	return false
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
	r.logger.Info("Cleaning up resources for policy", sdklog.Operation("cleanup_policy_resources"), sdklog.String("policy", nn.Namespace+"/"+nn.Name), sdklog.String("uid", string(uid)))

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
		r.logger.Debug("Cleaned up rate limiter for policy", sdklog.Operation("cleanup_rate_limiter"), sdklog.String("uid", string(policyUID)))
	}

	// Update metrics
	recordRateLimiterCount(len(r.rateLimiters))
}

// recordPolicyPhaseMetrics records metrics for policy phases.
// Uses controller-runtime cache to list all policies.
func (r *PolicyReconciler) recordPolicyPhaseMetrics(ctx context.Context) {
	// List all policies using the client cache
	policyList := &v1alpha1.ZenCleanerPolicyList{}
	if err := r.List(ctx, policyList); err != nil {
		// Use struct logger to avoid allocations
		r.logger.Debug("Failed to list policies for metrics", sdklog.Operation("record_policy_phase_metrics"), sdklog.Error(err))
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

// SetupWithManager sets up the controller with the Manager.
func (r *PolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ZenCleanerPolicy{}).
		Complete(r)
}
