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

// HELPER-H258+ H2: receipt-bearing execution path. Wires the existing
// ExecutionReceipt model into the REAL delete execution path with:
//
//   policy binding      (PolicyRef: namespace/name/UID of the governing
//                        ZenCleanerPolicy)
//   plan binding        (PlanDigest over the planned UID set)
//   effect classification by OBSERVED state, never by requested action:
//     SUCCEEDED                        object absent after the delete path
//     ALREADY_ABSENT                   absent before the delete path ran
//     SKIPPED_PRECONDITION_MISMATCH    UID changed since planning
//     FAILED                           still present with the planned UID
//   dry-run law         a dry-run emits NO receipt (no fake success)
//   restart law         a re-planned run classifies already-deleted objects
//                       as ALREADY_ABSENT — exactly one SUCCEEDED receipt
//                       per object lifetime

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"
	gcapi "github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

// policyRef binds the receipt to its governing policy.
type policyRef struct {
	Namespace string    `json:"namespace,omitempty"`
	Name      string    `json:"name,omitempty"`
	UID       types.UID `json:"uid,omitempty"`
}

// ExecutionReceiptV2 extends the receipt with the policy binding (additive,
// backward-compatible field) without changing deletion semantics.
type ExecutionReceiptV2 struct {
	ExecutionReceipt
	PolicyRef policyRef `json:"policyRef"`
}

// ExecuteWithReceipt runs the existing batch delete path (rate limiting,
// backoff, events, dry-run accounting unchanged) and emits a receipt whose
// per-object outcomes are classified from OBSERVED state.
//
// DRY-RUN LAW: when the policy is dry-run, this function returns a nil
// receipt — a dry-run must never emit a (fake) successful effect receipt.
func (r *PolicyReconciler) ExecuteWithReceipt(
	ctx context.Context,
	batch []*unstructured.Unstructured,
	policy *gcapi.ZenCleanerPolicy,
	rateLimiter *ratelimiter.RateLimiter,
	reasons map[string]string,
) (int64, []error, *ExecutionReceiptV2) {
	if policy.Spec.Behavior.DryRun {
		// Delegate for identical dry-run accounting/events, emit NO receipt.
		n, errs := deleteBatchShared(ctx, batch, policy, rateLimiter, reasons, r)
		return n, errs, nil
	}

	receipt := &ExecutionReceiptV2{
		ExecutionReceipt: ExecutionReceipt{Version: ReceiptVersion, StartedAt: time.Now().UTC()},
		PolicyRef: policyRef{Namespace: policy.Namespace, Name: policy.Name, UID: policy.UID},
	}
	plannedUID := map[string]types.UID{}
	prePresent := map[string]bool{} // object existed before execution
	for _, o := range batch {
		plannedUID[objectKey(o)] = o.GetUID()
		gvr := r.resolveGVRForDeletion(o)
		var perr error
		if o.GetNamespace() == "" {
			_, perr = r.dynamicClient.Resource(gvr).Get(ctx, o.GetName(), metav1.GetOptions{})
		} else {
			_, perr = r.dynamicClient.Resource(gvr).Namespace(o.GetNamespace()).Get(ctx, o.GetName(), metav1.GetOptions{})
		}
		prePresent[objectKey(o)] = perr == nil
	}
	planned := make([]unstructured.Unstructured, len(batch))
	for i, o := range batch {
		planned[i] = *o
	}
	receipt.PlanDigest = PlanDigest(planned)

	deleted, errs := deleteBatchShared(ctx, batch, policy, rateLimiter, reasons, r)

	// Classify from OBSERVED state (effect took effect vs did not), one
	// result per planned object. The read uses the SAME dynamic client the
	// delete path acted on — never a different authority view.
	for _, o := range batch {
		res := ObjectResult{Namespace: o.GetNamespace(), Kind: o.GetKind(), Name: o.GetName(), UID: o.GetUID()}
		gvr := r.resolveGVRForDeletion(o)
		var current *unstructured.Unstructured
		var err error
		if o.GetNamespace() == "" {
			current, err = r.dynamicClient.Resource(gvr).Get(ctx, o.GetName(), metav1.GetOptions{})
		} else {
			current, err = r.dynamicClient.Resource(gvr).Namespace(o.GetNamespace()).Get(ctx, o.GetName(), metav1.GetOptions{})
		}
		switch {
		case errors.IsNotFound(err):
			if prePresent[objectKey(o)] {
				res.Class = ResultSucceeded
				res.Detail = "absent after execution (effect confirmed)"
			} else {
				res.Class = ResultAlreadyAbsent
				res.Detail = "already absent (idempotent)"
			}
		case err != nil:
			res.Class = ResultFailed
			res.Detail = "post-execution read failed: " + err.Error()
		case uidMismatch(plannedUID[objectKey(o)], current.GetUID()):
			res.Class = ResultSkippedPreconditionMismatch
			res.Detail = "UID changed since planning"
		default:
			res.Class = ResultFailed
			res.Detail = "still present with planned UID after delete path"
		}
		receipt.Results = append(receipt.Results, res)
	}
	receipt.CompletedAt = time.Now().UTC()
	receipt.OverallStatus = computeOverall(receipt.Results)
	_ = deleted
	return deleted, errs, receipt
}

func objectKey(o *unstructured.Unstructured) string {
	return o.GetNamespace() + "/" + o.GetKind() + "/" + o.GetName()
}

func uidMismatch(planned, current types.UID) bool {
	return planned != types.UID("") && current != planned
}

