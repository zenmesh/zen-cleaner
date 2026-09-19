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
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
)

// TestPolicyReconciler_getRequeueInterval_Coverage tests getRequeueInterval for coverage.
func TestPolicyReconciler_getRequeueInterval_Coverage(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	statusUpdater := NewStatusUpdater(dynamicClient)
	eventRecorder := NewEventRecorder(fake.NewSimpleClientset())

	// Test with default config
	reconciler := NewPolicyReconcilerWithRESTMapper(
		fakeClient,
		scheme,
		dynamicClient,
		nil,
		statusUpdater,
		eventRecorder,
		config.NewControllerConfig(),
	)

	interval := reconciler.getRequeueInterval()
	if interval <= 0 {
		t.Errorf("getRequeueInterval() returned invalid interval: %v", interval)
	}

	// Test with custom config
	customConfig := config.NewControllerConfig().WithCleanupInterval(2 * time.Minute)
	reconciler2 := NewPolicyReconcilerWithRESTMapper(
		fakeClient,
		scheme,
		dynamicClient,
		nil,
		statusUpdater,
		eventRecorder,
		customConfig,
	)

	interval2 := reconciler2.getRequeueInterval()
	if interval2 != 2*time.Minute {
		t.Errorf("getRequeueInterval() = %v, want %v", interval2, 2*time.Minute)
	}
}

// TestPolicyReconciler_getRequeueIntervalForPolicy tests getRequeueIntervalForPolicy.
func TestPolicyReconciler_getRequeueIntervalForPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	statusUpdater := NewStatusUpdater(dynamicClient)
	eventRecorder := NewEventRecorder(fake.NewSimpleClientset())

	reconciler := NewPolicyReconcilerWithRESTMapper(
		fakeClient,
		scheme,
		dynamicClient,
		nil,
		statusUpdater,
		eventRecorder,
		config.NewControllerConfig(),
	)

	// Test with policy-specific interval
	policyInterval := metav1.Duration{Duration: 5 * time.Minute}
	policy := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			EvaluationInterval: &policyInterval,
		},
	}

	interval := reconciler.getRequeueIntervalForPolicy(policy)
	if interval != 5*time.Minute {
		t.Errorf("getRequeueIntervalForPolicy() = %v, want %v", interval, 5*time.Minute)
	}

	// Test without policy-specific interval (should use default)
	policy2 := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{},
	}

	interval2 := reconciler.getRequeueIntervalForPolicy(policy2)
	if interval2 <= 0 {
		t.Errorf("getRequeueIntervalForPolicy() returned invalid interval: %v", interval2)
	}
}

// TestPolicyReconciler_trackPolicyUID_Coverage tests trackPolicyUID for coverage.
func TestPolicyReconciler_trackPolicyUID_Coverage(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	statusUpdater := NewStatusUpdater(dynamicClient)
	eventRecorder := NewEventRecorder(fake.NewSimpleClientset())

	reconciler := NewPolicyReconcilerWithRESTMapper(
		fakeClient,
		scheme,
		dynamicClient,
		nil,
		statusUpdater,
		eventRecorder,
		config.NewControllerConfig(),
	)

	nn := types.NamespacedName{Name: "test-policy", Namespace: "default"}
	uid := types.UID("test-uid")

	reconciler.trackPolicyUID(nn, uid)

	reconciler.policyUIDsMu.RLock()
	trackedUID, exists := reconciler.policyUIDs[nn]
	reconciler.policyUIDsMu.RUnlock()

	if !exists {
		t.Error("trackPolicyUID() did not track UID")
	}
	if trackedUID != uid {
		t.Errorf("trackPolicyUID() tracked UID = %v, want %v", trackedUID, uid)
	}
}

