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
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"

	sdklog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	cleanererrors "github.com/zenmesh/zen-cleaner/pkg/errors"
	"github.com/zenmesh/zen-cleaner/pkg/safety"
	"github.com/zenmesh/zen-cleaner/pkg/validation"
)

// handlePolicyDeletion handles cleanup when a policy is deleted.
func (r *PolicyReconciler) handlePolicyDeletion(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = ctx // Context reserved for future use (cancellation/timeout)
	r.logger.Debug("Policy not found, cleaning up resources", sdklog.Operation("reconcile"))
	r.cleanupPolicyResources(req.NamespacedName)
	return ctrl.Result{}, nil
}

// handlePolicyFetchError handles errors when fetching a policy.
func (r *PolicyReconciler) handlePolicyFetchError(err error) (ctrl.Result, error) {
	r.logger.Error(err, "Failed to fetch ZenCleanerPolicy", sdklog.Operation("fetch_policy"), sdklog.ErrorCode("FETCH_POLICY_FAILED"))
	return ctrl.Result{}, err
}

// handleInformerRecreation handles informer recreation when policy spec changes.
func (r *PolicyReconciler) handleInformerRecreation(policy *v1alpha1.ZenCleanerPolicy) {
	if !r.shouldRecreateInformer(policy) {
		return
	}

	r.logger.Debug("Policy spec changed, recreating informer", sdklog.Operation("update_informer"))
	r.cleanupResourceInformer(policy.UID)
	// Clear old spec to allow new one to be tracked
	r.policySpecsMu.Lock()
	delete(r.policySpecs, policy.UID)
	r.policySpecsMu.Unlock()
}

// handlePausedPolicy handles paused policies.
func (r *PolicyReconciler) handlePausedPolicy() (ctrl.Result, error) {
	r.logger.Debug("Policy is paused, skipping evaluation", sdklog.Operation("reconcile"))
	return ctrl.Result{RequeueAfter: r.getRequeueInterval()}, nil
}

