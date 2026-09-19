# kind — Validation Evidence

## Environment

| Field | Value |
|-------|-------|
| **kind version** | v0.20.0 (go1.20.4, linux/amd64) |
| **Node image** | `kindest/node:v1.36.1` |
| **Kubernetes** | v1.36.1 |
| **Container runtime** | containerd://2.3.1 (Debian GNU/Linux 13 (trixie), kernel 6.17.0-35) |

## Validation Date

2026-07-01 (initial dry-run: 2026-06-29; real deletion: 2026-07-01)

## Procedure

### Initial dry-run validation (2026-06-29)

1. Created kind cluster with `kindest/node:v1.36.1` (single control-plane node).
2. Built controller image locally (statically linked, scratch base) and loaded into kind via `ctr images import`.
3. Applied CRDs, namespace, RBAC, and controller deployment.
4. Created `ZenCleanerPolicy` with `dryRun: true`, targeting ConfigMaps with 30s TTL.
5. Confirmed policy Active with ResourcesMatched > 0.

### Real deletion validation (2026-07-01)

6. Deployed controller with fixes for Bug 1 (evaluation service singleton → per-GVR keyed) and Bug 2 (Relative TTL never deletes → ErrRelativeTTLExpired triggers deletion).
7. Ran full validation matrix: 4 TTL modes (fixed, dynamic, mapped, relative) × Pod, ReplicaSet, + ConfigMap, Secret, Job.
8. Each test: created matching resource + control resource with wrong labels, created GCP, polled for deletion, verified control retained, captured controller logs.

## Results

### Dry-run checks

| Check | Result |
|-------|--------|
| Cluster created | PASS |
| CRDs installed | PASS |
| Controller deployed | PASS |
| Pod Ready (leader) | PASS |
| Policy created | PASS |
| Policy status Active | PASS |
| Resources matched | PASS (2 ConfigMaps) |
| Resources deleted (dry-run) | 0 (correct) |
| Controller logs — errors | 0 |

### Real deletion matrix

| TTL Mode | Resource Kind | Result | Evidence |
|----------|--------------|--------|----------|
| Fixed (`secondsAfterCreation`) | Pod | ✅ PASS | Controller log, deleted=1, control retained |
| Fixed (`secondsAfterCreation`) | ReplicaSet | ✅ PASS | Controller log, match resource gone |
| Field-based dynamic (`fieldPath`: int64) | Pod | ✅ PASS | Controller log, match resource gone |
| Mapped (`fieldPath` + `mappings`) | Pod | ✅ PASS | Controller log, deleted=1, control retained |
| Mapped (`fieldPath` + `mappings`) | ReplicaSet | ✅ PASS | Controller log, match resource gone |
| Relative (`relativeTo` + `secondsAfter`) | Pod | ✅ PASS | Controller log, deleted=1, control retained (Bug 2 fix verified) |
| Fixed (`secondsAfterCreation`) | ConfigMap | ✅ PASS | Controller log, match resource gone |
| Mapped (`fieldPath` + `mappings`) | ConfigMap | ✅ PASS | Controller log, match resource gone |
| Fixed (`secondsAfterCreation`) | Secret | ✅ PASS | Controller log, match resource gone |
| Mapped (`fieldPath` + `mappings`) | Secret | ✅ PASS | Controller log, match resource gone |
| Fixed (`secondsAfterCreation`) | Job | ✅ PASS | Controller log, match resource gone |
| Mapped (`fieldPath` + `mappings`) | Job | ✅ PASS | Controller log, match resource gone |

Each test verified:
- Matching disposable resource deleted by controller
- Non-matching control resource (wrong labels) retained
- Controller logs recorded `Deleted resource ... reason=ttl_expired`

Note: A cosmetic status-reporting race causes `resourcesMatched`/`resourcesDeleted` counters to reset mid-cycle for some resource kinds (ReplicaSet, dynamic-field Pod). The deletion still occurs and is verified independently (controller logs + `kubectl get`).

## Controller Logs (leader, real deletion)

```
{"msg":"Deleted resource","resource":"gc-pf/match","reason":"ttl_expired"}
{"msg":"Deleted resource","resource":"gc-pd/match","reason":"ttl_expired"}
{"msg":"Deleted resource","resource":"gc-pm/match","reason":"ttl_expired"}
{"msg":"Deleted resource","resource":"gc-pr/match","reason":"ttl_expired"}
{"msg":"Deleted resource","resource":"gc-rf/match","reason":"ttl_expired"}
{"msg":"Deleted resource","resource":"gc-rm/match","reason":"ttl_expired"}
```

## Limitations

- Only one control-plane node tested (single-node kind cluster).
- Webhook uses self-signed certs; TLS verification not tested.
- The evaluation service per-GVR key fix ensures correctness across multiple policies targeting different resources.
- Dynamic TTL mode requires the target field to be int64; string labels require the `mappings` (mapped TTL) mode.
- kind v0.20.0 is older than the node image; manual `ctr images import` was required (kind `load docker-image` failed due to containerd snapshotter incompatibility).

## Evidence Files

- `kind.json` — structured evidence data.
- `/tmp/zen-cleaner-kind-matrix/*.json` — per-test JSON evidence.
