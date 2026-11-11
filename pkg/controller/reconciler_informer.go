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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cleanerlog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

// SetDiscoveryClient wires the discovery client used for bounded
// target-capability validation (SUPPORT2-003 §4). Optional: without it the
// capability gate still classifies via the list probe.
func (r *PolicyReconciler) SetDiscoveryClient(dc discovery.DiscoveryInterface) {
	r.discoveryClient = dc
}

// SetAPIReader wires the UNCACHED reader (manager.GetAPIReader()) used by
// the delete-time policy-freshness check: the confirmation that the
// governing policy is still the live one immediately before an irreversible
// DELETE must bypass the informer cache. Production wiring in
// cmd/zen-cleaner/main.go always sets it; nil (tests) means the check is
// skipped when no fallback reader is available either.
func (r *PolicyReconciler) SetAPIReader(reader client.Reader) {
	r.apiReader = reader
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

	// SUPPORT2-003 §4: bounded capability validation BEFORE informer
	// creation. Unsupported/unauthorized targets are SAFE failures: no
	// reflector is started (no hot loop), the policy carries a classified
	// condition, and evaluation retries with bounded backoff. Recovery is
	// automatic when the target becomes available/permitted.
	if err := r.validateTargetOrClassify(ctx, policy); err != nil {
		return nil, err
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

	// SUPPORT2-033 §5 (selector fail-closed): if the policy supplies a label
	// selector, it must convert cleanly BEFORE any informer/list/watch is
	// created. No informer is created or replaced for a policy whose selector
	// cannot be converted — the policy is surfaced as an error instead of
	// observing a broader scope than declared.
	if _, selErr := buildLabelSelectorFilter(policy); selErr != nil {
		return nil, fmt.Errorf("policy %s/%s: %w", policy.Namespace, policy.Name, selErr)
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
	r.logger.Debug("Created resource informer for policy", cleanerlog.Operation("get_or_create_informer"), cleanerlog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)), cleanerlog.String("uid", string(policy.UID)))
	return createdInformer, nil
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
