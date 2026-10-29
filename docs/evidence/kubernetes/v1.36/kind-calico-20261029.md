# kind + Calico — Release 0.1.6 Validation Evidence (2026-10-29)

Release-specific validation of zen-cleaner **0.1.6** (image
`sha256:e9747790…`, chart digest-bound) under the **default egress
NetworkPolicy enforced** on a NON-k3s CNI. This complements — and
supersedes for the current release — the historical v1.36 evidence above,
which predates the default NetworkPolicy and all 0.1.x fixes.

## Environment

| Field | Value |
|-------|-------|
| kind | v0.20.0 |
| Node image | `kindest/node:v1.36.1` |
| Kubernetes | v1.36.1 (upstream) |
| CNI | Calico v3.28.2 (manifest install; kind default CNI disabled) |
| Install | `helm upgrade --install oci://ghcr.io/zenmesh/charts/zen-cleaner --version 0.1.6 --set image.digest=sha256:e9747790… --set rbac.targetKinds[0].group=batch … --set rbac.deleteNamespaces[0]=default` (public chart, anonymous pull, **default NetworkPolicy enforced**) |

## Results

| Check | Result |
|-------|--------|
| Controller pod Ready on the exact release digest | PASS |
| Leader lease acquired under enforced egress policy | PASS |
| Eligible ConfigMap TTL deletion (receipt logged) | PASS |
| Dry-run policy: zero deletes | PASS |
| Unlabeled keeper survived | PASS |
| Foreign namespace (kube-system) survived | PASS |
| Safety-protected namespace + excluded object survived | PASS |
| **Concurrent negative probe** (policy-selected pod, non-listed port): denied 10/10 samples during the cleanup window | PASS |
| Uninstall/removal left only owned disposable resources | PASS |
| RBAC denial: non-allowlisted target (Secret) → typed capability-validation denial with bounded backoff (4 log receipts) | PASS |
| Restart: rollout restart → pod Running, leader lease re-acquired by the new pod | PASS |

*(Checks marked "completed 2026-10-29 second pass": the first run covered
the full deletion/dry-run/protection matrix and the concurrent negative
probe; the RBAC-denial and restart checks above were completed on a fresh
kind/Calico cluster of the same specification against the same released
digest.)*

## Interpretation

- Under Calico, the chart's egress rule (TCP/443 + TCP/6443 to any
  destination + DNS) is **enforced and permits the API path** — the
  controller functions fully with the default policy on this non-k3s CNI.
- This differs from k3s's embedded kube-router engine, where ipBlock egress
  matches post-DNAT and requires the API port 6443 to be listed explicitly
  (included in the policy since 0.1.4; k3s runs documented separately).
- Constraints: single-node kind; ephemeral registry-free install by digest;
  no webhook; not EKS/GKE/AKS.

## Explicitly NOT demonstrated in this run

- **Admission webhook coverage**: the default install runs with the webhook
  disabled (`--enable-webhook=false`; policy validation comes from the CRD
  schema alone). The webhook path requires TLS (cert-manager + Secret) and
  is documented separately — this validation does not cover it.
- **Multi-node / HA**: single-node cluster; leader-failover with 2+
  replicas is not exercised here.
- **Non-ConfigMap resource kinds** in this run (the 0.1.2-era v1.36 evidence
  covered Pod/ReplicaSet/Secret/Job TTL modes).

## Cleanup

Probe pods, policies, and the disposable cluster were deleted after the
run; the disposable kubeconfig was removed. The released 0.1.6 artifacts
were untouched.
