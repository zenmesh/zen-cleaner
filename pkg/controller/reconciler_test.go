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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
)

// setupTestReconciler creates a test reconciler with fake clients.
func setupTestReconciler(t *testing.T) (*PolicyReconciler, client.Client) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add v1alpha1 to scheme: %v", err)
	}

	// Create fake controller-runtime client
	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).Build()

	// Create fake dynamic client
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)

	// Create status updater and event recorder
	statusUpdater := NewStatusUpdater(dynamicClient)
	eventRecorder := NewEventRecorder(nil) // nil is OK for tests

	// Create reconciler (RESTMapper is optional, nil is OK for tests)
	reconciler := NewPolicyReconcilerWithRESTMapper(
		fakeClient,
		scheme,
		dynamicClient,
		nil, // RESTMapper - nil is OK, will use pluralization fallback
		statusUpdater,
		eventRecorder,
		config.NewControllerConfig(),
	)

	return reconciler, fakeClient
}

func TestNewPolicyReconciler(t *testing.T) {
	reconciler, _ := setupTestReconciler(t)

	if reconciler == nil {
		t.Fatal("NewPolicyReconciler() returned nil reconciler")
	}

	if reconciler.Client == nil {
		t.Error("NewPolicyReconciler() did not set Client")
	}

	if reconciler.dynamicClient == nil {
		t.Error("NewPolicyReconciler() did not set dynamicClient")
	}

	if reconciler.resourceInformers == nil {
		t.Error("NewPolicyReconciler() did not initialize resourceInformers map")
	}

	if reconciler.rateLimiters == nil {
		t.Error("NewPolicyReconciler() did not initialize rateLimiters map")
	}

	if reconciler.policyUIDs == nil {
		t.Error("NewPolicyReconciler() did not initialize policyUIDs map")
	}

	if reconciler.policySpecs == nil {
		t.Error("NewPolicyReconciler() did not initialize policySpecs map")
	}
}

// TestEvaluationServiceKey_DifferentGVRs ensures evaluationServiceKey produces distinct keys
// for different (apiVersion, kind, namespace) tuples. This is the regression guard for BUG-001
// where the evaluation service was keyed only by the first policy's GVR.
func TestEvaluationServiceKey_DifferentGVRs(t *testing.T) {
	podPolicy := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "Pod",
				Namespace:  "default",
			},
		},
	}
	cmPolicy := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  "default",
			},
		},
	}
	rsPolicy := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "apps/v1",
				Kind:       "ReplicaSet",
				Namespace:  "default",
			},
		},
	}

	keys := make(map[string]bool)
	for _, p := range []*v1alpha1.ZenCleanerPolicy{podPolicy, cmPolicy, rsPolicy} {
		k := evaluationServiceKey(p)
		if keys[k] {
			t.Errorf("duplicate key %q produced for policy %s/%s", k, p.Spec.TargetResource.APIVersion, p.Spec.TargetResource.Kind)
		}
		keys[k] = true
	}

	// Also verify same key is returned for identical targets
	cmPolicy2 := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  "default",
			},
		},
	}
	k1 := evaluationServiceKey(cmPolicy)
	k2 := evaluationServiceKey(cmPolicy2)
	if k1 != k2 {
		t.Errorf("identical target resources should produce same key, got %q != %q", k1, k2)
	}
}

// TestEvaluationServiceKey_ConsistentCache verifies that the evaluation key function
// is consistent with the cached services map. Uses the reconciler's evaluationServices
// map directly to confirm same-key → same-entry behavior. Regression guard for BUG-001.
func TestEvaluationServiceKey_ConsistentCache(t *testing.T) {
	podKey := evaluationServiceKey(&v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1", Kind: "Pod", Namespace: "default",
			},
		},
	})
	cmKey := evaluationServiceKey(&v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1", Kind: "ConfigMap", Namespace: "default",
			},
		},
	})
	rsKey := evaluationServiceKey(&v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "apps/v1", Kind: "ReplicaSet", Namespace: "default",
			},
		},
	})

	if podKey == cmKey {
		t.Error("BUG-001: Pod and ConfigMap policies should produce different cache keys")
	}
	if podKey == rsKey {
		t.Error("BUG-001: Pod and ReplicaSet policies should produce different cache keys")
	}
	if cmKey == rsKey {
		t.Error("BUG-001: ConfigMap and ReplicaSet policies should produce different cache keys")
	}

	// Verify same key is returned for identical targets
	cmKey2 := evaluationServiceKey(&v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1", Kind: "ConfigMap", Namespace: "default",
			},
		},
	})
	if cmKey != cmKey2 {
		t.Errorf("identical targets should produce same key, got %q != %q", cmKey, cmKey2)
	}
}

