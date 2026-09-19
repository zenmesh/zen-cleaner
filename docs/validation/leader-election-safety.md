# Leader Election Safety Validation Runbook

Validate that zen-cleaner controller leader election provides safe multi-replica behavior:

- Exactly one leader at a time
- Non-leader replicas do not reconcile or delete
- Leader failover on pod deletion
- No duplicate deletion attempts
- Cleanup reconciliation resumes after failover
- Leases reflect leadership transitions

## Current Evidence Status

The following scenarios have been validated:

| Environment | K8s Version | Replicas | Result | Date |
|-------------|-------------|----------|--------|------|
| **kind** | v1.36.1 | 2 | ✅ PASS | See evidence |
| **kind** | v1.36.1 | 3 | ✅ PASS | See evidence |
| **k3d** (K3s) | v1.36.2+k3s1 | 2 | ✅ PASS | See evidence |

**Not validated**: Cloud K8s, multi-node HA, performance under load.

## What This Validates

### Required Scenarios

1. Two controller replicas with leader election enabled
2. Three controller replicas (if supported and not flaky)
3. Exactly one active leader at a time
4. Non-leader replicas do not reconcile or delete
5. Leader pod deletion causes new leader to take over
6. No duplicate deletion attempts on the same resource
7. No non-matching control resource deletion
8. No not-yet-expired resource deletion
9. Reconciliation resumes after leader failover
10. Lease object proves leadership transitions

### Minimum Resource Matrix

| Resource | TTL Mode | Behavior |
|----------|----------|----------|
| Pod | Fixed (`secondsAfterCreation`) | Matching pod deleted, control retained |
| Pod | Dynamic (`fieldPath`: int64) | Matching pod deleted, control retained |
| Pod | Relative (`relativeTo` + `secondsAfter`) | Matching pod deleted, control retained |
| ReplicaSet | Fixed | Matching RS deleted, control retained |
| ConfigMap | Mapped (`fieldPath` + `mappings`) | Matching CM deleted, control retained |

## Prerequisites

- **Go** 1.26+ (for building the controller binary)
- **Docker** (for building container images)
- **kind** v0.20+ or **k3d** v5.8+ depending on target environment
- **kubectl** matching target K8s version

## Procedure

### kind (recommended)

```bash
# Full validation: 2 and 3 replicas, Pod/ReplicaSet/ConfigMap with fixed/dynamic/relative/mapped TTL
./scripts/validation/validate-leader-election-safety.sh --cluster-kind

# Specific replica count
./scripts/validation/validate-leader-election-safety.sh --cluster-kind --replica-counts 2

# Dry-run plan
./scripts/validation/validate-leader-election-safety.sh --dry-run-plan --cluster-kind

# Custom output directory
./scripts/validation/validate-leader-election-safety.sh --cluster-kind --output-dir /tmp/my-evidence
```

### k3d

```bash
# Full validation: 2 replicas
./scripts/validation/validate-leader-election-safety.sh --cluster-k3d --replica-counts 2
```

## Output Files

After a successful run:

| File | Description |
|------|-------------|
| `$OUTPUT_DIR/evidence-$RUN_ID/manifest.json` | Machine-readable evidence manifest |
| `$OUTPUT_DIR/evidence-$RUN_ID/summary.md` | Human-readable markdown summary |
| `$OUTPUT_DIR/evidence-$RUN_ID/lease-before-failover-*.json` | Lease state before leader failover |
| `$OUTPUT_DIR/evidence-$RUN_ID/lease-after-failover-*.json` | Lease state after leader failover |
| `$OUTPUT_DIR/evidence-$RUN_ID/logs-before-*.txt` | Pod logs before failover |
| `$OUTPUT_DIR/evidence-$RUN_ID/logs-after-*.txt` | Pod logs after failover |
| `$OUTPUT_DIR/evidence-$RUN_ID/leader-logs-*.txt` | Leader pod logs including deletion events |

## Expected PASS Signatures

```
=== LE Test: 2 replicas (run r2) ===
  PASS Leader elected: zen-cleaner-xxx
  PASS Exactly one leader: zen-cleaner-xxx
  PASS All non-leader pods are idle (no reconciliation)
  PASS Leadership changed from zen-cleaner-xxx to zen-cleaner-yyy
  PASS Matching pod match-fixed-r2 deleted after TTL
  PASS No duplicate deletion entries detected
  PASS Control resource control-wrong-labels-r2 retained
...
=== FINAL SUMMARY ===
  PASS All leader-election safety tests passed
```

## Leader-Election Proof

The harness captures:

### From Lease Object
- `kubectl get lease <lease-name> -n zen-cleaner-system` before and after failover
- Holder identity, acquire time, lease duration, renew times

### From Controller Logs
- `Successfully acquired lease` — confirming leadership
- `Started leading` — callback invoked
- `New leader elected: <identity>` — all pods notified
- `Deleted resource` — deletion events with resource identity
- `Stopped leading` — on leadership loss

### Key Assertions (code-verified)
1. Exactly one `Successfully acquired lease` event across all pods
2. Non-leader pods have zero `Reconciling` log entries
3. Deletion log entries have unique resource identities (no duplicate UIDs)
4. The lease `spec.holderIdentity` always identifies a running pod
5. After leader pod deletion, a new leader acquires the lease
6. Matching resources are deleted; control resources (wrong labels) are retained

## Safety Model

- Test resources created in disposable namespaces (`gc-le-test`)
- Namespace deleted between test runs
- Cluster deleted on exit (unless `--keep-on-failure`)
- Webhook admission disabled during validation
- Short cleanup interval for fast feedback
- No modification to user resources outside test namespace

## Unsupported / Non-Claimed Scopes

These are explicitly NOT validated and NOT claimed:

- **Cloud K8s** (EKS, GKE, AKS, OpenShift, Rancher) — Not tested
- **Multi-node HA** — Single-node clusters only
- **Performance / Load** — Not benchmarked
- **Multi-CNI** — Default CNI only
- **Webhook admission** — Not validated in multi-replica context
- **This validates local controller leader-election safety only**

## Claim Boundaries

- ✅ Leader-election safety validated with multi-replica controller on local substrates (kind/k3d)
- ❌ Not a cloud HA claim
- ❌ Not a multi-node HA claim
- ❌ Not a performance/load claim

## Known Issues

### Lease Re-acquisition by Restarted Pod
When the leader pod is deleted, the same pod name may re-acquire the lease if it restarts before another pod. This is correct behavior — the lease identity is the pod name, and a new pod with the same name is a valid leader. The harness checks that the lease transitions correctly and that reconciliation resumes.

### kind v0.20 containerd import
Same limitation as the cleanup controller deletion harness: `kind load docker-image` may fail due to containerd snapshotter incompatibility. Use the harness which handles this automatically.

## Classification

| Result | Meaning |
|--------|---------|
| PASS | All assertions met |
| FAIL | One or more assertions not met |
| SKIP | Scenario skipped (replica count unsupported) |
