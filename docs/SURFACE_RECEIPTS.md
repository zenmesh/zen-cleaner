# The cleaner's 4-surface receipts per operation (R051 LH-OC1)

The receipt table from the operation registry's list (internal/ops/
registry.go): each operation × each surface = LIVE / ABSENT-BY-DESIGN
/ ABSENT-ROADMAP, with the evidence. The parity law: backend handlers
alone are not parity — the receipts are the concrete census.

## cleaner.executions.delete-batch (MUTATION)

| Surface | State | Evidence |
|---|---|---|
| k8s | **LIVE** | the Cleanup CRD reconciliation (pkg/controller: the PolicyReconciler.ExecuteWithReceipt through the entitlegate's CleanerGate); the batch test pins delete-must-not-delete under DryRun |
| api | ABSENT-BY-DESIGN | no REST deletion route — a REST deletion API would be a new operation requiring its own §17 admission, never a shortcut |
| cli | ABSENT-ROADMAP | no operator verb (the controller runs headless) |
| mcp | ABSENT-ROADMAP | the task_start family is maestro's; a cleaner tool would be the new typed operation |
| ui | ABSENT-ROADMAP | no console screen |

## cleaner.executions.dry-run (DRY_RUN)

| Surface | State | Evidence |
|---|---|---|
| k8s | **LIVE** | the Cleanup CRD's DryRun behavior (pkg/controller: the batch contract — delete-must-not-delete pinned by test) |
| api | ABSENT-ROADMAP | no REST plan route |
| cli | ABSENT-ROADMAP | no dry-run verb |
| mcp | ABSENT-ROADMAP | no MCP tool |
| ui | ABSENT-ROADMAP | no console screen |

## cleaner.entitlements.check (READ)

| Surface | State | Evidence |
|---|---|---|
| k8s | **LIVE** | in-process (the reconciler's ExecuteWithReceipt calls Admit before the delete path) |
| api | ABSENT-ROADMAP | no REST admission route |
| cli | ABSENT-ROADMAP | no admission-check verb |
| mcp | ABSENT-ROADMAP | no MCP tool |
| ui | ABSENT-ROADMAP | no console screen |

## cleaner.health.read (READ)

| Surface | State | Evidence |
|---|---|---|
| k8s | **LIVE** | the health endpoints (internal/health) — the controller's own health |
| api | ABSENT-BY-DESIGN | the health endpoints ARE the HTTP surface (the only one, by design) |
| cli | ABSENT-ROADMAP | no health verb (kubectl observes) |
| mcp | ABSENT-ROADMAP | no MCP tool |
| ui | ABSENT-ROADMAP | no console screen |

## cleaner.config.validate-examples (READ)

| Surface | State | Evidence |
|---|---|---|
| cli | **LIVE** | cmd/validate-examples (the config schema and the examples' conformance; the Q5 journey exercises it) |
| api | ABSENT-ROADMAP | no REST route |
| k8s | ABSENT (the config validation is local by nature) |
| mcp | ABSENT-ROADMAP | no MCP tool |
| ui | ABSENT-ROADMAP | no console screen |

## The summary

LIVE: 5 (the k8s surface ×4 + the CLI ×1). ABSENT-BY-DESIGN: 2 (the
deletion REST per the §17 law; the health REST per the health-endpoints
design). ABSENT-ROADMAP: the rest — the CLI/MCP/UI additions are the
typed roadmap (the registry's AbsentSurfacesNote). The parity census
consumes this table (the landing-zone fact emitted alongside).