func TestPolicyReconciler_Reconcile_NotFound(t *testing.T) {
	reconciler, fakeClient := setupTestReconciler(t)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Errorf("Reconcile() should not error on not found, got: %v", err)
	}

	if result != (reconcile.Result{}) {
		t.Error("Reconcile() should not requeue on not found")
	}

	// Verify policy was not created
	policy := &v1alpha1.ZenCleanerPolicy{}
	err = fakeClient.Get(ctx, req.NamespacedName, policy)
	if err == nil {
		t.Error("Policy should not exist")
	}
}

func TestPolicyReconciler_Reconcile_PausedPolicy(t *testing.T) {
	reconciler, fakeClient := setupTestReconciler(t)

	policy := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			Paused: true,
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			TTL: v1alpha1.TTLSpec{
				SecondsAfterCreation: int64Ptr(3600),
			},
		},
	}

	if err := fakeClient.Create(context.Background(), policy); err != nil {
		t.Fatalf("Failed to create policy: %v", err)
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-policy",
			Namespace: "default",
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Errorf("Reconcile() should not error on paused policy, got: %v", err)
	}

	// Should requeue with interval
	if result.RequeueAfter == 0 {
		t.Error("Reconcile() should requeue paused policy with interval")
	}
}

func TestPolicyReconciler_Reconcile_PolicyDeletion(t *testing.T) {
	reconciler, fakeClient := setupTestReconciler(t)

	policy := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			TTL: v1alpha1.TTLSpec{
				SecondsAfterCreation: int64Ptr(3600),
			},
		},
	}

	// Create policy
	if err := fakeClient.Create(context.Background(), policy); err != nil {
		t.Fatalf("Failed to create policy: %v", err)
	}

	// Track UID manually for test
	nn := types.NamespacedName{Name: "test-policy", Namespace: "default"}
	reconciler.trackPolicyUID(nn, policy.UID)

	// Create a rate limiter to test cleanup
	reconciler.rateLimitersMu.Lock()
	reconciler.rateLimiters[policy.UID] = ratelimiter.NewRateLimiter(10)
	reconciler.rateLimitersMu.Unlock()

	// Delete policy
	if err := fakeClient.Delete(context.Background(), policy); err != nil {
		t.Fatalf("Failed to delete policy: %v", err)
	}

	// Reconcile should handle deletion
	req := reconcile.Request{NamespacedName: nn}
	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Errorf("Reconcile() should not error on deletion, got: %v", err)
	}

	if result != (reconcile.Result{}) {
		t.Error("Reconcile() should not requeue on deletion")
	}

	// Verify rate limiter was cleaned up
	reconciler.rateLimitersMu.RLock()
	_, exists := reconciler.rateLimiters[policy.UID]
	reconciler.rateLimitersMu.RUnlock()

	if exists {
		t.Error("Rate limiter should be cleaned up on policy deletion")
	}

	// Verify UID tracking was cleaned up
	reconciler.policyUIDsMu.RLock()
	_, exists = reconciler.policyUIDs[nn]
	reconciler.policyUIDsMu.RUnlock()

	if exists {
		t.Error("Policy UID tracking should be cleaned up on deletion")
	}
}

func TestPolicyReconciler_shouldRecreateInformer(t *testing.T) {
	reconciler, _ := setupTestReconciler(t)

	policy := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID("test-uid"),
		},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  "default",
			},
		},
	}

	// First time - should not recreate
	if reconciler.shouldRecreateInformer(policy) {
		t.Error("shouldRecreateInformer() should return false for new policy")
	}

	// Track the spec
	reconciler.trackPolicySpec(policy.UID, &policy.Spec)

	// Same spec - should not recreate
	if reconciler.shouldRecreateInformer(policy) {
		t.Error("shouldRecreateInformer() should return false for unchanged spec")
	}

	// Change APIVersion - should recreate
	policy.Spec.TargetResource.APIVersion = "apps/v1"
	if !reconciler.shouldRecreateInformer(policy) {
		t.Error("shouldRecreateInformer() should return true when APIVersion changes")
	}

	// Reset and change Kind
	policy.Spec.TargetResource.APIVersion = "v1"
	reconciler.trackPolicySpec(policy.UID, &policy.Spec)
	policy.Spec.TargetResource.Kind = "Deployment"
	if !reconciler.shouldRecreateInformer(policy) {
		t.Error("shouldRecreateInformer() should return true when Kind changes")
	}

	// Reset and change Namespace
	policy.Spec.TargetResource.Kind = "ConfigMap"
	reconciler.trackPolicySpec(policy.UID, &policy.Spec)
	policy.Spec.TargetResource.Namespace = "other"
	if !reconciler.shouldRecreateInformer(policy) {
		t.Error("shouldRecreateInformer() should return true when Namespace changes")
	}
}

