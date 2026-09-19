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

package testing

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	sdklog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/controller"
)

// TestPolicyEvaluationService_EvaluatePolicy demonstrates testing with interfaces and mocks.
func TestPolicyEvaluationService_EvaluatePolicy(t *testing.T) {
	// Create test resources
	now := time.Now()
	expiredResource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			k8sKeyAPIVersion: k8sAPIV1,
			k8sKeyKind:       k8sKindConfigMap,
			k8sKeyMetadata: map[string]interface{}{
				k8sKeyName:       "expired-cm",
				k8sKeyNamespace:  k8sNSDefault,
				k8sKeyUID:        "expired-uid",
				k8sKeyCreationTS: metav1.NewTime(now.Add(-2 * time.Hour)).Format(time.RFC3339),
			},
		},
	}

	validResource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			k8sKeyAPIVersion: k8sAPIV1,
			k8sKeyKind:       k8sKindConfigMap,
			k8sKeyMetadata: map[string]interface{}{
				k8sKeyName:       "valid-cm",
				k8sKeyNamespace:  k8sNSDefault,
				k8sKeyUID:        "valid-uid",
				k8sKeyCreationTS: metav1.NewTime(now.Add(-30 * time.Minute)).Format(time.RFC3339),
			},
		},
	}

	// Create test policy
	policy := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sPolicyNameTest,
			Namespace: k8sNSDefault,
			UID:       types.UID("policy-uid"),
		},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: k8sAPIV1,
				Kind:       k8sKindConfigMap,
				Namespace:  "default",
			},
			TTL: v1alpha1.TTLSpec{
				SecondsAfterCreation: func() *int64 { v := int64(3600); return &v }(), // 1 hour
			},
		},
	}

	// Create mocks - this is MUCH simpler than setting up real Kubernetes clients!
	mockLister := NewMockResourceLister()
	gvr := schema.GroupVersionResource{Group: "", Version: k8sAPIV1, Resource: k8sResConfigMaps}
	mockLister.SetResources(gvr, "default", []*unstructured.Unstructured{expiredResource, validResource})

	mockSelectorMatcher := NewMockSelectorMatcher()
	mockSelectorMatcher.SetMatch(expiredResource, true)
	mockSelectorMatcher.SetMatch(validResource, true)

	mockConditionMatcher := NewMockConditionMatcher()
	mockConditionMatcher.SetMeetsConditions(expiredResource, true)
	mockConditionMatcher.SetMeetsConditions(validResource, true)

	mockRateLimiter := NewMockRateLimiterProvider()

	mockDeleter := NewMockBatchDeleterCore()
	mockDeleter.SetDeleteResult(expiredResource, nil) // Success

	// Create service with mocks
	service := controller.NewPolicyEvaluationService(
		mockLister,
		mockSelectorMatcher,
		mockConditionMatcher,
		nil, // TTLCalculator (using shared function for now)
		mockRateLimiter,
		mockDeleter,
		nil, // StatusUpdater (not needed for this test)
		nil, // EventRecorder (not needed for this test)
		nil,
		sdklog.NewLogger("zen-cleaner"),
	)

	// Evaluate policy - this is now testable without complex setup!
	ctx := context.Background()
	err := service.EvaluatePolicy(ctx, policy)
	if err != nil {
		t.Fatalf("EvaluatePolicy failed: %v", err)
	}
}

// TestPolicyEvaluationService_EvaluatePolicy_EmptyResources tests with no resources.
func TestPolicyEvaluationService_EvaluatePolicy_EmptyResources(t *testing.T) {
	policy := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sPolicyNameTest,
			Namespace: k8sNSDefault,
			UID:       types.UID("policy-uid"),
		},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: k8sAPIV1,
				Kind:       k8sKindConfigMap,
			},
			TTL: v1alpha1.TTLSpec{
				SecondsAfterCreation: func() *int64 { v := int64(3600); return &v }(),
			},
		},
	}

	mockLister := NewMockResourceLister()
	mockSelectorMatcher := NewMockSelectorMatcher()
	mockConditionMatcher := NewMockConditionMatcher()
	mockRateLimiter := NewMockRateLimiterProvider()
	mockDeleter := NewMockBatchDeleterCore()

	service := controller.NewPolicyEvaluationService(
		mockLister,
		mockSelectorMatcher,
		mockConditionMatcher,
		nil,
		mockRateLimiter,
		mockDeleter,
		nil,
		nil,
		nil,
		sdklog.NewLogger("zen-cleaner"),
	)

	ctx := context.Background()
	err := service.EvaluatePolicy(ctx, policy)
	if err != nil {
		t.Fatalf("EvaluatePolicy failed: %v", err)
	}
}

// TestPolicyEvaluationService_EvaluatePolicy_ContextCanceled tests context cancellation.
func TestPolicyEvaluationService_EvaluatePolicy_ContextCanceled(t *testing.T) {
	policy := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sPolicyNameTest,
			Namespace: k8sNSDefault,
			UID:       types.UID("policy-uid"),
		},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: k8sAPIV1,
				Kind:       k8sKindConfigMap,
			},
			TTL: v1alpha1.TTLSpec{
				SecondsAfterCreation: func() *int64 { v := int64(3600); return &v }(),
			},
		},
	}

	mockLister := NewMockResourceLister()
	mockSelectorMatcher := NewMockSelectorMatcher()
	mockConditionMatcher := NewMockConditionMatcher()
	mockRateLimiter := NewMockRateLimiterProvider()
	mockDeleter := NewMockBatchDeleterCore()

	service := controller.NewPolicyEvaluationService(
		mockLister,
		mockSelectorMatcher,
		mockConditionMatcher,
		nil,
		mockRateLimiter,
		mockDeleter,
		nil,
		nil,
		nil,
		sdklog.NewLogger("zen-cleaner"),
	)

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := service.EvaluatePolicy(ctx, policy)
	if err != nil {
		t.Fatalf("EvaluatePolicy should handle canceled context gracefully: %v", err)
	}
}
