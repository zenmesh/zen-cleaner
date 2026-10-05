// Copyright 2026 Zen Mesh. All rights reserved.

// Package clireadops is the cleaner's read-only CLI verbs' engine (the
// R051 parity build-out): entitlement-status and health-summary as the
// typed, testable functions — the CLI wiring stays thin, the laws live
// here.
//
// The laws: the status reads ONLY the gate's own admission state (the
// metered usage counter and the entitlement key — never a secret); the
// health summary reads ONLY the process-local posture (the version,
// the commit, the process uptime — never the cluster data).
package clireadops

import (
	"fmt"
	"time"

	"github.com/zenmesh/zen-cleaner/internal/entitlegate"
)

// EntitlementStatus is the typed entitlement-status record (never a
// secret: the entitlement key and the metered usage counter).
type EntitlementStatus struct {
	EntitlementKey string `json:"entitlement_key"`
	MeteredUsage   int64  `json:"metered_usage"`
	SnapshotID     string `json:"snapshot_id"`
	Revision       int64  `json:"revision"`
	IssuedAt       string `json:"issued_at"`
	ExpiresAt      string `json:"expires_at"`
	InGrace        bool   `json:"in_grace"`
}

// ReadEntitlementStatus builds the record from the gate's own public
// accessors (the identity fields, never the signature material).
func ReadEntitlementStatus(g *entitlegate.CleanerGate) EntitlementStatus {
	id, rev, issued, expires, inGrace := g.StatusSnapshot()
	return EntitlementStatus{
		EntitlementKey: entitlegate.EntitlementKey,
		MeteredUsage:   g.Executed(),
		SnapshotID:     id,
		Revision:       rev,
		IssuedAt:       issued,
		ExpiresAt:      expires,
		InGrace:        inGrace,
	}
}

// Render renders the human form.
func (s EntitlementStatus) Render() string {
	base := fmt.Sprintf("entitlement: %s\nmetered usage: %d\n", s.EntitlementKey, s.MeteredUsage)
	if s.SnapshotID == "" {
		return base
	}
	return base + fmt.Sprintf("snapshot: %s (revision %d)\nwindow: %s .. %s\nin grace: %t\n",
		s.SnapshotID, s.Revision, s.IssuedAt, s.ExpiresAt, s.InGrace)
}

// HealthSummary is the typed health-summary record (the process-local
// posture: the version, the commit, the process uptime — never the
// cluster data).
type HealthSummary struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	ProcessUptime string `json:"process_uptime"`
}

var processStart = time.Now()

// ReadHealthSummary builds the summary from the process-local facts.
func ReadHealthSummary(version, commit string) HealthSummary {
	return HealthSummary{
		Version:       version,
		Commit:        commit,
		ProcessUptime: time.Since(processStart).Round(time.Second).String(),
	}
}

// Render renders the human form.
func (s HealthSummary) Render() string {
	return fmt.Sprintf("version: %s\ncommit: %s\nprocess uptime: %s\n", s.Version, s.Commit, s.ProcessUptime)
}

// StatusFromSnapshot builds the entitlement-status record from a
// VERIFIED snapshot (the caller runs the fail-closed verification; the
// record carries the identity fields, never the signature material).
func StatusFromSnapshot(snap EntitlementSnapshotView, inGrace bool) EntitlementStatus {
	return EntitlementStatus{
		EntitlementKey: entitlegate.EntitlementKey,
		SnapshotID:     snap.ID,
		Revision:       snap.Revision,
		IssuedAt:       snap.IssuedAt,
		ExpiresAt:      snap.ExpiresAt,
		InGrace:        inGrace,
	}
}

// entitlementSnapshotView is the narrow view this package reads from
// the sdk's verified snapshot (the structural decoupling: the cleaner
// does not import the snapshot type into its own API).
type EntitlementSnapshotView struct {
	ID        string
	Revision  int64
	IssuedAt  string
	ExpiresAt string
}
