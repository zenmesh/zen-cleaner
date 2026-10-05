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
	// Bindings names the consumer surfaces the operation binds today.
	Bindings map[Surface]string `json:"bindings"`
	// AbsentSurfaces types the parity gaps: the surfaces the operation
	// does NOT bind yet, each with the reason (the roadmap, typed).
	AbsentSurfaces map[Surface]string `json:"absent_surfaces,omitempty"`
}

// Registry is the cleaner's operation inventory (ID-sorted; the
// consumer reads, never mutates).
var Registry = []Operation{
	{
		ID:          "cleaner.config.validate-examples",
		Domain:      "config",
		Summary:     "Validate the example configs (the CLI verb: the config schema and the examples' conformance)",
		Kind:        KindRead,
		Consequence: "none (the validation only)",
		Idempotency: "pure",
		Auth:        "local (the operator's own identity)",
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
		ID:          "cleaner.entitlements.check",
		Domain:      "entitlements",
		Summary:     "The CleanerGate admission: the policy (the verified snapshot) -> the quota (this generation's window) -> the metered usage (cleaner.executions)",
		Kind:        KindRead,
		Consequence: "admits or refuses the batch; the usage meters",
		Idempotency: "the check is pure; the metered usage counts once per admission",
		Auth:        "the verified policy snapshot (the entitlegate's generation binding)",
		Bindings: map[Surface]string{
			SurfaceK8s: "in-process (the reconciler's ExecuteWithReceipt calls Admit before the delete path)",
		},
		AbsentSurfaces: map[Surface]string{
			SurfaceCLI: "no admission-check verb",
			SurfaceAPI: "no REST admission route",
			SurfaceMCP: "no MCP tool",
			SurfaceUI:  "no console screen",
		},
	},
	{
		ID:          "cleaner.executions.delete-batch",
		Domain:      "executions",
		Summary:     "Execute one entitlement-gated deletion batch (policy -> quota -> metered usage -> the rate-limited delete path)",
		Kind:        KindMutation,
		Consequence: "deletes the matched resources; every deletion lands in the receipt",
		Idempotency: "the reconciler's idempotent loop (the batch re-runs converge; the entitlegate counts one execution per admission)",
		Auth:        "the service account (the controller's own identity) + the CleanerGate's verified policy snapshot",
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
		ID:          "cleaner.executions.dry-run",
		Domain:      "executions",
		Summary:     "Plan one deletion batch WITHOUT effects (the DryRun behavior: the plan contract, the batch test pins delete-must-not-delete)",
		Kind:        KindDryRun,
		Consequence: "none (the plan only — the contract test pins delete-must-not-delete under DryRun)",
		Idempotency: "pure (the same input -> the same plan)",
		Auth:        "the service account",
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
		ID:          "cleaner.health.read",
		Domain:      "health",
		Summary:     "The liveness/readiness surface (the controller's own health)",
		Kind:        KindRead,
		Consequence: "none",
		Idempotency: "pure",
		Auth:        "the cluster-internal (the health endpoints are not auth-gated by design; the cluster boundary is the gate)",
		Bindings: map[Surface]string{
			SurfaceK8s: "the health endpoints (internal/health)",
		},
		AbsentSurfaces: map[Surface]string{
			SurfaceCLI: "no health verb (kubectl observes)",
			SurfaceAPI: "the health endpoints ARE the HTTP surface (the only one, by design)",
			SurfaceMCP: "no MCP tool",
			SurfaceUI:  "no console screen",
		},
	},
}

// AbsentSurfacesNote is the registry's parity note: the ABSENT
// bindings are the roadmap, typed here so the next session builds
// against a list, not a vibe.
const AbsentSurfacesNote = "the ABSENT bindings are the roadmap: the CLI and the MCP tools are the highest-value parity additions (the operators and the agents); the REST API is typed absent BY DESIGN for the deletions (the §17 front door) and owed for the reads"
