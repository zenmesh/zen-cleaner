# Real Deletion Validation Runbook

Validate that zen-cleaner performs **real (non-dry-run) deletion** of matched
resources across all 4 canonical TTL modes and supported GVRs.

## Current Evidence Status

| Environment | K8s Version | TTL Modes | Resource Kinds | Result | Status |
|-------------|-------------|-----------|----------------|--------|--------|
| **kind** | v1.36.1 | fixed, dynamic, mapped, relative | Pod, ReplicaSet, ConfigMap, Secret, Job | ✅ PASS | Current |
| **k3d** (K3s) | v1.36.2+k3s1 | fixed, dynamic, mapped, relative | Pod, ReplicaSet | ✅ PASS | Current |
| **kubeadm** | v1.36.2 | fixed | Pod | ✅ PASS | Preserved |
| **kubeadm** | v1.34.9 | fixed | Pod | ✅ PASS | Preserved |

## Prerequisites

- **Go** 1.26+ (for building the controller binary)
- **Docker** (for building container images)
- **kind** v0.20+ or **k3d** v5.8+ depending on target environment
- **kubectl** matching target K8s version
- **openssl** (for webhook TLS generation, even though webhook validation is disabled)

## Cluster Scenarios

### kind (recommended for full matrix)

```bash
# Full validation: 4 TTL modes × Pod + ReplicaSet (8 scenarios)
./scripts/validation/validate-real-gc-deletion.sh kind

# Single TTL mode
./scripts/validation/validate-real-gc-deletion.sh --cluster-kind --ttl-mode fixed

# Single resource kind
./scripts/validation/validate-real-gc-deletion.sh --cluster-kind --resource-kind Pod

# Dry-run plan (no destructive actions)
./scripts/validation/validate-real-gc-deletion.sh --dry-run-plan --cluster-kind

# Custom output directory
./scripts/validation/validate-real-gc-deletion.sh --cluster-kind --output-dir /tmp/my-evidence
```

### k3d

```bash
# Full validation: 4 TTL modes × Pod + ReplicaSet (8 scenarios)
./scripts/validation/validate-real-gc-deletion.sh k3d

# Or equivalently:
./scripts/validation/validate-real-gc-deletion.sh --cluster-k3d
```

### kubeadm

Validation evidence for kubeadm v1.36.2 and v1.34.9 is **preserved from
previous validated runs** and is not re-validated each cycle. To re-run on
kubeadm:

```bash
# Run on the cluster control-plane node or with remote KUBECONFIG
./scripts/validation/validate-real-gc-deletion.sh kubeadm
```

The kubeadm substrate was validated with:
- **containerd 2.2.5** (Debian 13 trixie; Debian default containerd 1.7.24
  is NOT part of the validated claim)
- **Flannel** v0.28.5 CNI
- 4 vCPUs, 11 GiB RAM single-node control plane

## What It Tests

### TTL Modes (validated)

| Mode | Config Field | Behavior |
|------|-------------|----------|
| Fixed | `ttl.secondsAfterCreation` | Deletes N seconds after resource creation |
| Dynamic | `ttl.fieldPath` (int64) | Reads TTL value from a numeric annotation/field |
| Mapped | `ttl.fieldPath` + `ttl.mappings` | Maps a string label value to TTL via lookup table |
| Relative | `ttl.relativeTo` + `ttl.secondsAfter` | Deletes N seconds after a timestamp field |

### Resource Kinds (validated)

- **Pod** (core/v1) — all 4 TTL modes
- **ReplicaSet** (apps/v1) — all 4 TTL modes
- ConfigMap, Secret, Job — fixed + mapped TTL (kind only)

### Per-Scenario Assertions

Each validation scenario creates 3 resources and asserts:

1. **Matching resource** (correct labels) — **deleted** by controller
2. **Control resource** (wrong labels, same namespace) — **retained**
3. **Other-namespace control** (correct labels, different NS) — **retained**

## Output Files

After a successful run:

| File | Description |
|------|-------------|
| `$OUTPUT_DIR/evidence-$RUN_ID/manifest.json` | Machine-readable evidence manifest with git commit, cluster info, test counts |
| `$OUTPUT_DIR/evidence-$RUN_ID/summary.md` | Human-readable markdown summary |
| `$OUTPUT_DIR/validation-results.json` | Legacy JSON result file |
| `$OUTPUT_DIR/controller-logs-*.txt` | Controller logs per test scenario |

## Expected PASS Signatures

```
=== TTL Mode: fixed, Resource: Pod (gc-pod-fixed) ===
  PASS Pod/fixed: Resources matched (1)
  PASS Pod/fixed: Resources deleted (1)
  PASS Pod/fixed: Matching resource 'match-fixed' deleted
  PASS Pod/fixed: Control resource (wrong labels) retained
  PASS Pod/fixed: Other-namespace control resource retained
```

