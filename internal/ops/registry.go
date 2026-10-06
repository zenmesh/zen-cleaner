// Copyright 2026 Zen Mesh. All rights reserved.

// Package ops is zen-cleaner's OPERATION REGISTRY (the R051 rotation's
// parity census artifact — the maestro internal/ops pattern applied):
// one typed inventory of what this product SERVES, with the surface
// bindings made explicit and the gaps typed ABSENT rather than UNKNOWN.
//
// The parity law: "backend handlers alone are not parity" — an
// operation exists as parity only when its consumers' surface binding
// exists. The ABSENT bindings are the roadmap, typed here so the next
// session builds against a list, not a vibe.
//
// The controller surface note: zen-cleaner is a Kubernetes CONTROLLER —
// its primary "API" is the k8s API itself (the Cleanup CRDs drive the
// reconciler). The HTTP surface is health/readiness only by design; a
// REST deletion API would be a NEW operation requiring its own
// admission (the §17 front door), never a shortcut.
package ops

// Kind classifies the operation's effect.
type Kind string

const (
	KindMutation Kind = "MUTATION" // effectful: changes the world
	KindRead     Kind = "READ"     // observation only
	KindDryRun   Kind = "DRY_RUN"  // the plan without the effect
)

// Surface names the consumer binding.
type Surface string

const (
	SurfaceK8s Surface = "k8s" // the CRD/reconciliation (the controller's native surface)
	SurfaceCLI Surface = "cli" // the operator binary verbs
	SurfaceAPI Surface = "api" // the HTTP REST
	SurfaceMCP Surface = "mcp" // the MCP tools
	SurfaceUI  Surface = "ui"  // the console screens
)

// Operation is one typed operation with its surface bindings.
type Operation struct {
	ID          string `json:"id"`
	Domain      string `json:"domain"`
	Summary     string `json:"summary"`
	Kind        Kind   `json:"kind"`
	Consequence string `json:"consequence"`
	Idempotency string `json:"idempotency"`
	Auth        string `json:"auth"`
	// ConsequenceClass uses the parity law's closed vocabulary
	// (READ_ONLY | REVERSIBLE_MUTATION | IRREVERSIBLE_MUTATION |
	// SECURITY_SENSITIVE | EXTERNAL_SIDE_EFFECT).
	ConsequenceClass string `json:"consequence_class"`
	// TenantScope states the operation's tenant/environment reach.
	TenantScope string `json:"tenant_environment_scope"`
	// OCC states the optimistic-concurrency posture.
	OCC string `json:"occ"`
	// DestructiveCeremony names the admission ceremony guarding an
	// irreversible effect ("none" for reads).
	DestructiveCeremony string `json:"destructive_ceremony"`
	// Bindings names the consumer surfaces the operation binds today.
	Bindings map[Surface]string `json:"bindings"`
	// AbsentSurfaces types the parity gaps: the surfaces the operation
	// does NOT bind yet, each with the reason (the roadmap, typed).
	AbsentSurfaces map[Surface]string `json:"absent_surfaces,omitempty"`
}

// SurfaceBools is the census-facing per-surface map (the parity grid
// reads it, never infers it).
func (op Operation) SurfaceBools() map[Surface]bool {
	out := map[Surface]bool{SurfaceK8s: false, SurfaceCLI: false, SurfaceAPI: false, SurfaceMCP: false, SurfaceUI: false}
	for s := range op.Bindings {
		out[s] = true
	}
	return out
}

