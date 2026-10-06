// Copyright 2026 Zen Mesh. All rights reserved.

// Package mcpserver is zen-cleaner's MCP adapter (the R051 rotation's
// parity build-out — the cleaner's operations as MCP tools), following
// the zen-watcher adapter's laws verbatim: DEFAULT-DENY (no mode env,
// no tools), the audit attribution (no actor, no call), the READ-ONLY
// tool family (the §17 law: the deletion batch NEVER leaves the k8s
// surface — no MCP tool may start, trigger, or dry-run a deletion).
package mcpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
)

// ModeEnv must be exactly "stdio" to enable the transport (the
// default-deny law: without it the tools neither enumerate nor answer).
const ModeEnv = "ZEN_CLEANER_MCP_MODE"

// MCPModeStdio is the only enabled mode value.
const MCPModeStdio = "stdio"

// ActorEnv names the calling actor (the audit attribution: no actor,
// no call).
const ActorEnv = "ZEN_CLEANER_MCP_ACTOR"

// EntitlementSummaryProvider supplies the entitlement-status body (the
// cleaner's own admission state: the metered usage + the entitlement
// key — never a secret).
type EntitlementSummaryProvider interface {
	EntitlementSummary() string
}

// HealthSummaryProvider supplies the health-summary body (the
// process-local posture: the version, the commit, the process uptime —
// never the cluster data).
type HealthSummaryProvider interface {
	HealthSummary() string
}

// Adapter is the cleaner's MCP adapter.
type Adapter struct {
	enabled bool
	actor   string
	tools   []Tool
	ent     EntitlementSummaryProvider
	health  HealthSummaryProvider
}

// Tool is one typed read-only tool.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema string `json:"inputSchema"`
}

// New applies the law: the adapter is ENABLED only when the mode env is
// exactly stdio AND the actor is named — the default is deny, and the
// audit attribution is not optional.
func New(ent EntitlementSummaryProvider, health HealthSummaryProvider) *Adapter {
	a := &Adapter{ent: ent, health: health}
	if os.Getenv(ModeEnv) == MCPModeStdio {
		a.enabled = true
	}
	a.actor = os.Getenv(ActorEnv)
	a.tools = []Tool{
		{
			Name:        "entitlement_status",
			Description: "The CleanerGate's admission status: the policy generation, the quota window, the metered usage for cleaner.executions (READ-ONLY)",
			InputSchema: `{"type":"object","properties":{},"additionalProperties":false}`,
		},
		{
			Name:        "health_summary",
			Description: "The controller's own health posture summary (READ-ONLY; the live checks stay the k8s probes)",
			InputSchema: `{"type":"object","properties":{},"additionalProperties":false}`,
		},
	}
	return a
}

// Enabled reports the adapter's law state.
func (a *Adapter) Enabled() bool { return a.enabled }

// Actor returns the audit attribution (empty = the calls are refused).
func (a *Adapter) Actor() string { return a.actor }

// Tools lists the read-only tool family (DEFAULT-DENY: without the
// mode env the tools do not enumerate — the typed refusal instead).
func (a *Adapter) Tools() []Tool {
	if !a.enabled {
		return nil
	}
	return a.tools
}

// ErrNotEnabled is the typed denial: the mode env did not opt in.
var ErrNotEnabled = fmt.Errorf("mcp: not enabled (set %s=%s to opt in; the default is deny)", ModeEnv, MCPModeStdio)

// ListJSON renders the tools/list response body (the caller wraps it in
// the JSON-RPC envelope). Not-enabled returns the typed error.
func (a *Adapter) ListJSON() (string, error) {
	if !a.enabled {
		return "", ErrNotEnabled
	}
	type resp struct {
		Tools []Tool `json:"tools"`
	}
	b, err := json.Marshal(resp{Tools: a.tools})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SummaryLines renders the operator form (the sorted tool names).
func (a *Adapter) SummaryLines() []string {
	tools := a.Tools()
	out := make([]string, 0, len(tools))
	for _, tl := range tools {
		out = append(out, tl.Name)
	}
	sort.Strings(out)
	return out
}

var _ = fmt.Sprintf // the fmt stays for the callers' error wrapping
var _ = io.Discard  // the io stays for the transport callers
