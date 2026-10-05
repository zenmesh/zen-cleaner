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
}

// ReadEntitlementStatus builds the record from the gate's own public
// accessor.
func ReadEntitlementStatus(g *entitlegate.CleanerGate) EntitlementStatus {
	return EntitlementStatus{
		EntitlementKey: entitlegate.EntitlementKey,
		MeteredUsage:   g.Executed(),
	}
}

// Render renders the human form.
func (s EntitlementStatus) Render() string {
	return fmt.Sprintf("entitlement: %s\nmetered usage: %d\n", s.EntitlementKey, s.MeteredUsage)
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