// TestPolicyReconciler_trackPolicySpec tests trackPolicySpec.
func TestPolicyReconciler_trackPolicySpec(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	statusUpdater := NewStatusUpdater(dynamicClient)
	eventRecorder := NewEventRecorder(fake.NewSimpleClientset())

	reconciler := NewPolicyReconcilerWithRESTMapper(
		fakeClient,
		scheme,
		dynamicClient,
		nil,
		statusUpdater,
		eventRecorder,
		config.NewControllerConfig(),
	)

	uid := types.UID("test-uid")
	spec := &v1alpha1.ZenCleanerPolicySpec{
		TargetResource: v1alpha1.TargetResourceSpec{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
	}

	reconciler.trackPolicySpec(uid, spec)

	reconciler.policySpecsMu.RLock()
	trackedSpec, exists := reconciler.policySpecs[uid]
	reconciler.policySpecsMu.RUnlock()

	if !exists {
		t.Error("trackPolicySpec() did not track spec")
	}
	if trackedSpec.TargetResource.Kind != spec.TargetResource.Kind {
		t.Errorf("trackPolicySpec() tracked spec = %v, want %v", trackedSpec, spec)
	}
}

// TestPolicyReconciler_getOrCreateRateLimiter tests getOrCreateRateLimiter.
func TestPolicyReconciler_getOrCreateRateLimiter(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	statusUpdater := NewStatusUpdater(dynamicClient)
	eventRecorder := NewEventRecorder(fake.NewSimpleClientset())

	reconciler := NewPolicyReconcilerWithRESTMapper(
		fakeClient,
		scheme,
		dynamicClient,
		nil,
		statusUpdater,
		eventRecorder,
		config.NewControllerConfig(),
	)

	policy := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID("test-uid"),
		},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			Behavior: v1alpha1.BehaviorSpec{
				MaxDeletionsPerSecond: 10,
			},
		},
	}

	// First call should create rate limiter
	limiter1 := reconciler.getOrCreateRateLimiter(policy)
	if limiter1 == nil {
		t.Error("getOrCreateRateLimiter() returned nil")
	}

	// Second call should return same rate limiter
	limiter2 := reconciler.getOrCreateRateLimiter(policy)
	if limiter1 != limiter2 {
		t.Error("getOrCreateRateLimiter() returned different rate limiter on second call")
	}
}

// TestPolicyReconciler_getBatchSize tests getBatchSize.
func TestPolicyReconciler_getBatchSize(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	statusUpdater := NewStatusUpdater(dynamicClient)
	eventRecorder := NewEventRecorder(fake.NewSimpleClientset())

	reconciler := NewPolicyReconcilerWithRESTMapper(
		fakeClient,
		scheme,
		dynamicClient,
		nil,
		statusUpdater,
		eventRecorder,
		config.NewControllerConfig(),
	)

	// Test with policy-specific batch size
	policy := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			Behavior: v1alpha1.BehaviorSpec{
				BatchSize: 100,
			},
		},
	}

	batchSize := reconciler.getBatchSize(policy)
	if batchSize != 100 {
		t.Errorf("getBatchSize() = %d, want 100", batchSize)
	}

	// Test without policy-specific batch size (should use default)
	policy2 := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{},
	}

	batchSize2 := reconciler.getBatchSize(policy2)
	if batchSize2 <= 0 {
		t.Errorf("getBatchSize() returned invalid batch size: %d", batchSize2)
	}
}

// TestPolicyReconciler_cleanupPolicyResources tests cleanupPolicyResources.
func TestPolicyReconciler_cleanupPolicyResources(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	statusUpdater := NewStatusUpdater(dynamicClient)
	eventRecorder := NewEventRecorder(fake.NewSimpleClientset())

	reconciler := NewPolicyReconcilerWithRESTMapper(
		fakeClient,
		scheme,
		dynamicClient,
		nil,
		statusUpdater,
		eventRecorder,
		config.NewControllerConfig(),
	)

	nn := types.NamespacedName{Name: "test-policy", Namespace: "default"}
	uid := types.UID("test-uid")

	// Track policy UID
	reconciler.trackPolicyUID(nn, uid)

	// Create a rate limiter for this policy
	policy := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{
			UID: uid,
		},
	}
	_ = reconciler.getOrCreateRateLimiter(policy)

	// Cleanup should remove tracked UID and rate limiter
	reconciler.cleanupPolicyResources(nn)

	reconciler.policyUIDsMu.RLock()
	_, exists := reconciler.policyUIDs[nn]
	reconciler.policyUIDsMu.RUnlock()

	if exists {
		t.Error("cleanupPolicyResources() did not remove tracked UID")
	}

	reconciler.rateLimitersMu.RLock()
	_, exists = reconciler.rateLimiters[uid]
	reconciler.rateLimitersMu.RUnlock()

	if exists {
		t.Error("cleanupPolicyResources() did not remove rate limiter")
	}
}
