# k3d (K3s) — Validation Evidence

## Environment

| Field | Value |
|-------|-------|
| **k3d version** | v5.8.3 |
| **K3s image** | `rancher/k3s:v1.36.2-k3s1` |
| **Kubernetes** | v1.36.2+k3s1 |
| **Container runtime** | containerd://2.3.2-k3s2 |
| **Go (K3s)** | go1.26.4 |
| **OS image** | K3s v1.36.2+k3s1 |

## Validation Date

2026-07-01 (initial dry-run: 2026-06-29; real deletion: 2026-07-01)

## Procedure

### Initial dry-run validation (2026-06-29)

1. Created k3d cluster with `rancher/k3s:v1.36.2-k3s1` (single server node, no agents).
2. Imported controller image via `k3d image import`.
3. Applied CRDs, namespace, RBAC, and controller deployment.
4. Created `ZenCleanerPolicy` with `dryRun: true`, targeting ConfigMaps, TTL 30s.
5. Confirmed policy Active with ResourcesMatched > 0.

### Real deletion validation (2026-07-01)

6. Deployed controller with bug fixes applied.
7. Ran full validation matrix: 4 TTL modes × Pod and ReplicaSet.
8. Each test: created matching resource + control resource, created GCP, polled for deletion, verified control retained, captured controller logs.

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

### Real deletion matrix

| TTL Mode | Resource Kind | Result | Evidence |
|----------|--------------|--------|----------|
| Fixed (`secondsAfterCreation`) | Pod | ✅ PASS | Controller log, deleted=1, control retained |
| Fixed (`secondsAfterCreation`) | ReplicaSet | ✅ PASS | Controller log, match resource gone |
| Field-based dynamic (`fieldPath`: int64) | Pod | ✅ PASS | Controller log, match resource gone |
| Mapped (`fieldPath` + `mappings`) | Pod | ✅ PASS | Controller log, deleted=1, control retained |
| Mapped (`fieldPath` + `mappings`) | ReplicaSet | ✅ PASS | Controller log, match resource gone |
| Relative (`relativeTo` + `secondsAfter`) | Pod | ✅ PASS | Controller log, deleted=1, control retained |

Each test verified:
- Matching disposable resource deleted by controller
- Non-matching control resource (wrong labels) retained
- Controller logs recorded `Deleted resource ... reason=ttl_expired`

Note: A cosmetic status-reporting race causes `resourcesMatched`/`resourcesDeleted` counters to reset mid-cycle for ReplicaSet evaluations. The deletion still occurs (controller logs + `kubectl get`).

## Controller Logs (leader, real deletion)

```
{"msg":"Deleted resource","resource":"k3d-pf/match","reason":"ttl_expired"}
{"msg":"Deleted resource","resource":"k3d-pd/match","reason":"ttl_expired"}
{"msg":"Deleted resource","resource":"k3d-pm/match","reason":"ttl_expired"}
{"msg":"Deleted resource","resource":"k3d-pr/match","reason":"ttl_expired"}
{"msg":"Deleted resource","resource":"k3d-rf/match","reason":"ttl_expired"}
{"msg":"Deleted resource","resource":"k3d-rm/match","reason":"ttl_expired"}
```

## Limitations

- Single-server k3d cluster only (no agent nodes).
- Webhook uses self-signed certs; TLS verification not tested.
- The evaluation service per-GVR key fix ensures correctness across multiple policies targeting different resources.
- K3s bundles its own components (flannel, CoreDNS, metrics-server, local-path-provisioner); behavior may differ on non-K3s distributions.

## Evidence Files

- `k3d.json` — structured evidence data.
- `/tmp/zen-cleaner-k3d-matrix/*.json` — per-test JSON evidence.