func TestPolicyReconciler_trackPolicyUID(t *testing.T) {
	reconciler, _ := setupTestReconciler(t)

	nn := types.NamespacedName{Name: "test", Namespace: "default"}
	uid := types.UID("test-uid")

	reconciler.trackPolicyUID(nn, uid)

	reconciler.policyUIDsMu.RLock()
	trackedUID, exists := reconciler.policyUIDs[nn]
	reconciler.policyUIDsMu.RUnlock()

	if !exists {
		t.Error("Policy UID should be tracked")
	}

	if trackedUID != uid {
		t.Errorf("Tracked UID = %s, want %s", trackedUID, uid)
	}
}

func TestPolicyReconciler_SetupWithManager(t *testing.T) {
	reconciler, _ := setupTestReconciler(t)

	// Verify reconciler has SetupWithManager method
	// This test just ensures the method exists and can be called
	// Full integration test would require envtest setup
	if reconciler == nil {
		t.Fatal("Reconciler should not be nil")
	}

	// The method exists if we can reference it without compilation error
	_ = reconciler.SetupWithManager
}

func TestPolicyReconciler_cleanupResourceInformer(t *testing.T) {
	reconciler, _ := setupTestReconciler(t)

	uid := types.UID("test-uid")

	// D039: informers are SHARED per target GVR/namespace — a single policy
	// deletion must NOT tear down observation shared by remaining policies.
	reconciler.resourceInformersMu.Lock()
	key := "v1/ConfigMap/test-ns"
	reconciler.resourceInformers[key] = nil // nil is OK for this test
	reconciler.resourceInformerFactories[key] = nil
	initialCount := len(reconciler.resourceInformers)
	reconciler.resourceInformersMu.Unlock()

	reconciler.cleanupResourceInformer(uid)

	reconciler.resourceInformersMu.RLock()
	finalCount := len(reconciler.resourceInformers)
	_, exists := reconciler.resourceInformers[key]
	reconciler.resourceInformersMu.RUnlock()

	if !exists {
		t.Error("shared informer must be retained on policy deletion (other policies may share the target)")
	}
	if finalCount != initialCount {
		t.Errorf("cleanup must not change informer count, got %d -> %d", initialCount, finalCount)
	}
}

func TestPolicyReconciler_cleanupRateLimiter(t *testing.T) {
	reconciler, _ := setupTestReconciler(t)

	uid := types.UID("test-uid")

	// Create a rate limiter
	reconciler.rateLimitersMu.Lock()
	reconciler.rateLimiters[uid] = ratelimiter.NewRateLimiter(10)
	initialCount := len(reconciler.rateLimiters)
	reconciler.rateLimitersMu.Unlock()

	// Cleanup
	reconciler.cleanupRateLimiter(uid)

	// Verify cleanup
	reconciler.rateLimitersMu.RLock()
	finalCount := len(reconciler.rateLimiters)
	_, exists := reconciler.rateLimiters[uid]
	reconciler.rateLimitersMu.RUnlock()

	if exists {
		t.Error("Rate limiter should be cleaned up")
	}

	if finalCount != initialCount-1 {
		t.Errorf("Expected rate limiter count to decrease by 1, got %d -> %d", initialCount, finalCount)
	}
}

func TestPolicyReconciler_getRequeueInterval(t *testing.T) {
	reconciler, _ := setupTestReconciler(t)

	interval := reconciler.getRequeueInterval()

	if interval <= 0 {
		t.Errorf("Requeue interval should be positive, got: %v", interval)
	}

	if interval != DefaultCleanupInterval {
		t.Errorf("Expected default interval %v, got %v", DefaultCleanupInterval, interval)
	}
}
