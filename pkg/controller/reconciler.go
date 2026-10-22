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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cleanerlog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
	"github.com/zenmesh/zen-cleaner/pkg/safety"
	"github.com/zenmesh/zen-cleaner/pkg/validation"
)

// PolicyReconciler reconciles ZenCleanerPolicy resources.
// It implements the controller-runtime Reconciler interface.
type PolicyReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	dynamicClient dynamic.Interface

	// Controller configuration.
	config *config.ControllerConfig

	// safetyGate is the single destructive-decision choke point
	// (SUPPORT2-033 §2). Nil in tests without a gate; the production
	// constructor always sets it.
	safetyGate *safety.Gate

	// policyIntegrity is the git-gitops integrity overlay (the declared
	// digest + the optional ed25519 anchor). Nil = the overlay OFF (the
	// drift condition still receipts, visibility-only).
	policyIntegrity *PolicyIntegrityVerifier

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
	logger *cleanerlog.Logger

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

	// SUPPORT2-003: discovery client for bounded target-capability
	// validation before informer creation (§4).
	discoveryClient discovery.DiscoveryInterface

	// SUPPORT2-003: cached target capability probes (bounded TTL) and
	// consecutive-failure counters driving the bounded backoff (§7).
	targetProbes     map[string]targetProbeEntry
	targetProbeMu    sync.RWMutex
	targetFailures   map[types.UID]int
	targetFailuresMu sync.RWMutex
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
		Client:         kubeClient,
		Scheme:         scheme,
		dynamicClient:  dynamicClient,
		targetProbes:   map[string]targetProbeEntry{},
		targetFailures: map[types.UID]int{},
		config:         cfg,
		safetyGate: safety.NewGate(safety.Config{
			ProtectedNamespaces: cfg.ProtectedNamespaces,
			OwnNamespace:        cfg.OwnNamespace,
			DeletesDisabled:     !cfg.DeleteEnabled,
		}),
		shouldReconcile:           func() bool { return true }, // Default: always reconcile
		resourceInformers:         make(map[string]cache.SharedInformer),
		resourceInformerFactories: make(map[string]dynamicinformer.DynamicSharedInformerFactory),
		rateLimiters:              make(map[types.UID]*ratelimiter.RateLimiter),
		policyUIDs:                make(map[types.NamespacedName]types.UID),
		policySpecs:               make(map[types.UID]*v1alpha1.ZenCleanerPolicySpec),
		statusUpdater:             statusUpdater,
		eventRecorder:             eventRecorder,
		logger:                    cleanerlog.NewLogger("zen-cleaner"),
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
		Client:         kubeClient,
		Scheme:         scheme,
		dynamicClient:  dynamicClient,
		targetProbes:   map[string]targetProbeEntry{},
		targetFailures: map[types.UID]int{},
		config:         cfg,
		safetyGate: safety.NewGate(safety.Config{
			ProtectedNamespaces: cfg.ProtectedNamespaces,
			OwnNamespace:        cfg.OwnNamespace,
			DeletesDisabled:     !cfg.DeleteEnabled,
		}),
		shouldReconcile:           func() bool { return true }, // Always true (Manager handles leader election)
		resourceInformers:         make(map[string]cache.SharedInformer),
		resourceInformerFactories: make(map[string]dynamicinformer.DynamicSharedInformerFactory),
		rateLimiters:              make(map[types.UID]*ratelimiter.RateLimiter),
		policyUIDs:                make(map[types.NamespacedName]types.UID),
		policySpecs:               make(map[types.UID]*v1alpha1.ZenCleanerPolicySpec),
		statusUpdater:             statusUpdater,
		eventRecorder:             eventRecorder,
		logger:                    cleanerlog.NewLogger("zen-cleaner"),
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

	// SUPPORT2-033 §3: reconcile-time policy validation. Admission is the
	// first line, but policies created while the webhook was down would
	// otherwise bypass every rule; the runtime path fails closed instead.
	// A validated policy is also checked against the policy-target safety
	// laws (protected namespaces/kinds, broad scope).
	if r.safetyGate != nil {
		invalid := validation.ValidatePolicy(policy)
		if invalid == nil {
			if _, refusal := r.safetyGate.EvaluatePolicyTarget(policy.Spec.TargetResource); refusal != "" {
				invalid = fmt.Errorf("%w", stderrors.New("policy target refused: "+refusal))
			}
		}
		if invalid != nil {
			RecordReconcileOutcome("invalid_policy")
			RecordSafetyRefusal(safety.ReasonPolicyInvalid)
			r.logger.Error(invalid, "Policy failed runtime validation; no cleanup will run",
				cleanerlog.Operation("reconcile"),
				cleanerlog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)),
				cleanerlog.ErrorCode("POLICY_INVALID"))
			if r.statusUpdater != nil {
				policy.Status.Phase = "Error"
				r.updateConditionReceipt(ctx, policy, "Ready", "PolicyInvalid", invalid.Error(), true)
			}
			return ctrl.Result{RequeueAfter: time.Hour}, nil
		}
	}

	// The git-gitops integrity overlay (the declared digest + the
	// optional trust anchor): drift is always receipted as a condition;
	// with the anchor configured, an unsigned or drifted policy is
	// policy_invalid — the mutated intent never executes.
	if r.policyIntegrity != nil {
		verdict := r.policyIntegrity.CheckIntegrity(policy)
		switch {
		case verdict.Refuses:
			RecordReconcileOutcome("invalid_policy")
			RecordSafetyRefusal(safety.ReasonPolicyInvalid)
			r.logger.Error(nil, "Policy integrity refused; no cleanup will run",
				cleanerlog.Operation("reconcile"),
				cleanerlog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)),
				cleanerlog.ErrorCode("POLICY_INTEGRITY_REFUSED"),
				cleanerlog.String("detail", verdict.Detail))
			if r.statusUpdater != nil {
				policy.Status.Phase = "Error"
				r.updateConditionReceipt(ctx, policy, "Ready", "PolicyIntegrityRefused", verdict.Detail, true)
			}
			return ctrl.Result{RequeueAfter: time.Hour}, nil
		case verdict.Drifted:
			if r.statusUpdater != nil {
				r.updateConditionReceipt(ctx, policy, "PolicyDrifted", "DeclaredDigestMismatch", verdict.Detail, false)
			}
			r.logger.Info("Policy drift receipted (visibility)",
				cleanerlog.Operation("reconcile"),
				cleanerlog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)),
				cleanerlog.String("detail", verdict.Detail))
		default:
			if r.statusUpdater != nil {
				r.updateConditionReceipt(ctx, policy, "PolicyDrifted", "InSync", verdict.Detail, false)
			}
		}
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
		RecordReconcileOutcome("error")
		RecordAPIError(err)
		return r.handleEvaluationError(err, policy)
	}

	// Record policy phase metrics periodically
	r.recordPolicyPhaseMetrics(ctx)

	RecordReconcileOutcome("success")

	// Determine requeue interval based on policy evaluation interval or default
	requeueAfter := r.getRequeueIntervalForPolicy(policy)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// getRequeueInterval returns the controller-level requeue interval (used
// where no specific policy applies, e.g. paused-skip requeue).
func (r *PolicyReconciler) getRequeueInterval() time.Duration {
	interval := DefaultCleanupInterval
	if r.config != nil {
		interval = r.config.CleanupInterval
	}
	return interval
}

// getRequeueIntervalForPolicy returns the requeue interval for a specific policy.
// Derives from the SHARED interval law (EffectiveEvaluationInterval) so the
// requeue schedule and the status.nextGCRun promise can never diverge.
func (r *PolicyReconciler) getRequeueIntervalForPolicy(policy *v1alpha1.ZenCleanerPolicy) time.Duration {
	interval := DefaultCleanupInterval
	if r.config != nil {
		interval = r.config.CleanupInterval
	}
	return EffectiveEvaluationInterval(policy, interval)
}

// GetLogger returns the logger (for testing).
func (r *PolicyReconciler) GetLogger() *cleanerlog.Logger {
	return r.logger
}

// SetupWithManager sets up the controller with the Manager.
func (r *PolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ZenCleanerPolicy{}).
		Complete(r)
}
