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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

// Static errors for adapters.
var (
	errInformerStoreNil = errors.New("informer store is nil")
)

// InformerStoreResourceLister adapts a cache.Store to ResourceLister interface.
// This allows us to use existing informer stores with the new ResourceLister interface.
type InformerStoreResourceLister struct {
	store cache.Store
}

// NewInformerStoreResourceLister creates a new InformerStoreResourceLister.
func NewInformerStoreResourceLister(store cache.Store) ResourceLister {
	return &InformerStoreResourceLister{store: store}
}

// ListResources lists all resources from the store.
func (l *InformerStoreResourceLister) ListResources(ctx context.Context, gvr schema.GroupVersionResource, namespace string) ([]*unstructured.Unstructured, error) {
	items := l.store.List()
	resources := make([]*unstructured.Unstructured, 0, len(items))

	for _, obj := range items {
		resource, ok := obj.(*unstructured.Unstructured)
		if !ok {
			continue
		}

		// Filter by namespace if specified
		if namespace != "" && namespace != "*" && resource.GetNamespace() != namespace {
			continue
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

// PolicyReconcilerAdapter adapts PolicyReconciler to provide interfaces for PolicyEvaluationService.
// This allows PolicyReconciler to use PolicyEvaluationService internally while maintaining backward compatibility.
type PolicyReconcilerAdapter struct {
	reconciler *PolicyReconciler
}

// NewPolicyReconcilerAdapter creates a new PolicyReconcilerAdapter.
func NewPolicyReconcilerAdapter(reconciler *PolicyReconciler) *PolicyReconcilerAdapter {
	return &PolicyReconcilerAdapter{reconciler: reconciler}
}

// GetResourceListerForPolicy creates a ResourceLister from the policy's informer.
func (a *PolicyReconcilerAdapter) GetResourceListerForPolicy(ctx context.Context, policy *v1alpha1.ZenCleanerPolicy) (ResourceLister, error) {
	informer, err := a.reconciler.getOrCreateResourceInformer(ctx, policy)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource informer: %w", err)
	}
	store := informer.GetStore()
	if store == nil {
		return nil, fmt.Errorf("%w for policy %s/%s", errInformerStoreNil, policy.Namespace, policy.Name)
	}
	return NewInformerStoreResourceLister(store), nil
}

// GetSelectorMatcher returns a SelectorMatcher using PolicyReconciler's implementation.
func (a *PolicyReconcilerAdapter) GetSelectorMatcher() SelectorMatcher {
	return &PolicyReconcilerSelectorMatcher{reconciler: a.reconciler}
}

// GetConditionMatcher returns a ConditionMatcher using PolicyReconciler's implementation.
func (a *PolicyReconcilerAdapter) GetConditionMatcher() ConditionMatcher {
	return &PolicyReconcilerConditionMatcher{reconciler: a.reconciler}
}

// GetRateLimiterProvider returns a RateLimiterProvider using PolicyReconciler's implementation.
func (a *PolicyReconcilerAdapter) GetRateLimiterProvider() RateLimiterProvider {
	return &PolicyReconcilerRateLimiterProvider{reconciler: a.reconciler}
}

// GetBatchDeleter returns a BatchDeleterCore using PolicyReconciler's implementation.
func (a *PolicyReconcilerAdapter) GetBatchDeleter() BatchDeleterCore {
	return &PolicyReconcilerBatchDeleter{reconciler: a.reconciler}
}

// PolicyReconcilerSelectorMatcher adapts PolicyReconciler to SelectorMatcher interface.
type PolicyReconcilerSelectorMatcher struct {
	reconciler *PolicyReconciler
}

// MatchesSelectors checks if a resource matches selectors.
func (m *PolicyReconcilerSelectorMatcher) MatchesSelectors(resource *unstructured.Unstructured, spec *v1alpha1.TargetResourceSpec) bool {
	return m.reconciler.matchesSelectors(resource, spec)
}

// PolicyReconcilerConditionMatcher adapts PolicyReconciler to ConditionMatcher interface.
type PolicyReconcilerConditionMatcher struct {
	reconciler *PolicyReconciler
}

// MeetsConditions checks if a resource meets conditions.
func (m *PolicyReconcilerConditionMatcher) MeetsConditions(resource *unstructured.Unstructured, conditions *v1alpha1.ConditionsSpec) bool {
	return m.reconciler.meetsConditions(resource, conditions)
}

// PolicyReconcilerRateLimiterProvider adapts PolicyReconciler to RateLimiterProvider interface.
type PolicyReconcilerRateLimiterProvider struct {
	reconciler *PolicyReconciler
}

// GetOrCreateRateLimiter returns a rate limiter for the policy.
func (p *PolicyReconcilerRateLimiterProvider) GetOrCreateRateLimiter(policy *v1alpha1.ZenCleanerPolicy) *ratelimiter.RateLimiter {
	return p.reconciler.getOrCreateRateLimiter(policy)
}

// PolicyReconcilerBatchDeleter adapts PolicyReconciler to BatchDeleterCore interface.
type PolicyReconcilerBatchDeleter struct {
	reconciler *PolicyReconciler
}

// DeleteBatch deletes a batch of resources.
func (d *PolicyReconcilerBatchDeleter) DeleteBatch(ctx context.Context, batch []*unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter, reasons map[string]string) (int64, []error) {
	return d.reconciler.deleteBatch(ctx, batch, policy, rateLimiter, reasons)
}
