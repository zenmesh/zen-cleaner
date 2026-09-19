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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
)

func TestDeleteResourceWithBackoff_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	dynamicClient := fake.NewSimpleDynamicClient(scheme)
	statusUpdater := NewStatusUpdater(dynamicClient)
	eventRecorder := NewEventRecorder(nil)

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
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			Behavior: v1alpha1.BehaviorSpec{
				DryRun: true,
			},
		},
	}

	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-resource",
				"namespace": "default",
			},
		},
	}

	rateLimiter := ratelimiter.NewRateLimiter(10)
	ctx := context.Background()

	// Should succeed (dry run, so no actual deletion)
	err := reconciler.deleteResourceWithBackoff(ctx, resource, policy, rateLimiter)
	if err != nil {
		t.Errorf("deleteResourceWithBackoff() returned error: %v", err)
	}

	err = DeleteResourceWithBackoff(ctx, reconciler, resource, policy, rateLimiter)
	if err != nil {
		t.Errorf("DeleteResourceWithBackoff package func returned error: %v", err)
	}
}

func TestDeleteResourceWithBackoff_ContextCanceled(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	dynamicClient := fake.NewSimpleDynamicClient(scheme)
	statusUpdater := NewStatusUpdater(dynamicClient)
	eventRecorder := NewEventRecorder(nil)

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
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
		},
	}

	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-resource",
				"namespace": "default",
			},
		},
	}

	rateLimiter := ratelimiter.NewRateLimiter(10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should return context.Canceled error
	err := reconciler.DeleteResourceWithBackoff(ctx, resource, policy, rateLimiter)
	if err == nil {
		t.Error("deleteResourceWithBackoff() should return error when context is canceled")
	}
}

func TestDeleteResourceWithBackoff_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	dynamicClient := fake.NewSimpleDynamicClient(scheme)
	statusUpdater := NewStatusUpdater(dynamicClient)
	eventRecorder := NewEventRecorder(nil)

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
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
		},
	}

	// Resource that doesn't exist
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "non-existent",
				"namespace": "default",
			},
		},
	}

	rateLimiter := ratelimiter.NewRateLimiter(10)
	ctx := context.Background()

	// NotFound errors should be treated as success (already deleted)
	err := reconciler.DeleteResourceWithBackoff(ctx, resource, policy, rateLimiter)
	if err != nil {
		t.Errorf("deleteResourceWithBackoff() should treat NotFound as success, got error: %v", err)
	}
}
