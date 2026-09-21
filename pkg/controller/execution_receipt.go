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
	type ident struct{ ns, kind, name, uid string }
	ids := make([]string, 0, len(objects))
	for _, o := range objects {
		ids = append(ids, fmt.Sprintf("%s/%s/%s/%s", o.GetNamespace(), o.GetKind(), o.GetName(), o.GetUID()))
	}
	sort.Strings(ids)
	h := sha256.Sum256([]byte(fmt.Sprintf("%v", ids)))
	return fmt.Sprintf("%x", h[:16])
}

// DeleteWithReceipt deletes each planned object with a UID precondition
// (stale-plan protection: a replaced object is skipped, not deleted) and
// honest ALREADY_ABSENT idempotency. Results are per-object; one object's
// failure does not abort the bounded set.
func DeleteWithReceipt(ctx context.Context, c client.Client, planned []unstructured.Unstructured) ExecutionReceipt {
	receipt := ExecutionReceipt{Version: ReceiptVersion, StartedAt: time.Now().UTC()}
	for _, obj := range planned {
		res := ObjectResult{Namespace: obj.GetNamespace(), Kind: obj.GetKind(), Name: obj.GetName(), UID: obj.GetUID()}
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
		case obj.GetUID() != types.UID("") && current.GetUID() != obj.GetUID():
			res.Class = ResultSkippedPreconditionMismatch
			res.Detail = "UID changed since plan (stale plan protection)"
		default:
			if err := c.Delete(ctx, current); err != nil {
				if errors.IsNotFound(err) {
					res.Class = ResultAlreadyAbsent
					res.Detail = "already absent"
				} else {
					res.Class = ResultFailed
					res.Detail = "delete failed: " + err.Error()
				}
			} else {
				res.Class = ResultSucceeded
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
