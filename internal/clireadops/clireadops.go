// Package clireadops provides process-local health diagnostics.
package clireadops

import (
	"fmt"
	"time"
)

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
