package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	sdklog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

func TestPolicyReconciler_shouldDelete_TTLExpired(t *testing.T) {
	reconciler := &PolicyReconciler{
		logger: sdklog.NewLogger("zen-cleaner"),
	}

	// Create a resource that was created 2 hours ago
	creationTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"creationTimestamp": creationTime.Format(time.RFC3339),
			},
		},
	}

	policy := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TTL: v1alpha1.TTLSpec{
				SecondsAfterCreation: int64Ptr(3600), // 1 hour TTL
			},
		},
	}

	shouldDelete, reason := reconciler.shouldDelete(resource, policy)
	if !shouldDelete {
		t.Errorf("shouldDelete() = false, want true (resource is expired)")
	}
	if reason != ReasonTTLExpired {
		t.Errorf("shouldDelete() reason = %q, want %q", reason, ReasonTTLExpired)
	}
}

func TestPolicyReconciler_shouldDelete_TTLNotExpired(t *testing.T) {
	reconciler := &PolicyReconciler{
		logger: sdklog.NewLogger("zen-cleaner"),
	}

	// Create a resource that was created 30 minutes ago
	creationTime := metav1.NewTime(time.Now().Add(-30 * time.Minute))
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"creationTimestamp": creationTime.Format(time.RFC3339),
			},
		},
	}

	policy := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TTL: v1alpha1.TTLSpec{
				SecondsAfterCreation: int64Ptr(3600), // 1 hour TTL
			},
		},
	}

	shouldDelete, reason := reconciler.shouldDelete(resource, policy)
	if shouldDelete {
		t.Errorf("shouldDelete() = true, want false (resource is not expired)")
	}
	if reason != "not_expired" {
		t.Errorf("shouldDelete() reason = %q, want 'not_expired'", reason)
	}
}

func TestPolicyReconciler_shouldDelete_ConditionNotMet(t *testing.T) {
	reconciler := &PolicyReconciler{
		logger: sdklog.NewLogger("zen-cleaner"),
	}

	// Create an expired resource
	creationTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"creationTimestamp": creationTime.Format(time.RFC3339),
			},
			"status": map[string]interface{}{
				"phase": "Pending", // Not Processed
			},
		},
	}

	policy := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TTL: v1alpha1.TTLSpec{
				SecondsAfterCreation: int64Ptr(3600), // 1 hour TTL
			},
			Conditions: &v1alpha1.ConditionsSpec{
				Phase: []string{"Processed"}, // Only delete Processed resources
			},
		},
	}

	shouldDelete, reason := reconciler.shouldDelete(resource, policy)
	if shouldDelete {
		t.Errorf("shouldDelete() = true, want false (condition not met)")
	}
	if reason != ReasonConditionNotMet {
		t.Errorf("shouldDelete() reason = %q, want %q", reason, ReasonConditionNotMet)
	}
}

func TestPolicyReconciler_shouldDelete_RelativeTTL_Expired(t *testing.T) {
	reconciler := &PolicyReconciler{
		logger: sdklog.NewLogger("zen-cleaner"),
	}

	// RelativeTo timestamp 3 hours ago + 1 hour SecondsAfter = already expired
	lastProcessed := metav1.NewTime(time.Now().Add(-3 * time.Hour))
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"creationTimestamp": metav1.NewTime(time.Now().Add(-3 * time.Hour)).Format(time.RFC3339),
			},
			"status": map[string]interface{}{
				"lastProcessedAt": lastProcessed.Format(time.RFC3339),
			},
		},
	}

	policy := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TTL: v1alpha1.TTLSpec{
				RelativeTo:   "status.lastProcessedAt",
				SecondsAfter: int64Ptr(3600), // 1 hour after lastProcessedAt (3h ago → expired)
			},
		},
	}

	shouldDelete, reason := reconciler.shouldDelete(resource, policy)
	if !shouldDelete {
		t.Error("BUG-002: shouldDelete() = false, want true (relative TTL already expired)")
	}
	if reason != ReasonTTLExpired {
		t.Errorf("BUG-002: shouldDelete() reason = %q, want %q", reason, ReasonTTLExpired)
	}
}

func TestPolicyReconciler_shouldDelete_RelativeTTL_NotExpired(t *testing.T) {
	reconciler := &PolicyReconciler{
		logger: sdklog.NewLogger("zen-cleaner"),
	}

	// RelativeTo timestamp 30 minutes ago + 2 hours SecondsAfter = not yet expired
	lastProcessed := metav1.NewTime(time.Now().Add(-30 * time.Minute))
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"creationTimestamp": metav1.NewTime(time.Now().Add(-30 * time.Minute)).Format(time.RFC3339),
			},
			"status": map[string]interface{}{
				"lastProcessedAt": lastProcessed.Format(time.RFC3339),
			},
		},
	}

	policy := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TTL: v1alpha1.TTLSpec{
				RelativeTo:   "status.lastProcessedAt",
				SecondsAfter: int64Ptr(7200), // 2 hours after lastProcessedAt (30min ago → not expired)
			},
		},
	}

	shouldDelete, reason := reconciler.shouldDelete(resource, policy)
	if shouldDelete {
		t.Error("shouldDelete() = true, want false (relative TTL not yet expired)")
	}
	if reason != ReasonNotExpired {
		t.Errorf("shouldDelete() reason = %q, want %q", reason, ReasonNotExpired)
	}
}

func TestPolicyReconciler_shouldDelete_NoTTL(t *testing.T) {
	reconciler := &PolicyReconciler{
		logger: sdklog.NewLogger("zen-cleaner"),
	}

	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}

	policy := &v1alpha1.ZenCleanerPolicy{
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TTL: v1alpha1.TTLSpec{}, // No TTL configured
		},
	}

	shouldDelete, reason := reconciler.shouldDelete(resource, policy)
	if shouldDelete {
		t.Errorf("shouldDelete() = true, want false (no TTL)")
	}
	if reason != ReasonNoTTL {
		t.Errorf("shouldDelete() reason = %q, want %q", reason, ReasonNoTTL)
	}
}
