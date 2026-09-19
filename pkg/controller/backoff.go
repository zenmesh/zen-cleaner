// Package controller implements the Zen Cleaner controller.
package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

// DeleteResourceWithBackoff deletes a resource with exponential backoff retry logic.
// This is a convenience wrapper for PolicyReconciler.
func DeleteResourceWithBackoff(ctx context.Context, reconciler *PolicyReconciler, resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter) error {
	return deleteResourceWithBackoff(ctx, reconciler, resource, policy, rateLimiter)
}

// deleteResourceWithBackoff is the internal implementation.
func deleteResourceWithBackoff(ctx context.Context, reconciler *PolicyReconciler, resource *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter) error {
	// Use the deleter from PolicyReconciler
	return reconciler.deleteResource(ctx, resource, policy, rateLimiter)
}
