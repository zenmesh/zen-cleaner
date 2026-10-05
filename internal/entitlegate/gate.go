// Package entitlegate is zen-cleaner's entitlement enforcement point
// for deletion executions (H2 custody-block cell: the shared engine
// adopted per product). Every deletion batch passes through
// CleanerGate.Execute: policy (verified snapshot) -> quota (this
// generation's window) -> metered usage. Wiring: the reconciler's
// ExecuteWithReceipt calls Admit before deleteBatchShared.
package entitlegate

import (
	"context"
	"fmt"

	"github.com/zenmesh/zen-sdk/pkg/entitlement"
	"github.com/zenmesh/zen-sdk/pkg/entitlement/engine"
)


// StatusSnapshot exposes the verified policy snapshot's identity fields
// (the ID, the revision, the window — never the signature material).
func (g *CleanerGate) StatusSnapshot() (id string, revision int64, issuedAt, expiresAt string, inGrace bool) {
	return g.res.Snapshot.SnapshotID, g.res.Snapshot.Revision, g.res.Snapshot.IssuedAt, g.res.Snapshot.ExpiresAt, g.res.InGrace
}

// EntitlementKey is the single capability zen-cleaner meters.
const EntitlementKey = "cleaner.executions"

// CleanerGate binds one verified snapshot generation to its enforcement
// window. A new snapshot generation means a new gate (counters never
// straddle generations — the engine's law).
type CleanerGate struct {
	res   *entitlement.Result
	quota *engine.Quota
	meter *engine.Meter
}

// New binds the gate (nil meter = enforcement-only mode).
func New(res *entitlement.Result, meter *engine.Meter) *CleanerGate {
	return &CleanerGate{res: res, quota: engine.NewQuota(), meter: meter}
}

// Executed reports this window's admitted deletion batches.
func (g *CleanerGate) Executed() int64 { return g.quota.Used(EntitlementKey) }

// Execute admits one deletion batch under the org's entitlements. The
// execute function runs only after policy+quota admit it; a failed
// batch releases its reservation (failed deletions never bill); success
// meters one usage event (the batch is ONE custody action regardless of
// object count — per-object accounting lives in the execution receipt).
// Refusals are typed (ErrDenied with a closed-vocabulary reason,
// ErrQuotaExhausted when the generation's window is spent).
func (g *CleanerGate) Execute(ctx context.Context, planDigest string, execute func(context.Context) error) error {
	_, err := engine.Enforce(ctx, g.res, g.quota, EntitlementKey, 1, true, g.meter,
		func(ctx context.Context) error {
			if planDigest == "" {
				return fmt.Errorf("plan digest is required (approval binding)")
			}
			return execute(ctx)
		})
	return err
}