Controller logs confirm with:
```
{"msg":"Deleted resource","resource":"gc-pf/match","reason":"ttl_expired"}
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All tests PASS |
| 1 | One or more tests FAIL, or SUBSTRATE_BLOCKED |

## Result Classification

Use these categories when interpreting output:

- **PASS** — Assertion met
- **FAIL** — Assertion not met (harness exits non-zero)
- **SKIP_UNSUPPORTED** — Scenario skipped due to unsupported resource kind or TTL mode (not currently emitted by this harness; reserved for future use)
- **SUBSTRATE_BLOCKED** — Cluster/Prerequisite failure preventing validation
- **CLEANUP_FAILED** — Cluster resource cleanup failed (non-fatal to validation result)

## Safety Model

- All test resources are created in disposable namespaces (`gc-*`)
- All namespaces are deleted on exit (unless `--keep-on-failure` or `KEEP_CLUSTER=1`)
- The cluster itself is deleted on exit (unless `--keep-on-failure` or `KEEP_CLUSTER=1`)
- Webhook admission is DISABLED during validation (`--enable-webhook=false`)
- Controller runs as single replica with short cleanup interval for fast feedback
- No modification to user resources outside `gc-*` namespaces

## Unsupported / Non-Claimed Scopes

These are explicitly NOT validated and NOT claimed:

- **v1.34 kind/k3d** — Not tested on v1.34
- **Cloud K8s** (EKS, GKE, AKS, OpenShift, Rancher) — Not tested
- **Multi-node HA** — Single-node clusters only
- **Performance / Load** — Not tested
- **Multi-CNI** — Flannel only on kubeadm; kind/k3d use default CNI
- **v1.32** — Not tested or claimed
- **Webhook admission full runtime** — Dry-run/partial only
- **All Kubernetes resource kinds** — Only Pod, ReplicaSet, ConfigMap, Secret, Job validated

## Adding a New Resource Kind

1. Add the resource kind to `run_ttl_scenario` case statement in
   `scripts/validation/validate-real-gc-deletion.sh`
2. Add the GVR and namespace prefix to the matrix loop
3. Run with `--resource-kind <Kind>` to test just the new kind
4. Document the result in `docs/evidence/` and `docs/claims.md`

## Classifying Failures Without Publishing

If a scenario FAILS:
1. Check whether the failure is **substrate** (cluster not ready, API server
   unavailable, controller not starting) or **behavioral** (TTL not honored,
   wrong resources affected)
2. For **substrate failures**, mark as SUBSTRATE_BLOCKED — do not update
   evidence
3. For **behavioral failures**, investigate root cause, fix, re-run, and
   only update evidence after a clean PASS
4. Never publish FAIL evidence as PASS. If a previously validated scenario
   fails, do not update evidence until the failure is understood and fixed.

## Cleanup Instructions

The harness cleans up automatically:
1. Deletes all `gc-*` namespaces
2. Deletes the cluster (kind/k3d)
3. Removes temporary kubeconfig and TLS cert files

To preserve the cluster for debugging: `--keep-on-failure` (or `KEEP_CLUSTER=1`).

## Known Issues

### Status counter reset (cosmetic)
The `resourcesMatched`/`resourcesDeleted` status counters in the cleanup controllerP status
sub-resource may reset mid-cycle for some resource kinds (ReplicaSet,
dynamic-field Pod). The actual deletion still occurs and is verified
independently via controller logs (`{"msg":"Deleted resource",...}`) and
`kubectl get`. This is a cosmetic status-reporting race, not a deletion bug.

### Fixed Bugs

Three bugs were identified and fixed during the initial validation cycle:

| Bug | Symptom | Fix | File |
|-----|---------|-----|------|
| BUG-001: `getOrCreateEvaluationService` singleton | Different-GVR policies got wrong `matched=0` | Key cache by target resource (apiVersion/kind/namespace) | `pkg/controller/reconciler.go` |
| BUG-002: Relative TTL never deletes | `ErrRelativeTTLExpired` returned `(false, ReasonNoTTL)` | Check for `errors.Is(err, sdkttl.ErrRelativeTTLExpired)` | `pkg/controller/reconciler.go`, `evaluate_policy_refactored.go` |
| BUG-003: `parseFieldPath` splits annotation dots | Dotted annotation keys broken | Backslash-escaped dot support (`\.`) | `internal/ttl/evaluator.go` |

Each bug has corresponding regression tests:
- BUG-001: `TestEvaluationServiceKey_DifferentGVRs`, `TestEvaluationServiceKey_ConsistentCache`
- BUG-002: `TestPolicyReconciler_shouldDelete_RelativeTTL_Expired`, `TestPolicyReconciler_shouldDelete_RelativeTTL_NotExpired`
- BUG-003: `TestParseFieldPath_EscapedDots`, `TestCalculateExpirationTime_DynamicTTL_DottedAnnotation`, `TestCalculateExpirationTime_RelativeTTL_DottedAnnotation`

### kind v0.20.0 containerd import
kind v0.20.0 requires manual `ctr images import` for the controller image
(`kind load docker-image` fails due to containerd snapshotter incompatibility).
This is handled by the harness — see the `kind` case in the validation script.
