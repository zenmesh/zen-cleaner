# Leader Election Safety — Validation Evidence

## Environment

| Field | Value |
|-------|-------|
| **kind version** | v0.20.0 (go1.20.4, linux/amd64) |
| **Node image** | `kindest/node:v1.36.1` |
| **Kubernetes** | v1.36.1 |
| **Container runtime** | containerd://2.3.1 |
| **Controller replicas tested** | 2, 3 |
| **cleanup interval** | 10s |
| **TTL** | 25s |

## Validation Date

2026-07-01

## Procedure

1. Created kind cluster with `kindest/node:v1.36.1` (single control-plane node).
2. Built controller image (statically linked, scratch base) and loaded into kind.
3. Applied CRDs, namespace, RBAC, and controller deployment.
4. For each replica count (2, 3):
   a. Verified exactly one leader elected (lease object + logs).
   b. Verified non-leader pods are idle (no reconciliation logs).
   c. Created matching + control resources (Pod fixed/dynamic/relative TTL, ReplicaSet fixed, ConfigMap mapped).
   d. Created cleanup policies.
   e. Killed leader pod.
   f. Verified new leader elected.
   g. Verified non-leader pods remain idle after failover.
   h. Waited for TTL expiry.
   i. Verified matching resources deleted, control resources retained.
   j. Checked for cross-leader duplicate deletions.

## Results

### Scenario 1: 2 Controller Replicas ✅ PASS

| Check | Result |
|-------|--------|
| Leader elected | ✅ PASS |
| Exactly one leader | ✅ PASS (zen-cleaner-6d4f87fbf7-g6n9x) |
| Non-leader pods idle | ✅ PASS |
| Leader pod deleted, new leader elected | ✅ PASS (zen-cleaner-6d4f87fbf7-hdjv7) |
| Matching resources deleted after TTL | ✅ PASS (fixed, dynamic, relative, mapped) |
| Control resources retained | ✅ PASS |
| No cross-leader duplicate deletions | ✅ PASS |
| No error entries in leader logs | ✅ PASS |

### Scenario 2: 3 Controller Replicas ✅ PASS

| Check | Result |
|-------|--------|
| Leader elected | ✅ PASS |
| Exactly one leader | ✅ PASS (zen-cleaner-6d4f87fbf7-hdjv7) |
| Non-leader pods idle | ✅ PASS |
| Leader pod deleted, new leader elected | ✅ PASS |
| Matching resources deleted after TTL | ✅ PASS (fixed, dynamic, relative, mapped) |
| Control resources retained | ✅ PASS |
| No cross-leader duplicate deletions | ✅ PASS |
| No error entries in leader logs | ✅ PASS |

### Lease Transitions

Lease before and after failover captured as evidence. The lease `spec.holderIdentity` transitions from the deleted leader pod to the new leader, proving leadership transfer.

### Logs Evidence

- `leader-logs-r2.txt` — Leader logs for 2-replica run
- `leader-logs-r3.txt` — Leader logs for 3-replica run
- Pre/post-failover logs for each pod captured in evidence directory

## What This Proves

- **Exactly one leader at a time** — Only one pod holds the Lease lock. Non-leader pods show no reconciliation activity.
- **Leader failover** — Deleting the leader pod triggers a new leader election within seconds. The new leader resumes cleanup reconciliation.
- **No cross-leader duplicate deletion** — Resources deleted by the old leader are not re-deleted by the new leader.
- **cleanup behavior preserved across failover** — Matching resources are deleted after TTL, control resources are retained.
- **Lease object tracks leadership** — `spec.holderIdentity` is the authoritative source of leadership identity.

## What This Does NOT Prove

- **Cloud HA** — Single-node kind cluster only. Not validated on EKS, GKE, AKS, etc.
- **Multi-node HA** — Single control-plane node.
- **Performance / Load** — Not benchmarked.
- **Webhook admission** — Disabled during validation (`--enable-webhook=false`).

## Claim Boundaries

- ✅ Leader-election safety validated with multi-replica controller on kind/k3d.
- ❌ Not a cloud HA claim.
- ❌ Not a multi-node HA claim.
- ❌ Not a performance/load claim.

## Evidence Files

| File | Description |
|------|-------------|
| `leader-election/manifest.json` | Machine-readable evidence manifest |
| `leader-election/summary.md` | Human-readable summary |
| `leader-election/lease-before-failover-r2.json` | Lease before failover (2 replicas) |
| `leader-election/lease-after-failover-r2.json` | Lease after failover (2 replicas) |
| `leader-election/lease-before-failover-r3.json` | Lease before failover (3 replicas) |
| `leader-election/lease-after-failover-r3.json` | Lease after failover (3 replicas) |
| `leader-election/leader-logs-r2.txt` | Leader logs (2 replicas) |
| `leader-election/leader-logs-r3.txt` | Leader logs (3 replicas) |
