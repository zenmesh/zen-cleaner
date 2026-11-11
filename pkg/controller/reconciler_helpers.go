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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	cleanerlog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	cleanererrors "github.com/zenmesh/zen-cleaner/pkg/errors"
	"github.com/zenmesh/zen-cleaner/pkg/safety"
	"github.com/zenmesh/zen-cleaner/pkg/validation"
)

// handlePolicyDeletion handles cleanup when a policy is deleted.
func (r *PolicyReconciler) handlePolicyDeletion(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = ctx // Context reserved for future use (cancellation/timeout)
	r.logger.Debug("Policy not found, cleaning up resources", cleanerlog.Operation("reconcile"))
	r.cleanupPolicyResources(req.NamespacedName)
	return ctrl.Result{}, nil
}

// handlePolicyFetchError handles errors when fetching a policy.
func (r *PolicyReconciler) handlePolicyFetchError(err error) (ctrl.Result, error) {
	r.logger.Error(err, "Failed to fetch ZenCleanerPolicy", cleanerlog.Operation("fetch_policy"), cleanerlog.ErrorCode("FETCH_POLICY_FAILED"))
	return ctrl.Result{}, err
}

// handleInformerRecreation handles informer recreation when policy spec changes.
func (r *PolicyReconciler) handleInformerRecreation(policy *v1alpha1.ZenCleanerPolicy) {
	if !r.shouldRecreateInformer(policy) {
		return
	}

	r.logger.Debug("Policy spec changed, recreating informer", cleanerlog.Operation("update_informer"))
	r.cleanupResourceInformer(policy.UID)
	// Clear old spec to allow new one to be tracked
	r.policySpecsMu.Lock()
	delete(r.policySpecs, policy.UID)
	r.policySpecsMu.Unlock()
}

