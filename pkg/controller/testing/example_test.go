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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

// TestMockUsage demonstrates how to use mocks for testing.
func TestMockUsage(t *testing.T) {
	// Create mock informer with test resources
	resources := []*unstructured.Unstructured{
		{
			Object: map[string]interface{}{
				k8sKeyAPIVersion: k8sAPIV1,
				k8sKeyKind:       k8sKindConfigMap,
				k8sKeyMetadata: map[string]interface{}{
					k8sKeyName:      k8sNameTestCM,
					k8sKeyNamespace: k8sNSDefault,
					k8sKeyUID:       k8sUIDTest,
				},
			},
		},
	}

	mockInformer := NewMockResourceInformer(resources)
	if !mockInformer.HasSynced() {
		t.Error("Mock informer should be synced")
	}

	store := mockInformer.GetStore()
	if store == nil {
		t.Error("Mock informer should have a store")
	}

	// Create mock resource lister
	mockLister := NewMockResourceLister()
	gvr := schema.GroupVersionResource{Group: "", Version: k8sAPIV1, Resource: k8sResConfigMaps}
	mockLister.SetResources(gvr, "default", resources)

	listed, err := mockLister.ListResources(context.Background(), gvr, "default")
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	if len(listed) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(listed))
	}

	// Create mock selector matcher
	mockMatcher := NewMockSelectorMatcher()
	resource := resources[0]
	mockMatcher.SetMatch(resource, true)

	spec := &v1alpha1.TargetResourceSpec{
		APIVersion: k8sAPIV1,
		Kind:       k8sKindConfigMap,
	}
	if !mockMatcher.MatchesSelectors(resource, spec) {
		t.Error("Resource should match selectors")
	}

	// Create mock rate limiter provider
	mockRateLimiter := NewMockRateLimiterProvider()
	policy := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sPolicyNameTest,
			Namespace: k8sNSDefault,
			UID:       types.UID("policy-uid"),
		},
	}
	limiter := mockRateLimiter.GetOrCreateRateLimiter(policy)
	if limiter == nil {
		t.Error("Rate limiter should not be nil")
	}

	// Create mock batch deleter
	mockDeleter := NewMockBatchDeleterCore()
	mockDeleter.SetDeleteResult(resource, nil) // Success

	batch := []*unstructured.Unstructured{resource}
	deleted, errors := mockDeleter.DeleteBatch(context.Background(), batch, policy, limiter, nil)
	if deleted != 1 {
		t.Errorf("Expected 1 deletion, got %d", deleted)
	}
	if len(errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(errors))
	}
}

// TestMockResourceInformer tests the mock resource informer.
func TestMockResourceInformer(t *testing.T) {
	resources := []*unstructured.Unstructured{
		{
			Object: map[string]interface{}{
				k8sKeyAPIVersion: k8sAPIV1,
				k8sKeyKind:       k8sKindConfigMap,
				k8sKeyMetadata: map[string]interface{}{
					k8sKeyName:      k8sNameTestCM,
					k8sKeyNamespace: k8sNSDefault,
				},
			},
		},
	}

	mock := NewMockResourceInformer(resources)
	if !mock.HasSynced() {
		t.Error("Should be synced by default")
	}

	store := mock.GetStore()
	if store == nil {
		t.Fatal("Store should not be nil")
	}

	items := store.List()
	if len(items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(items))
	}

	mock.SetSynced(false)
	if mock.HasSynced() {
		t.Error("Should not be synced after SetSynced(false)")
	}
}

// TestMockResourceLister tests the mock resource lister.
func TestMockResourceLister(t *testing.T) {
	mock := NewMockResourceLister()
	gvr := schema.GroupVersionResource{Group: "", Version: k8sAPIV1, Resource: k8sResConfigMaps}

	// Test empty list
	resources, err := mock.ListResources(context.Background(), gvr, "default")
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	if len(resources) != 0 {
		t.Errorf("Expected empty list, got %d resources", len(resources))
	}

	// Set resources
	testResources := []*unstructured.Unstructured{
		{
			Object: map[string]interface{}{
				k8sKeyAPIVersion: k8sAPIV1,
				k8sKeyKind:       k8sKindConfigMap,
				k8sKeyMetadata: map[string]interface{}{
					k8sKeyName:      k8sNameTestCM,
					k8sKeyNamespace: k8sNSDefault,
				},
			},
		},
	}
	mock.SetResources(gvr, "default", testResources)

	// List resources
	resources, err = mock.ListResources(context.Background(), gvr, "default")
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	if len(resources) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(resources))
	}
}
