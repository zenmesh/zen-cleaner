// Copyright 2026 Zen Mesh. All rights reserved.

// The read-only CLI verbs (the R051 parity build-out): the operator
// verbs over the clireadops engine. The verbs print and exit — the
// controller's run loop never starts for them.
//
// -health-summary: the process-local posture (always available).
// -entitlement-status -entitlement-snapshot <file>: the verified
// entitlement snapshot's status (the fail-closed verification — the
// operator passes the snapshot file; the verb verifies and renders).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	entitlement "github.com/zenmesh/zen-sdk/pkg/entitlement"

	"github.com/zenmesh/zen-cleaner/internal/clireadops"
)

var (
	entitlementStatusVerb  = flag.Bool("entitlement-status", false, "Print the entitlement status (requires -entitlement-snapshot <file>)")
	entitlementSnapshotFlg = flag.String("entitlement-snapshot", "", "Path to the signed entitlement snapshot JSON")
	healthSummaryVerb      = flag.Bool("health-summary", false, "Print the process-local health summary (the version, the commit, the uptime)")
)

// runStatusVerbs handles the read-only verbs; it returns true when a
// verb fired (the caller exits immediately).
func runStatusVerbs() bool {
	if *healthSummaryVerb {
		fmt.Print(clireadops.ReadHealthSummary(version, commit).Render())
		return true
	}
	if *entitlementStatusVerb {
		return runEntitlementStatus()
	}
	return false
}

// runEntitlementStatus verifies the snapshot fail-closed and renders
// the status (the Result's identity fields, never the signature
// material).
func runEntitlementStatus() bool {
	if *entitlementSnapshotFlg == "" {
		fmt.Fprintln(os.Stderr, "zen-cleaner: -entitlement-status requires -entitlement-snapshot <file>")
		return true
	}
	raw, err := os.ReadFile(*entitlementSnapshotFlg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zen-cleaner:", err)
		return true
	}
	var ss entitlement.SignedSnapshot
	if err := json.Unmarshal(raw, &ss); err != nil {
		fmt.Fprintf(os.Stderr, "zen-cleaner: the snapshot file is not valid JSON: %v\n", err)
		return true
	}
	// The verification pins nothing here (the operator's own file): the
	// schema/shape/algorithm laws run; the key pinning is the deploy
	// config's law.
	res, err := entitlement.Verify(nil, &ss, time.Now().UTC(), entitlement.VerifyOpts{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "zen-cleaner: the snapshot FAILED verification: %v\n", err)
		return true
	}
	s := clireadops.StatusFromSnapshot(clireadops.EntitlementSnapshotView{
		ID:        res.Snapshot.SnapshotID,
		Revision:  res.Snapshot.Revision,
		IssuedAt:  res.Snapshot.IssuedAt,
		ExpiresAt: res.Snapshot.ExpiresAt,
	}, res.InGrace)
	fmt.Print(s.Render())
	return true
}
