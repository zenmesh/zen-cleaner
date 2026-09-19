# Release-Candidate Evidence Pack — v1 RC

## Metadata

- **Target:** v1 release candidate
- **Commit:** `3011af1e3b5d06dd291a0e5416a815e16a3e28ea`
- **Date:** 2026-07-01
- **Go version:** 1.26.4
- **Kubernetes versions tested:**
  - kind v1.36.1
  - k3d (K3s) v1.36.2+k3s1
  - kubeadm v1.36.2 (containerd 2.2.5, Debian 13)
  - kubeadm v1.34.9 (containerd 2.2.5, Debian 13)
- **Validation harness:**
  - `scripts/validation/validate-real-deletion.sh` — real deletion
  - `scripts/validation/validate-leader-election-safety.sh` — Leader-election safety
- **Runbooks:**
  - `docs/validation/real-gc-deletion.md` — real deletion procedure
  - `docs/validation/leader-election-safety.md` — Leader-election safety procedure

## Leader-Election Safety (Added)

Multi-replica leader-election safety validated on kind v1.36.1 (2 and 3 controller replicas):

| Check | 2 Replicas | 3 Replicas |
|-------|:----------:|:----------:|
| Exactly one leader | ✅ | ✅ |
| Non-leader pods idle | ✅ | ✅ |
| Leader kill → failover to different pod | ✅ | ✅ |
| Matching resources deleted after TTL | ✅ | ✅ |
| Control resources retained | ✅ | ✅ |
| Mapped-ConfigMap deleted | ✅ | ✅ |
| No cross-leader duplicate deletions | ✅ | ✅ |
| No error entries in leader logs | ✅ | ✅ |

Run: `./scripts/validation/validate-leader-election-safety.sh --cluster-kind --replica-counts "2,3" --output-dir /tmp/zen-cleaner-le-validation`
Evidence: `docs/evidence/kubernetes/v1.36/leader-election.md`

## Validation Summary

Real (non-dry-run) deletion validated across 4 TTL modes and 5 resource kinds on kind, 2 resource kinds on k3d. Full matrix:

| Environment | TTL Modes | Resource Kinds | Result |
|-------------|-----------|----------------|--------|
| kind v1.36.1 | fixed, dynamic, mapped, relative | Pod, ReplicaSet, ConfigMap, Secret, Job | ✅ PASS |
| k3d v1.36.2+k3s1 | fixed, dynamic, mapped, relative | Pod, ReplicaSet | ✅ PASS |
| kubeadm v1.36.2 | fixed | Pod | ✅ PASS (preserved) |
| kubeadm v1.34.9 | fixed | Pod | ✅ PASS (preserved) |

## Bug Fixes from Full Validation

| Bug | Symptom | Fix |
|-----|---------|-----|
| BUG-001: Evaluation service singleton | Different-GVR policies got wrong `matched=0` | Key evaluation service cache by target resource (apiVersion/kind/namespace) |
| BUG-002: Relative TTL never deletes | `ErrRelativeTTLExpired` returned `(false, ReasonNoTTL)` | Check `errors.Is(err, sdkttl.ErrRelativeTTLExpired)` return `(true, ReasonTTLExpired)` |
| BUG-003: `parseFieldPath` breaks on dotted annotation keys | Annotation keys like `cleaner.zen-mesh.io/ttl-seconds` split incorrectly | Backslash-escaped dot support (`\.`) |

All bugs have regression tests. See `pkg/controller/reconciler_test.go`, `should_delete_test.go`, `internal/ttl/evaluator_test.go`.

## Reproduction Commands

```bash
# kind — full matrix
./scripts/validation/validate-real-deletion.sh kind

# k3d — full matrix
./scripts/validation/validate-real-deletion.sh k3d

# kind — single TTL mode
./scripts/validation/validate-real-deletion.sh --cluster-kind --ttl-mode fixed

# kind — dry-run plan
./scripts/validation/validate-real-deletion.sh --dry-run-plan --cluster-kind

# Run regression tests
go test ./pkg/controller/... ./internal/ttl/... -count=1
```

## Public Evidence Links

| Evidence | Location |
|----------|----------|
| Leader-election safety | `docs/evidence/kubernetes/v1.36/leader-election.md` |
| Leader-election structured data | `docs/evidence/kubernetes/v1.36/leader-election/manifest.json` |
| Kind validation | `docs/evidence/kubernetes/v1.36/kind.md` |
| Kind structured data | `docs/evidence/kubernetes/v1.36/kind.json` |
| K3d validation | `docs/evidence/kubernetes/v1.36/k3d.md` |
| K3d structured data | `docs/evidence/kubernetes/v1.36/k3d.json` |
| Kubeadm v1.36 | `docs/evidence/kubernetes/v1.36/kubeadm.md` |
| Kubeadm v1.34 | `docs/evidence/kubernetes/v1.34/kubeadm.md` |
| Summary | `docs/evidence/kubernetes/v1.36/summary.json` |
| Claims & maturity | `docs/claims.md` |
| K8s compatibility | `docs/compatibility/kubernetes.md` |
| Machine-readable status | `docs/ai/status.json` |
| Machine-readable evidence index | `docs/ai/evidence-index.json` |
| Validation runbook (cleanup) | `docs/validation/real-gc-deletion.md` |
| Validation harness (cleanup) | `scripts/validation/validate-real-deletion.sh` |
| Validation runbook (leader-election) | `docs/validation/leader-election-safety.md` |
| Validation harness (leader-election) | `scripts/validation/validate-leader-election-safety.sh` |

## Unsupported / Non-Claimed Scope

- **Cloud K8s** (EKS, GKE, AKS, OpenShift, Rancher) — Not tested
- **Multi-node HA** — Single-node clusters only
- **Performance / Load** — Not benchmarked
- **Multi-CNI** — Flannel only on kubeadm
- **Webhook admission** — Partial implementation, not validated
- **v1.34 kind/k3d** — Not tested on v1.34
- **CRD deletion targets** — Not validated
- **All Kubernetes versions** — Only validated versions listed above

## Known Limitations

- Status counter reset (cosmetic): `resourcesMatched`/`resourcesDeleted` may reset mid-cycle for ReplicaSet and dynamic-field Pod evaluations. Deletion still occurs and is verified via controller logs.
- kind v0.20.0 requires manual image import via `ctr images import` (containerd snapshotter incompatibility with `kind load docker-image`).
- Webhook admission disabled during validation (`--enable-webhook=false`).
- Container image not signed (no cosign), no SBOM/provenance published.

## Operator Safety Notes

1. **Dry-run first**: Always validate with `dryRun: true` before enabling real deletion.
2. **Namespace scope**: Policies are namespace-scoped. Cluster-scoped policies not validated.
3. **RBAC scope**: Controller has broad `*.*` list/watch/delete permissions — review before production use.
4. **Rate limiting**: Default 10 deletions/second per policy. Tune via `maxDeletionsPerSecond` and `batchSize`.
5. **Rollback**: Delete a `ZenCleanerPolicy` to stop cleanup for that target. No automatic rollback of already-deleted resources.
6. **No-op while paused**: Set `spec.paused: true` to temporarily disable a policy.

## Release Recommendation

**READY_FOR_REVIEW** — Not production-certified. This evidence pack demonstrates real deletion behavior is validated and reproducible. External review and additional validation (cloud K8s, multi-node HA, performance) are needed before production certification.
