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
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/controller"
)

// Static errors for testing.
var (
	errDeletionFailed = errors.New("deletion failed")
)

// Test constants to avoid magic numbers.
const (
	testTTLSeconds    = 3600 // 1 hour in seconds
	testResourceCount = 5
)

// TestInformerStoreResourceLister tests the InformerStoreResourceLister adapter.
func TestInformerStoreResourceLister(t *testing.T) {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)

	resources := []*unstructured.Unstructured{
		{
			Object: map[string]interface{}{
				k8sKeyAPIVersion: k8sAPIV1,
				k8sKeyKind:       k8sKindConfigMap,
				k8sKeyMetadata: map[string]interface{}{
					k8sKeyName:      "cm1",
					k8sKeyNamespace: k8sNSDefault,
				},
			},
		},
		{
			Object: map[string]interface{}{
				k8sKeyAPIVersion: k8sAPIV1,
				k8sKeyKind:       k8sKindConfigMap,
				k8sKeyMetadata: map[string]interface{}{
					k8sKeyName:      "cm2",
					k8sKeyNamespace: k8sNSTest,
				},
			},
		},
	}

	for _, r := range resources {
		_ = store.Add(r)
	}

	lister := controller.NewInformerStoreResourceLister(store)

	// Test listing all resources
	all, err := lister.ListResources(context.Background(), schema.GroupVersionResource{}, "")
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("Expected 2 resources, got %d", len(all))
	}

	// Test filtering by namespace
	defaultNS, err := lister.ListResources(context.Background(), schema.GroupVersionResource{}, "default")
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	if len(defaultNS) != 1 {
		t.Errorf("Expected 1 resource in default namespace, got %d", len(defaultNS))
	}

	// Test wildcard namespace
	wildcard, err := lister.ListResources(context.Background(), schema.GroupVersionResource{}, "*")
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	if len(wildcard) != 2 {
		t.Errorf("Expected 2 resources with wildcard, got %d", len(wildcard))
	}
}

// TestPolicyEvaluationServiceWithConditions tests policy evaluation with conditions.
func TestPolicyEvaluationServiceWithConditions(t *testing.T) {
	now := time.Now()
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			k8sKeyAPIVersion: k8sAPIV1,
			k8sKeyKind:       k8sKindConfigMap,
			k8sKeyMetadata: map[string]interface{}{
				k8sKeyName:       k8sNameTestCM,
				k8sKeyNamespace:  k8sNSDefault,
				k8sKeyUID:        k8sUIDTest,
				k8sKeyCreationTS: metav1.NewTime(now.Add(-2 * time.Hour)).Format(time.RFC3339),
				k8sKeyLabels: map[string]interface{}{
					k8sLabelKeyApp: k8sLabelValTest,
				},
			},
		},
	}

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
				SecondsAfterCreation: func() *int64 { v := int64(testTTLSeconds); return &v }(),
			},
			Conditions: &v1alpha1.ConditionsSpec{
				HasLabels: []v1alpha1.LabelCondition{
					{Key: "app", Value: "test"},
				},
			},
		},
	}

	mockLister := NewMockResourceLister()
	gvr := schema.GroupVersionResource{Group: "", Version: k8sAPIV1, Resource: k8sResConfigMaps}
	mockLister.SetResources(gvr, "default", []*unstructured.Unstructured{resource})

	mockSelectorMatcher := NewMockSelectorMatcher()
	mockSelectorMatcher.SetMatch(resource, true)

	mockConditionMatcher := NewMockConditionMatcher()
	mockConditionMatcher.SetMeetsConditions(resource, true) // Meets conditions

	mockRateLimiter := NewMockRateLimiterProvider()
	mockDeleter := NewMockBatchDeleterCore()
	mockDeleter.SetDeleteResult(resource, nil)

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
		nil,
	)

	ctx := context.Background()
	err := service.EvaluatePolicy(ctx, policy)
	if err != nil {
		t.Fatalf("EvaluatePolicy failed: %v", err)
	}
}

