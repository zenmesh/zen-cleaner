package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	sdklog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
	cleanererrors "github.com/zenmesh/zen-cleaner/pkg/errors"
)

// statusSubresourceKey is the unstructured object key for CRD status.
const statusSubresourceKey = "status"

// PolicyGVR is the GroupVersionResource for ZenCleanerPolicy CRDs.
var PolicyGVR = schema.GroupVersionResource{
	Group:    "cleaner.zen-mesh.io",
	Version:  "v1alpha1",
	Resource: "zencleanerpolicies",
}

// StatusUpdater updates ZenCleanerPolicy CRD status subresource.
type StatusUpdater struct {
	dynClient dynamic.Interface
	config    *config.ControllerConfig
}

// NewStatusUpdater creates a new status updater.
func NewStatusUpdater(dynClient dynamic.Interface) *StatusUpdater {
	return &StatusUpdater{
		dynClient: dynClient,
		config:    config.NewControllerConfig(),
	}
}

// NewStatusUpdaterWithConfig creates a new status updater with configuration.
func NewStatusUpdaterWithConfig(dynClient dynamic.Interface, cfg *config.ControllerConfig) *StatusUpdater {
	if cfg == nil {
		cfg = config.NewControllerConfig()
	}
	return &StatusUpdater{
		dynClient: dynClient,
		config:    cfg,
	}
}

// UpdateStatus updates the ZenCleanerPolicy CRD status subresource.
func (s *StatusUpdater) UpdateStatus(
	ctx context.Context,
	policy *v1alpha1.ZenCleanerPolicy,
	matched, deleted, pending int64,
) error {
	// Get the current policy CRD
	unstructuredPolicy, err := s.dynClient.Resource(PolicyGVR).
		Namespace(policy.Namespace).
		Get(ctx, policy.Name, metav1.GetOptions{})
	if err != nil {
		cleanerErr := cleanererrors.Wrap(err, "status_get_failed", "failed to get ZenCleanerPolicy CRD")
		cleanerErr = cleanerErr.WithContext("policy_namespace", policy.Namespace)
		cleanerErr = cleanerErr.WithContext("policy_name", policy.Name)
		return cleanerErr
	}

	// Build status object
	now := metav1.Now()
	interval := DefaultCleanupInterval
	if s.config != nil {
		interval = s.config.CleanupInterval
	}
	nextRun := metav1.NewTime(now.Add(interval))

	statusObj := map[string]interface{}{
		"resourcesMatched": matched,
		"resourcesDeleted": deleted,
		"resourcesPending": pending,
		"lastGCRun":        now.Format(time.RFC3339),
		"nextGCRun":        nextRun.Format(time.RFC3339),
	}

	// Set phase based on spec.paused and evaluation state
	// Phase is controller-owned output only, not user-settable
	phase := PolicyPhaseActive
	if policy.Spec.Paused {
		phase = PolicyPhasePaused
	}
	// "Error" phase should be set by controller when evaluation fails consistently
	// For now, we keep existing phase if it's "Error", otherwise use computed phase
	if policy.Status.Phase == PolicyPhaseError {
		phase = PolicyPhaseError // Preserve error state until cleared by successful evaluation
	}
	statusObj["phase"] = phase

	// Set status conditions
	conditions := []map[string]interface{}{}
	nowStr := now.Format(time.RFC3339)

	// Ready condition
	readyCondition := map[string]interface{}{
		"type":               "Ready",
		statusSubresourceKey: "True",
		"lastTransitionTime": nowStr,
		"reason":             "PolicyActive",
		"message":            "Policy is active and processing resources",
	}
	if phase == PolicyPhaseError {
		readyCondition[statusSubresourceKey] = "False"
		readyCondition["reason"] = "PolicyError"
		readyCondition["message"] = "Policy evaluation encountered errors"
	} else if phase == PolicyPhasePaused {
		readyCondition[statusSubresourceKey] = "False"
		readyCondition["reason"] = "PolicyPaused"
		readyCondition["message"] = "Policy is paused"
	}
	conditions = append(conditions, readyCondition)

	// Error condition (only set if there are errors)
	if phase == PolicyPhaseError {
		errorCondition := map[string]interface{}{
			"type":               PolicyPhaseError,
			statusSubresourceKey: "True",
			"lastTransitionTime": nowStr,
			"reason":             "EvaluationFailed",
			"message":            "Policy evaluation failed - check logs for details",
		}
		conditions = append(conditions, errorCondition)
	}

	// Convert conditions to []interface{} to avoid deep copy issues with []map[string]interface{}
	conditionsInterface := make([]interface{}, len(conditions))
	for i, cond := range conditions {
		conditionsInterface[i] = cond
	}
	statusObj["conditions"] = conditionsInterface

	// Merge status (preserve existing fields, update only provided fields)
	if existingStatus, ok := unstructuredPolicy.Object[statusSubresourceKey].(map[string]interface{}); ok {
		// Merge: update provided fields, keep others
		for k, v := range statusObj {
			existingStatus[k] = v
		}
		unstructuredPolicy.Object[statusSubresourceKey] = existingStatus
	} else {
		// No existing status, set new status
		unstructuredPolicy.Object[statusSubresourceKey] = statusObj
	}

	// Update status subresource
	_, err = s.dynClient.Resource(PolicyGVR).
		Namespace(policy.Namespace).
		UpdateStatus(ctx, unstructuredPolicy, metav1.UpdateOptions{})
	if err != nil {
		cleanerErr := cleanererrors.Wrap(err, "status_update_failed", "failed to update ZenCleanerPolicy status")
		cleanerErr = cleanerErr.WithContext("policy_namespace", policy.Namespace)
		cleanerErr = cleanerErr.WithContext("policy_name", policy.Name)
		logger := sdklog.NewLogger("zen-cleaner")
		logger.Warn("Failed to update ZenCleanerPolicy status", sdklog.Operation("update_status"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)), sdklog.Error(cleanerErr))
		return cleanerErr
	}

	logger := sdklog.NewLogger("zen-cleaner")
	logger.Debug("Updated ZenCleanerPolicy status", sdklog.Operation("update_status"), sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)), sdklog.Int64("matched", matched), sdklog.Int64("deleted", deleted), sdklog.Int64("pending", pending))

	return nil
}
