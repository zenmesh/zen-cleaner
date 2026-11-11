package controller

// SUPPORT2_026 — deterministic effect-executor primitives.
//
// Law: an execution receipt records what Cleaner ATTEMPTED/PERFORMED. It is
// NOT remediation verification — independent postcondition observation
// (Mon, later) owns that. Execution success must never be labeled VERIFIED.

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ReceiptVersion is the execution receipt schema version.
const ReceiptVersion = "v1"

// ObjectResultClass classifies a per-object outcome.
type ObjectResultClass string

const (
	ResultSucceeded                   ObjectResultClass = "SUCCEEDED"
	ResultAlreadyAbsent               ObjectResultClass = "ALREADY_ABSENT"
	ResultSkippedPreconditionMismatch ObjectResultClass = "SKIPPED_PRECONDITION_MISMATCH"
	ResultFailed                      ObjectResultClass = "FAILED"
)

// ObjectResult is one bounded per-object outcome.
type ObjectResult struct {
	Namespace string            `json:"namespace"`
	Kind      string            `json:"kind"`
	Name      string            `json:"name"`
	UID       types.UID         `json:"uid,omitempty"`
	Class     ObjectResultClass `json:"class"`
	Detail    string            `json:"detail,omitempty"`
}

// ExecutionReceipt is the machine-readable record of one bounded execution.
// It contains digests and references only — no secrets, no object dumps.
type ExecutionReceipt struct {
	Version       string         `json:"receiptVersion"`
	PlanDigest    string         `json:"planDigest"`
	StartedAt     time.Time      `json:"startedAt"`
	CompletedAt   time.Time      `json:"completedAt"`
	Results       []ObjectResult `json:"results"`
	OverallStatus string         `json:"overallStatus"`
}

// Overall computation law: SUCCEEDED only if every object succeeded or was
// already absent; any precondition mismatch or failure degrades honestly.
func computeOverall(results []ObjectResult) string {
	status := "SUCCEEDED"
	for _, r := range results {
		switch r.Class {
		case ResultFailed:
			return "FAILED"
		case ResultSkippedPreconditionMismatch:
			status = "PARTIAL"
		}
	}
	return status
}

// PlanDigest computes an order-independent canonical digest of the planned
// object set. Kubernetes list ordering must not affect it.
func PlanDigest(objects []unstructured.Unstructured) string {
	ids := make([]string, 0, len(objects))
	for _, o := range objects {
		ids = append(ids, fmt.Sprintf("%s/%s/%s/%s", o.GetNamespace(), o.GetKind(), o.GetName(), o.GetUID()))
	}
	sort.Strings(ids)
	h := sha256.Sum256([]byte(fmt.Sprintf("%v", ids)))
	return fmt.Sprintf("%x", h[:16])
}

// DeleteWithReceipt deletes each planned object with server-side UID and
// resourceVersion preconditions (stale-plan protection: a replaced object is
// skipped, not deleted; a same-UID update after the verification read
// refuses atomically instead of deleting the changed object) and honest
// ALREADY_ABSENT idempotency. A planned object with an EMPTY UID refuses:
// without a UID no precondition can protect the delete, and delete-by-name
// is never silently degraded to. Results are per-object; one object's
// failure does not abort the bounded set.
func DeleteWithReceipt(ctx context.Context, c client.Client, planned []unstructured.Unstructured) ExecutionReceipt {
	receipt := ExecutionReceipt{Version: ReceiptVersion, StartedAt: time.Now().UTC()}
	for _, obj := range planned {
		res := ObjectResult{Namespace: obj.GetNamespace(), Kind: obj.GetKind(), Name: obj.GetName(), UID: obj.GetUID()}
		if obj.GetUID() == types.UID("") {
			// Fail closed: no delete-by-name without a preconditionable UID.
			res.Class = ResultSkippedPreconditionMismatch
			res.Detail = "planned object carries no UID; delete-by-name refused"
			receipt.Results = append(receipt.Results, res)
			continue
		}
		current := &unstructured.Unstructured{}
		current.SetGroupVersionKind(obj.GroupVersionKind())
		err := c.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, current)
		switch {
		case errors.IsNotFound(err):
			res.Class = ResultAlreadyAbsent
			res.Detail = "already absent"
		case err != nil:
			res.Class = ResultFailed
			res.Detail = "get failed: " + err.Error()
		case current.GetUID() != obj.GetUID():
			res.Class = ResultSkippedPreconditionMismatch
			res.Detail = "UID changed since plan (stale plan protection)"
		default:
			// Preconditions bind the delete to exactly the planned UID and
			// the object version actually verified above; anything landing
			// between the verification read and the DELETE is rejected
			// server-side as a conflict.
			plannedUID := obj.GetUID()
			verifiedRV := current.GetResourceVersion()
			err := c.Delete(ctx, current, client.Preconditions{
				UID:             &plannedUID,
				ResourceVersion: &verifiedRV,
			})
			switch {
			case err == nil:
				res.Class = ResultSucceeded
			case errors.IsNotFound(err):
				res.Class = ResultAlreadyAbsent
				res.Detail = "already absent"
			case errors.IsConflict(err):
				res.Class = ResultSkippedPreconditionMismatch
				res.Detail = "precondition conflict: object replaced or updated since plan (stale plan protection)"
			default:
				res.Class = ResultFailed
				res.Detail = "delete failed: " + err.Error()
			}
		}
		receipt.Results = append(receipt.Results, res)
	}
	receipt.CompletedAt = time.Now().UTC()
	receipt.PlanDigest = PlanDigest(planned)
	receipt.OverallStatus = computeOverall(receipt.Results)
	return receipt
}

// ExecutionIsVerification is a compile-time documentation anchor: Cleaner
// receipts record attempts/performed effects. Postcondition verification is
// a separate concern owned by independent observation (Mon, later).
const ExecutionIsVerification = false