// TestPolicyEvaluationServiceConditionsNotMet tests when conditions are not met.
func TestPolicyEvaluationServiceConditionsNotMet(t *testing.T) {
	now := time.Now()
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			k8sKeyAPIVersion: k8sAPIV1,
			k8sKeyKind:       k8sKindConfigMap,
			k8sKeyMetadata: map[string]interface{}{
				k8sKeyName:       k8sNameTestCM,
				k8sKeyNamespace:  k8sNSDefault,
				k8sKeyUID:        k8sUIDTest,
				k8sKeyCreationTS: metav1.NewTime(now.Add(-2 * time.Hour)).Format(time.RFC3339),
			},
		},
	}

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
				SecondsAfterCreation: func() *int64 { v := int64(testTTLSeconds); return &v }(),
			},
			Conditions: &v1alpha1.ConditionsSpec{
				HasLabels: []v1alpha1.LabelCondition{
					{Key: "app", Value: "test"},
				},
			},
		},
	}

	mockLister := NewMockResourceLister()
	gvr := schema.GroupVersionResource{Group: "", Version: k8sAPIV1, Resource: k8sResConfigMaps}
	mockLister.SetResources(gvr, "default", []*unstructured.Unstructured{resource})

	mockSelectorMatcher := NewMockSelectorMatcher()
	mockSelectorMatcher.SetMatch(resource, true)

	mockConditionMatcher := NewMockConditionMatcher()
	mockConditionMatcher.SetMeetsConditions(resource, false) // Does NOT meet conditions

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
		nil,
	)

	ctx := context.Background()
	err := service.EvaluatePolicy(ctx, policy)
	if err != nil {
		t.Fatalf("EvaluatePolicy failed: %v", err)
	}
	// Resource should not be deleted because conditions are not met
}

// TestPolicyEvaluationServiceSelectorNotMatched tests when selectors don't match.
func TestPolicyEvaluationServiceSelectorNotMatched(t *testing.T) {
	now := time.Now()
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			k8sKeyAPIVersion: k8sAPIV1,
			k8sKeyKind:       k8sKindConfigMap,
			k8sKeyMetadata: map[string]interface{}{
				k8sKeyName:       k8sNameTestCM,
				k8sKeyNamespace:  k8sNSDefault,
				k8sKeyUID:        k8sUIDTest,
				k8sKeyCreationTS: metav1.NewTime(now.Add(-2 * time.Hour)).Format(time.RFC3339),
			},
		},
	}

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
				SecondsAfterCreation: func() *int64 { v := int64(testTTLSeconds); return &v }(),
			},
		},
	}

	mockLister := NewMockResourceLister()
	gvr := schema.GroupVersionResource{Group: "", Version: k8sAPIV1, Resource: k8sResConfigMaps}
	mockLister.SetResources(gvr, "default", []*unstructured.Unstructured{resource})

	mockSelectorMatcher := NewMockSelectorMatcher()
	mockSelectorMatcher.SetMatch(resource, false) // Does NOT match selectors

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
		nil,
	)

	ctx := context.Background()
	err := service.EvaluatePolicy(ctx, policy)
	if err != nil {
		t.Fatalf("EvaluatePolicy failed: %v", err)
	}
	// Resource should not be matched because selectors don't match
}