// handleEvaluationError handles errors during policy evaluation.
// SUPPORT2-003 §7: classified target-capability failures requeue with
// BOUNDED exponential backoff (30s doubling, capped at 10m) — no hot loop,
// no informer recreation storm, no leader churn. Other errors keep the
// fixed 30s requeue.
func (r *PolicyReconciler) handleEvaluationError(err error, policy *v1alpha1.ZenCleanerPolicy) (ctrl.Result, error) {
	var tcErr *TargetCapabilityError
	if errors.As(err, &tcErr) {
		count := r.targetFailureCount(policy.UID)
		delay := TargetBackoffFor(count)
		r.logger.Warn("Target capability validation failed; bounded backoff",
			sdklog.Operation("evaluate_policy"),
			sdklog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)),
			sdklog.String("target_class", string(tcErr.Type)),
			sdklog.String("retry_in", delay.String()))
		return ctrl.Result{RequeueAfter: delay}, nil
	}
	cleanerErr := cleanererrors.WithPolicy(err, policy.Namespace, policy.Name)
	if cleanerErr.Type == "" {
		cleanerErr.Type = ErrorTypeEvaluationFailed
	}
	r.logger.Error(cleanerErr, "Error evaluating policy", sdklog.Operation("evaluate_policy"), sdklog.ErrorCode("EVALUATE_POLICY_FAILED"))
	// Requeue with backoff on error
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// resolveGVRForDeletion resolves the GVR for a resource deletion.
func (r *PolicyReconciler) resolveGVRForDeletion(resource *unstructured.Unstructured) schema.GroupVersionResource {
	if r.gvrResolver != nil {
		resolvedGVR, gvrErr := r.gvrResolver.ResolveGVR(resource)
		if gvrErr == nil {
			return resolvedGVR
		}
		// Fall back to pluralization if GVRResolver fails
		r.logger.Debug("GVRResolver failed, falling back to pluralization", sdklog.Operation("delete_resource"), sdklog.String("resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName())), sdklog.Error(gvrErr))
	}

	// Use pluralization fallback
	return schema.GroupVersionResource{
		Group:    resource.GroupVersionKind().Group,
		Version:  resource.GroupVersionKind().Version,
		Resource: validation.PluralizeKind(resource.GetKind()),
	}
}

// buildDeleteOptions builds delete options from policy behavior.
func buildDeleteOptions(policy *v1alpha1.ZenCleanerPolicy) *metav1.DeleteOptions {
	deleteOptions := &metav1.DeleteOptions{}
	if policy.Spec.Behavior.GracePeriodSeconds != nil {
		deleteOptions.GracePeriodSeconds = policy.Spec.Behavior.GracePeriodSeconds
	}

	propagationPolicy := getDeletionPropagationPolicy(policy.Spec.Behavior.PropagationPolicy)
	deleteOptions.PropagationPolicy = &propagationPolicy

	return deleteOptions
}

// ErrStaleUID is returned by performResourceDeletion when the live object's
// UID differs from the observation the deletion decision was made on. The
// object is NOT deleted: a stale-cache decision must never destroy a
// replacement resource (SUPPORT2-033 Â§4).
var ErrStaleUID = errors.New("stale observation: live UID differs from the observed UID; deletion refused")

// performResourceDeletion is the single choke point for DELETE calls. The
// safety gate runs here unconditionally (last line of defense), and the live
// UID precondition guarantees the decision still applies to the object that
// is actually deleted.
func (r *PolicyReconciler) performResourceDeletion(ctx context.Context, resource *unstructured.Unstructured, gvr schema.GroupVersionResource, deleteOptions *metav1.DeleteOptions, behavior v1alpha1.BehaviorSpec) error {
	// Last-line safety gate (SUPPORT2-033 Â§2): protected namespaces,
	// hard-protected kinds, exclusion labels, workload laws, emergency stop.
	if r.safetyGate != nil {
		if allowed, reason := r.safetyGate.EvaluateObject(resource, behavior); !allowed {
			RecordSafetyRefusal(reason)
			r.logger.Info("Safety gate refused deletion",
				sdklog.Operation("delete_resource"),
				sdklog.String("reason", reason),
				sdklog.String("resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName())))
			return nil
		}
	}

	// Live UID precondition: re-read the object from the API server and
	// refuse to delete a replacement (same name, new UID) or an object that
	// vanished after the decision (treated as already deleted).
	live, err := r.getLiveForPrecondition(ctx, resource, gvr)
	if err != nil {
		RecordAPIError(err)
		return err
	}
	if live == nil {
		// Already gone: idempotent success, nothing was deleted.
		return nil
	}
	if live.GetUID() != resource.GetUID() {
		RecordSafetyRefusal(safety.ReasonStaleUID)
		r.logger.Info("Stale-cache deletion refused (UID precondition)",
			sdklog.Operation("delete_resource"),
			sdklog.String("reason", safety.ReasonStaleUID),
			sdklog.String("resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName())))
		return nil
	}

	namespace := resource.GetNamespace()
	RecordDeletionAttempted()
	var err2 error
	if namespace == "" {
		err2 = r.dynamicClient.Resource(gvr).Delete(ctx, resource.GetName(), *deleteOptions)
	} else {
		err2 = r.dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, resource.GetName(), *deleteOptions)
	}

	if err2 != nil && !apierrors.IsNotFound(err2) {
		RecordDeletionFailed()
		RecordAPIError(err2)
		return err2
	}

	return nil
}

// getLiveForPrecondition re-reads the object from the API server for the UID
// precondition. nil (with nil error) means the object is already gone.
func (r *PolicyReconciler) getLiveForPrecondition(ctx context.Context, resource *unstructured.Unstructured, gvr schema.GroupVersionResource) (*unstructured.Unstructured, error) {
	var live *unstructured.Unstructured
	var err error
	if resource.GetNamespace() == "" {
		live, err = r.dynamicClient.Resource(gvr).Get(ctx, resource.GetName(), metav1.GetOptions{})
	} else {
		live, err = r.dynamicClient.Resource(gvr).Namespace(resource.GetNamespace()).Get(ctx, resource.GetName(), metav1.GetOptions{})
	}
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return live, nil
}

// normalizeNamespace normalizes namespace for informer creation.
func normalizeNamespace(namespace string) string {
	// Normalize: empty defaults to "*" (cluster-wide) to match webhook behavior
	if namespace == "" {
		namespace = "*"
	}
	// Translate "*" to NamespaceAll (empty string) for cluster-wide watching
	if namespace == "*" {
		namespace = metav1.NamespaceAll
	}
	return namespace
}

// buildLabelSelectorFilter builds a label selector filter function for informer factory.
func buildLabelSelectorFilter(policy *v1alpha1.ZenCleanerPolicy) func(options *metav1.ListOptions) {
	return func(options *metav1.ListOptions) {
		if policy.Spec.TargetResource.LabelSelector != nil {
			selector, err := metav1.LabelSelectorAsSelector(policy.Spec.TargetResource.LabelSelector)
			if err == nil {
				options.LabelSelector = selector.String()
			}
		}
	}
}