// Registry is the cleaner's operation inventory (ID-sorted; the
// consumer reads, never mutates).
var Registry = []Operation{
	{
		ID:                  "cleaner.config.validate-examples",
		Domain:              "config",
		Summary:             "Validate the example configs (the CLI verb: the config schema and the examples' conformance)",
		Kind:                KindRead,
		Consequence:         "none (the validation only)",
		Idempotency:         "pure",
		Auth:                "local (the operator's own identity)",
		ConsequenceClass:    "READ_ONLY",
		TenantScope:         "local (the operator's own example files)",
		OCC:                 "n/a (pure)",
		DestructiveCeremony: "none",
		Bindings: map[Surface]string{
			SurfaceCLI: "cmd/validate-examples",
		},
		AbsentSurfaces: map[Surface]string{
			SurfaceAPI: "no REST route",
			SurfaceMCP: "no MCP tool",
			SurfaceUI:  "no console screen",
		},
	},
	{
		ID:                  "cleaner.entitlements.check",
		Domain:              "entitlements",
		Summary:             "The CleanerGate admission: the policy (the verified snapshot) -> the quota (this generation's window) -> the metered usage (cleaner.executions)",
		Kind:                KindRead,
		Consequence:         "admits or refuses the batch; the usage meters",
		Idempotency:         "the check is pure; the metered usage counts once per admission",
		Auth:                "the verified policy snapshot (the entitlegate's generation binding)",
		ConsequenceClass:    "READ_ONLY",
		TenantScope:         "the tenant bound in the verified snapshot",
		OCC:                 "the snapshot's generation binding (a stale snapshot refuses)",
		DestructiveCeremony: "none",
		Bindings: map[Surface]string{
			SurfaceK8s: "in-process (the reconciler's ExecuteWithReceipt calls Admit before the delete path)",
			SurfaceCLI: "-entitlement-status -entitlement-snapshot <file> (the fail-closed snapshot verification + the identity render; verified live s105/s121)",
			SurfaceMCP: "the entitlement_status tool (the read-only family; the stdio posture answers the empty body when no provider is wired)",
		},
		AbsentSurfaces: map[Surface]string{
			SurfaceAPI: "no REST admission route",
			SurfaceUI:  "no console screen",
		},
	},
	{
		ID:                  "cleaner.executions.delete-batch",
		Domain:              "executions",
		Summary:             "Execute one entitlement-gated deletion batch (policy -> quota -> metered usage -> the rate-limited delete path)",
		Kind:                KindMutation,
		Consequence:         "deletes the matched resources; every deletion lands in the receipt",
		Idempotency:         "the reconciler's idempotent loop (the batch re-runs converge; the entitlegate counts one execution per admission)",
		Auth:                "the service account (the controller's own identity) + the CleanerGate's verified policy snapshot",
		ConsequenceClass:    "IRREVERSIBLE_MUTATION",
		TenantScope:         "the CRD's namespace scope (the controller's watched cluster)",
		OCC:                 "resourceVersion-guarded status writes; the idempotent reconcile loop converges re-runs",
		DestructiveCeremony: "the entitlement-gated admission (policy -> quota -> metered usage) precedes every delete path; DryRun plans first",
		Bindings: map[Surface]string{
			SurfaceK8s: "the Cleanup CRD reconciliation (pkg/controller: the PolicyReconciler.ExecuteWithReceipt through the entitlegate's CleanerGate)",
		},
		AbsentSurfaces: map[Surface]string{
			SurfaceCLI: "no operator verb exists (the controller runs headless)",
			SurfaceAPI: "no REST deletion route BY DESIGN (a REST deletion API would be a new operation requiring its own §17 admission — never a shortcut)",
			SurfaceMCP: "no MCP tool (the task_start family is maestro's; a cleaner tool would be the new typed operation)",
			SurfaceUI:  "no console screen",
		},
	},
	{
		ID:                  "cleaner.executions.dry-run",
		Domain:              "executions",
		Summary:             "Plan one deletion batch WITHOUT effects (the DryRun behavior: the plan contract, the batch test pins delete-must-not-delete)",
		Kind:                KindDryRun,
		Consequence:         "none (the plan only — the contract test pins delete-must-not-delete under DryRun)",
		Idempotency:         "pure (the same input -> the same plan)",
		Auth:                "the service account",
		ConsequenceClass:    "READ_ONLY",
		TenantScope:         "the CRD's namespace scope",
		OCC:                 "n/a (the same input plans the same batch)",
		DestructiveCeremony: "none (the contract test pins delete-must-not-delete under DryRun)",
		Bindings: map[Surface]string{
			SurfaceK8s: "the Cleanup CRD's DryRun behavior (pkg/controller: the batch contract)",
		},
		AbsentSurfaces: map[Surface]string{
			SurfaceCLI: "no dry-run verb",
			SurfaceAPI: "no REST plan route",
			SurfaceMCP: "no MCP tool",
			SurfaceUI:  "no console screen",
		},
	},
	{
		ID:                  "cleaner.health.read",
		Domain:              "health",
		Summary:             "The liveness/readiness surface (the controller's own health)",
		Kind:                KindRead,
		Consequence:         "none",
		Idempotency:         "pure",
		Auth:                "the cluster-internal (the health endpoints are not auth-gated by design; the cluster boundary is the gate)",
		ConsequenceClass:    "READ_ONLY",
		TenantScope:         "the controller process itself",
		OCC:                 "n/a",
		DestructiveCeremony: "none",
		Bindings: map[Surface]string{
			SurfaceK8s: "the health endpoints (internal/health)",
			SurfaceCLI: "-health-summary (the process-local posture: version/commit/uptime; verified live s105/s121)",
			SurfaceMCP: "the health_summary tool (the read-only family)",
		},
		AbsentSurfaces: map[Surface]string{
			SurfaceAPI: "the health endpoints ARE the HTTP surface (the only one, by design)",
			SurfaceUI:  "no console screen",
		},
	},
}

// AbsentSurfacesNote is the registry's parity note: the ABSENT
// bindings are the roadmap, typed here so the next session builds
// against a list, not a vibe.
const AbsentSurfacesNote = "the ABSENT bindings are the roadmap: the CLI and the MCP tools are the highest-value parity additions (the operators and the agents); the REST API is typed absent BY DESIGN for the deletions (the §17 front door) and owed for the reads"