// TestPolicyEvaluationServiceBatchDeletion tests batch deletion.
func TestPolicyEvaluationServiceBatchDeletion(t *testing.T) {
	now := time.Now()
	resources := make([]*unstructured.Unstructured, 0, testResourceCount)
	for i := 0; i < testResourceCount; i++ {
		resources = append(resources, &unstructured.Unstructured{
			Object: map[string]interface{}{
				k8sKeyAPIVersion: k8sAPIV1,
				k8sKeyKind:       k8sKindConfigMap,
				k8sKeyMetadata: map[string]interface{}{
					k8sKeyName:       "test-cm-" + string(rune('0'+i)),
					k8sKeyNamespace:  k8sNSDefault,
					k8sKeyUID:        types.UID("test-uid-" + string(rune('0'+i))),
					k8sKeyCreationTS: metav1.NewTime(now.Add(-2 * time.Hour)).Format(time.RFC3339),
				},
			},
		})
	}

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
				SecondsAfterCreation: func() *int64 { v := int64(testTTLSeconds); return &v }(),
			},
			Behavior: v1alpha1.BehaviorSpec{
				BatchSize: 2, // Small batch size for testing
			},
		},
	}

	mockLister := NewMockResourceLister()
	gvr := schema.GroupVersionResource{Group: "", Version: k8sAPIV1, Resource: k8sResConfigMaps}
	mockLister.SetResources(gvr, "default", resources)

	mockSelectorMatcher := NewMockSelectorMatcher()
	for _, r := range resources {
		mockSelectorMatcher.SetMatch(r, true)
	}

	mockConditionMatcher := NewMockConditionMatcher()
	for _, r := range resources {
		mockConditionMatcher.SetMeetsConditions(r, true)
	}

	mockRateLimiter := NewMockRateLimiterProvider()
	mockDeleter := NewMockBatchDeleterCore()
	for _, r := range resources {
		mockDeleter.SetDeleteResult(r, nil) // All succeed
	}

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
		nil,
	)

	ctx := context.Background()
	err := service.EvaluatePolicy(ctx, policy)
	if err != nil {
		t.Fatalf("EvaluatePolicy failed: %v", err)
	}
	// All 5 resources should be deleted in batches of 2
}

// TestPolicyEvaluationServiceDeletionErrors tests handling of deletion errors.
func TestPolicyEvaluationServiceDeletionErrors(t *testing.T) {
	now := time.Now()
	resource1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			k8sKeyAPIVersion: k8sAPIV1,
			k8sKeyKind:       k8sKindConfigMap,
			k8sKeyMetadata: map[string]interface{}{
				k8sKeyName:       "test-cm-1",
				k8sKeyNamespace:  k8sNSDefault,
				k8sKeyUID:        "test-uid-1",
				k8sKeyCreationTS: metav1.NewTime(now.Add(-2 * time.Hour)).Format(time.RFC3339),
			},
		},
	}
	resource2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			k8sKeyAPIVersion: k8sAPIV1,
			k8sKeyKind:       k8sKindConfigMap,
			k8sKeyMetadata: map[string]interface{}{
				k8sKeyName:       "test-cm-2",
				k8sKeyNamespace:  k8sNSDefault,
				k8sKeyUID:        "test-uid-2",
				k8sKeyCreationTS: metav1.NewTime(now.Add(-2 * time.Hour)).Format(time.RFC3339),
			},
		},
	}

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
				SecondsAfterCreation: func() *int64 { v := int64(testTTLSeconds); return &v }(),
			},
		},
	}

	mockLister := NewMockResourceLister()
	gvr := schema.GroupVersionResource{Group: "", Version: k8sAPIV1, Resource: k8sResConfigMaps}
	mockLister.SetResources(gvr, "default", []*unstructured.Unstructured{resource1, resource2})

	mockSelectorMatcher := NewMockSelectorMatcher()
	mockSelectorMatcher.SetMatch(resource1, true)
	mockSelectorMatcher.SetMatch(resource2, true)

	mockConditionMatcher := NewMockConditionMatcher()
	mockConditionMatcher.SetMeetsConditions(resource1, true)
	mockConditionMatcher.SetMeetsConditions(resource2, true)

	mockRateLimiter := NewMockRateLimiterProvider()
	mockDeleter := NewMockBatchDeleterCore()
	mockDeleter.SetDeleteResult(resource1, nil)               // Success
	mockDeleter.SetDeleteResult(resource2, errDeletionFailed) // Error

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
		nil,
	)

	ctx := context.Background()
	err := service.EvaluatePolicy(ctx, policy)
	if err != nil {
		t.Fatalf("EvaluatePolicy should handle deletion errors gracefully: %v", err)
	}
	// One deletion should succeed, one should fail
}
