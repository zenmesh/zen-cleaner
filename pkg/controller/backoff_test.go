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
	"strings"
	"testing"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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

	// NotFound is terminal: the wrapper must NOT retry absence, and it must
	// surface the typed absence signal (never a plain nil success — the
	// batch layer consumes ErrAlreadyAbsent so absence is not counted as a
	// performed deletion).
	err := reconciler.DeleteResourceWithBackoff(ctx, resource, policy, rateLimiter)
	if !errors.Is(err, ErrAlreadyAbsent) {
		t.Errorf("deleteResourceWithBackoff() must return the typed absence signal without retry, got: %v", err)
	}
}

// ── Retry-law coverage (deletion retry loop, previously uncovered) ────────

type retrySequenceDeleter struct {
	attempts int
	errors   []error // returned in order; nil = success
}

func (d *retrySequenceDeleter) DeleteResourceWithContext(ctx context.Context, resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter) error {
	i := d.attempts
	d.attempts++
	if i < len(d.errors) {
		return d.errors[i]
	}
	return nil
}

func retryTestPolicy() *v1alpha1.ZenCleanerPolicy {
	return &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "p"},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{APIVersion: "v1", Kind: "ConfigMap"},
		},
	}
}

// Retryable API failures (429/503/timeouts) are retried with backoff until
// success; the resource is deleted on the first non-retryable outcome.
func TestDeleteResourceWithBackoffShared_RetryableErrorRetriedUntilSuccess(t *testing.T) {
	tooMany := k8serrors.NewTooManyRequests("slow down", 1)
	d := &retrySequenceDeleter{errors: []error{tooMany, k8serrors.NewServiceUnavailable("overloaded")}}
	err := deleteResourceWithBackoffShared(context.Background(), &unstructured.Unstructured{}, retryTestPolicy(), ratelimiter.NewRateLimiter(100), d, nil)
	if err != nil {
		t.Fatalf("retry-until-success failed: %v", err)
	}
	if d.attempts != 3 {
		t.Fatalf("attempts = %d, want 3 (two retryable failures then success)", d.attempts)
	}
}

// Exhaustion is honest: after the configured steps the error surfaces as a
// retry-exhaustion failure naming the last cause (never a silent no-op).
func TestDeleteResourceWithBackoffShared_RetryableErrorExhausts(t *testing.T) {
	deleter := &alwaysTooManyDeleter{}
	err := deleteResourceWithBackoffShared(context.Background(), &unstructured.Unstructured{}, retryTestPolicy(), ratelimiter.NewRateLimiter(100), deleter, nil)
	if err == nil {
		t.Fatal("exhausted retries must surface an error")
	}
	if !strings.Contains(err.Error(), "deletion failed after retries") {
		t.Fatalf("exhaustion error must name the retry exhaustion: %v", err)
	}
	if deleter.attempts == 1 {
		t.Fatal("exhaustion requires more than one attempt")
	}
}

type alwaysTooManyDeleter struct{ attempts int }

func (d *alwaysTooManyDeleter) DeleteResourceWithContext(ctx context.Context, resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter) error {
	d.attempts++
	return k8serrors.NewTooManyRequests("slow down", 1)
}

// The legacy without-context deleter branch still reaches the same laws.
func TestDeleteResourceWithBackoffShared_WithoutContextDeleter(t *testing.T) {
	calls := 0
	stub := &withoutCtxDeleter{onCall: func() error { calls++; return nil }}
	err := deleteResourceWithBackoffShared(context.Background(), &unstructured.Unstructured{}, retryTestPolicy(), ratelimiter.NewRateLimiter(100), nil, stub)
	if err != nil {
		t.Fatalf("got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("without-context deleter calls = %d, want 1", calls)
	}
}

type withoutCtxDeleter struct {
	onCall func() error
}

func (d *withoutCtxDeleter) DeleteResourceWithoutContext(resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter) error {
	return d.onCall()
}