// handlePausedPolicy handles paused policies.
func (r *PolicyReconciler) handlePausedPolicy() (ctrl.Result, error) {
	r.logger.Debug("Policy is paused, skipping evaluation", cleanerlog.Operation("reconcile"))
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
			cleanerlog.Operation("evaluate_policy"),
			cleanerlog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)),
			cleanerlog.String("target_class", string(tcErr.Type)),
			cleanerlog.String("retry_in", delay.String()))
		return ctrl.Result{RequeueAfter: delay}, nil
	}
	cleanerErr := cleanererrors.WithPolicy(err, policy.Namespace, policy.Name)
	if cleanerErr.Type == "" {
		cleanerErr.Type = ErrorTypeEvaluationFailed
	}
	r.logger.Error(cleanerErr, "Error evaluating policy", cleanerlog.Operation("evaluate_policy"), cleanerlog.ErrorCode("EVALUATE_POLICY_FAILED"))
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
		r.logger.Debug("GVRResolver failed, falling back to pluralization", cleanerlog.Operation("delete_resource"), cleanerlog.String("resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName())), cleanerlog.Error(gvrErr))
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

// ErrStaleUID is the sentinel cause of a stale-observation refusal: the live
// object's UID differs from the UID the deletion decision was made on. The
// object is NOT deleted: a stale-cache decision must never destroy a
// replacement resource (SUPPORT2-033 §4). It is carried as the Cause of a
// *DeletionRefusedError.
var ErrStaleUID = errors.New("stale observation: live UID differs from the observed UID; deletion refused")

// performResourceDeletion is the single choke point for DELETE calls. The
// contract (SUPPORT2-033 §4, atomic UID deletion):
//
//  1. The CURRENT object is re-read from the API server; every safety and
//     policy condition (exclusion labels/annotations, workload laws, scope
//     selectors, conditions, TTL) is re-evaluated on THAT object — never on
//     the earlier observation the batch decision was made from.
//  2. The DELETE carries the UID observed at decision time AND the
//     resourceVersion of the object actually evaluated as server-side
//     preconditions, so a replacement (new UID) or a same-UID update after
//     the evaluation refuses atomically at the API server.
//  3. An empty observed UID refuses the deletion outright instead of
//     silently degrading to delete-by-name.
//  4. The returned outcome is truthful: refusals and precondition conflicts
//     are non-deletions (the object survives), never fake successes.
func (r *PolicyReconciler) performResourceDeletion(ctx context.Context, resource *unstructured.Unstructured, gvr schema.GroupVersionResource, deleteOptions *metav1.DeleteOptions, policy *v1alpha1.ZenCleanerPolicy) (DeletionOutcomeRecord, error) {
	// Live UID precondition: re-read the object from the API server. An
	// object that vanished after the decision is already deleted (idempotent
	// no-op, nothing was deleted by this call).
	live, err := r.getLiveForPrecondition(ctx, resource, gvr)
	if err != nil {
		RecordAPIError(err)
		return DeletionOutcomeRecord{Outcome: OutcomeFailed}, err
	}
	if live == nil {
		return DeletionOutcomeRecord{Outcome: OutcomeAlreadyAbsent}, nil
	}

	// Last-line safety gate (SUPPORT2-033 §2) re-evaluated on the LIVE
	// object: protected namespaces, hard-protected kinds, exclusion
	// labels/annotations, workload laws, emergency stop. A same-UID update
	// that adds an exclusion label between the batch decision and this gate
	// is refused here. The gate runs even when no governing policy context is
	// available (empty behavior), so protection never depends on the policy.
	gateBehavior := v1alpha1.BehaviorSpec{}
	if policy != nil {
		gateBehavior = policy.Spec.Behavior
	}
	if r.safetyGate != nil {
		if allowed, reason := r.safetyGate.EvaluateObject(live, gateBehavior); !allowed {
			RecordSafetyRefusal(reason)
			r.logger.Info("Safety gate refused deletion (re-evaluated on live object)",
				cleanerlog.Operation("delete_resource"),
				cleanerlog.String("reason", reason),
				cleanerlog.String("resource", fmt.Sprintf("%s/%s", live.GetNamespace(), live.GetName())))
			return DeletionOutcomeRecord{Outcome: OutcomeRefused, Reason: reason}, nil
		}
	}

	// Policy re-evaluation on the LIVE object: the batch decision was made on
	// an informer-cache observation; if the current object no longer matches
	// the policy's selectors, conditions, or TTL, the decision no longer
	// applies (fail closed and requeue under current policy). A missing
	// governing policy context cannot be re-evaluated — fail closed.
	if reason, detail := r.deletionEligibilityRefusal(live, policy); reason != "" {
		RecordSafetyRefusal(safety.ReasonNoLongerEligible)
		r.logger.Info("Deletion refused: current object no longer satisfies policy",
			cleanerlog.Operation("delete_resource"),
			cleanerlog.String("reason", safety.ReasonNoLongerEligible),
			cleanerlog.String("detail", detail),
			cleanerlog.String("resource", fmt.Sprintf("%s/%s", live.GetNamespace(), live.GetName())))
		return DeletionOutcomeRecord{Outcome: OutcomeRefused, Reason: safety.ReasonNoLongerEligible, Detail: detail}, nil
	}

	// UID precondition input: the UID of the object ON WHICH THE DECISION WAS
	// MADE — never a replacement UID re-read after the decision. An empty
	// observed UID refuses instead of degrading to delete-by-name.
	observedUID := resource.GetUID()
	if observedUID == "" {
		RecordSafetyRefusal(safety.ReasonMissingUID)
		r.logger.Error(nil, "Deletion refused: observed object carries no UID; delete-by-name refused",
			cleanerlog.Operation("delete_resource"),
			cleanerlog.String("reason", safety.ReasonMissingUID),
			cleanerlog.String("resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName())))
		return DeletionOutcomeRecord{Outcome: OutcomeRefused, Reason: safety.ReasonMissingUID, Detail: "empty observed UID"}, nil
	}
	if live.GetUID() != observedUID {
		RecordSafetyRefusal(safety.ReasonStaleUID)
		r.logger.Info("Stale-cache deletion refused (UID precondition)",
			cleanerlog.Operation("delete_resource"),
			cleanerlog.String("reason", safety.ReasonStaleUID),
			cleanerlog.String("resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName())))
		return DeletionOutcomeRecord{Outcome: OutcomeRefused, Reason: safety.ReasonStaleUID}, nil
	}

	// The evaluated safety/policy state must be bindable to a version. A
	// live object without a resourceVersion (real API-server objects always
	// carry one) refuses: a precondition that cannot be enforced must never
	// be sent optimistically.
	if live.GetResourceVersion() == "" {
		RecordSafetyRefusal(safety.ReasonMissingResourceVersion)
		r.logger.Error(nil, "Deletion refused: live object carries no resourceVersion; evaluated state cannot be preconditioned",
			cleanerlog.Operation("delete_resource"),
			cleanerlog.String("reason", safety.ReasonMissingResourceVersion),
			cleanerlog.String("resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName())))
		return DeletionOutcomeRecord{Outcome: OutcomeRefused, Reason: safety.ReasonMissingResourceVersion, Detail: "empty live resourceVersion"}, nil
	}

	// Server-side preconditions: the decision-time UID plus the
	// resourceVersion of the object actually evaluated above. A replacement
	// or a same-UID metadata/spec update after the evaluation is rejected by
	// the API server (409 Conflict) instead of being deleted by name.
	opts, err := deletionOptionsWithPreconditions(deleteOptions, observedUID, live.GetResourceVersion())
	if err != nil {
		// A caller-contract conflict is a local failure to compose a safe
		// delete, NOT an API error — it must not feed the API-error metric.
		// It still fails closed (no DELETE, error propagates).
		return DeletionOutcomeRecord{Outcome: OutcomeFailed}, err
	}

	namespace := resource.GetNamespace()
	RecordDeletionAttempted()
	var err2 error
	if namespace == "" {
		err2 = r.dynamicClient.Resource(gvr).Delete(ctx, resource.GetName(), *opts)
	} else {
		err2 = r.dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, resource.GetName(), *opts)
	}

	if err2 == nil {
		return DeletionOutcomeRecord{Outcome: OutcomeDeleted}, nil
	}
	if apierrors.IsNotFound(err2) {
		// Removed between the precondition read and the DELETE: already
		// gone, idempotent, nothing was deleted by this call.
		return DeletionOutcomeRecord{Outcome: OutcomeAlreadyAbsent}, nil
	}
	if apierrors.IsConflict(err2) {
		// The API server rejected the UID/RV precondition: the object was
		// replaced or updated after the evaluation. The object survives;
		// report the non-deletion truthfully and requeue under current
		// policy (the regular reconcile cycle re-evaluates).
		RecordSafetyRefusal(safety.ReasonPreconditionConflict)
		r.logger.Info("Deletion refused by API server precondition conflict",
			cleanerlog.Operation("delete_resource"),
			cleanerlog.String("reason", safety.ReasonPreconditionConflict),
			cleanerlog.String("resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName())))
		return DeletionOutcomeRecord{Outcome: OutcomePreconditionConflict, Reason: safety.ReasonPreconditionConflict}, nil
	}
	RecordDeletionFailed()
	RecordAPIError(err2)
	return DeletionOutcomeRecord{Outcome: OutcomeFailed}, err2
}

// deletionEligibilityRefusal re-evaluates the policy's scope selectors,
// conditions, and TTL against the CURRENT object. It returns a non-empty
// bounded detail when the deletion decision no longer applies (the caller
// refuses and requeues under current policy). A nil governing policy cannot
// be re-evaluated — fail closed (the safety gate above still ran with the
// default behavior).
func (r *PolicyReconciler) deletionEligibilityRefusal(live *unstructured.Unstructured, policy *v1alpha1.ZenCleanerPolicy) (string, string) {
	if policy == nil {
		return safety.ReasonNoLongerEligible, "governing policy unavailable for re-evaluation"
	}
	if !r.matchesSelectors(live, &policy.Spec.TargetResource) {
		return safety.ReasonNoLongerEligible, "live object no longer matches target selectors"
	}
	if shouldDelete, reason := r.shouldDelete(live, policy); !shouldDelete {
		return safety.ReasonNoLongerEligible, "live object no longer satisfies " + reason
	}
	return "", ""
}

// deletionOptionsWithPreconditions copies the caller's delete options and
// attaches the server-side UID + resourceVersion preconditions. The input
// options pointer is never mutated (callers may share it across attempts).
//
// PRESERVATION LAW: caller-supplied preconditions are preserved, never
// silently discarded or replaced. A caller precondition that AGREES with the
// decision-time UID / evaluated live resourceVersion is carried (identical
// values); one that CONFLICTS fails closed with an error — the conflicting
// constraint must not be dropped to let the delete proceed.
func deletionOptionsWithPreconditions(deleteOptions *metav1.DeleteOptions, observedUID types.UID, liveResourceVersion string) (*metav1.DeleteOptions, error) {
	var opts *metav1.DeleteOptions
	if deleteOptions != nil {
		opts = deleteOptions.DeepCopy()
	} else {
		opts = &metav1.DeleteOptions{}
	}
	if observedUID == "" {
		return nil, fmt.Errorf("cannot attach UID precondition: observed UID is empty")
	}
	if pre := opts.Preconditions; pre != nil {
		if pre.UID != nil && *pre.UID != observedUID {
			return nil, fmt.Errorf("caller-supplied UID precondition %q conflicts with the decision-time UID %q: refusing to overwrite a conflicting precondition", *pre.UID, observedUID)
		}
		if pre.ResourceVersion != nil && *pre.ResourceVersion != liveResourceVersion {
			return nil, fmt.Errorf("caller-supplied resourceVersion precondition %q conflicts with the evaluated live resourceVersion %q: refusing to overwrite a conflicting precondition", *pre.ResourceVersion, liveResourceVersion)
		}
	}
	rv := liveResourceVersion
	opts.Preconditions = &metav1.Preconditions{
		UID:             &observedUID,
		ResourceVersion: &rv,
	}
	return opts, nil
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

// buildLabelSelectorFilter builds a label selector filter function for an
// informer factory. FAIL-CLOSED LAW (SUPPORT2-033 §5): an invalid selector
// returns an error and the caller must refuse to create or replace any
// informer/list/watch — a silently unfiltered filter would broaden the
// observed scope beyond the policy. An INTENTIONALLY ABSENT selector (nil)
// yields a no-op filter: no selector is pushed down, and policy selection is
// applied per-object at evaluation time (matchesSelectorsShared, which
// independently fails closed on invalid selectors).
//
// NOTE (production reachability): the shared informer factory in
// getOrCreateResourceInformer is deliberately UNFILTERED (no selector is
// supplied), so no production caller supplies a selector today; this helper
// must stay fail-closed so any future wiring cannot silently drop the
// filter on a conversion error.
func buildLabelSelectorFilter(policy *v1alpha1.ZenCleanerPolicy) (func(options *metav1.ListOptions), error) {
	if policy.Spec.TargetResource.LabelSelector == nil {
		return func(options *metav1.ListOptions) {}, nil
	}
	selector, err := metav1.LabelSelectorAsSelector(policy.Spec.TargetResource.LabelSelector)
	if err != nil {
		return nil, fmt.Errorf("invalid label selector: refusing to build an informer filter that would silently drop the selector: %w", err)
	}
	return func(options *metav1.ListOptions) {
		options.LabelSelector = selector.String()
	}, nil
}

// updateConditionReceipt writes an operator-visible condition onto the policy
// status. The write is best-effort receipting, but a dropped receipt is never
// silent: the failure is logged so operators know the condition was lost.
func (r *PolicyReconciler) updateConditionReceipt(ctx context.Context, policy *v1alpha1.ZenCleanerPolicy, conditionType, reason, message string, permanent bool) {
	if r.statusUpdater == nil {
		return
	}
	if err := r.statusUpdater.UpdateTargetCondition(ctx, policy, conditionType, reason, message, permanent); err != nil {
		r.logger.Warn("Failed to update policy condition",
			cleanerlog.Operation("update_target_condition"),
			cleanerlog.String("policy", fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)),
			cleanerlog.Error(err))
	}
}
