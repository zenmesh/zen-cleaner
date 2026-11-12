
# zen-cleaner — Kubernetes Compatibility

This document summarizes what is **evidenced** about zen-cleaner's Kubernetes compatibility. Claims here are grounded in the repo's actual dependencies, tests, and CI configuration — not assumed.

## Compilation compatibility

zen-cleaner is built with:

- `client-go v0.37.x` — targets the Kubernetes v1.34 API surface
- `controller-runtime v0.25.2` — envtest-backed integration tests (the real
  API-server suite runs in CI with kube-apiserver+etcd binaries; a skip fails CI)
- `apiextensions-apiserver v0.37.x` (indirect) for CRD support
- CRD API version: `apiextensions.k8s.io/v1`

Build requirement: **Go 1.27.4** (see `go.mod`; the container builder is
digest-pinned `golang:1.27.4-alpine`).

## Release-specific validation (0.1.6, 2026-10-29)

Validated with the **released** chart/image (digest-bound) and the DEFAULT
egress NetworkPolicy enforced, full safety matrix (TTL deletion, dry-run,
exclusion, protected/foreign namespaces, RBAC denial, restart/lease,
concurrent negative connectivity probe):

| Environment | Kubernetes | CNI | Result |
|---|---|---|---|
| k3d | v1.37.0 (k3s) | kube-router (embedded) | PASS (API port 6443 allowance required — see README) |
| k3d | v1.35.5 (k3s) | kube-router (embedded) | PASS |
| kind | v1.36.1 (upstream) | Calico v3.28.2 | PASS |

## The default egress NetworkPolicy (install-time warning)

The chart ships an **opt-out-able but default-on egress NetworkPolicy** for the
controller: DNS to kube-system (namespace selector) and TCP/443 + TCP/6443 to
any destination (`ipBlock: 0.0.0.0/0`).

Platform note (verified by probe matrix, 2026-10-29, k3s v1.35.5 and v1.37.0):
the k3s-embedded kube-router policy engine matches egress rules against the
**post-DNAT endpoint tuple**, so the API path (ClusterIP:443 → endpoint:6443)
requires TCP/6443 in the rule; 0.1.2/0.1.3 allowed 443 only and denied the
controller's API access under enforcement (loudly signaled since 0.1.3:
NOT-Ready `/readyz`, `APIUnreachable` events, `zen_cleaner_api_reachable=0`).
0.1.4 allows 6443 and was **verified end-to-end under enforcement on both k3s
versions**: API reach, leadership, TTL cleanup, dry-run, exclusion and RBAC
denials all behave with the policy applied. On **upstream Kubernetes v1.36.1
(kind) + Calico v3.28.2**, the same enforced-policy matrix passes (single-node
cluster, webhook disabled in the default install — evidence:
`docs/evidence/kubernetes/v1.36/kind-calico-20261029.md`).

Troubleshooting: if the pod is NOT-Ready with `/readyz` 503 "API unreachable
via leader-election lock", `APIUnreachable` events, or
`zen_cleaner_api_reachable=0`, your CNI is blocking policy-covered egress.
Cilium and other CNIs have not been validated. If your CNI denies
policy-covered egress differently, set `networkPolicy.enabled=false`
**deliberately** and say so in your issue reports — disabling the policy is an
explicit exception, not the default solution. Operators can also narrow the
443/6443 allowance to the cluster network with
`networkPolicy.egressCidr` (e.g. `10.0.0.0/8`).

Historical tables below pre-date the default NetworkPolicy and the 0.1.x
fixes; they are preserved as earlier-generation evidence, not as coverage
of the current release. Non-k3s CNIs other than Calico are untested.

## Tested configurations

| Configuration | CI | Notes |
|---------------|----|-------|
| Unit tests (envtest) | ✅ `go test ./...` | Uses controller-runtime envtest (fake API server) |
| Integration tests | ✅ `test/integration/` | Runs against envtest-backed API |
| kind cluster (local) | ⚠️ `make e2e-kind` | Available but not run in CI |

## Validated environments

| Environment | K8s Version | Result | Evidence |
|-------------|-------------|--------|----------|
| kind | v1.36.1 | PASS | `docs/evidence/kubernetes/v1.36/kind.md` |
| k3d (K3s) | v1.36.2+k3s1 | PASS | `docs/evidence/kubernetes/v1.36/k3d.md` |
| kubeadm | v1.36.2 | PASS | `docs/evidence/kubernetes/v1.36/kubeadm.md` |
| kubeadm | v1.34.9 | PASS | `docs/evidence/kubernetes/v1.34/kubeadm.md` |

This does **not** imply validation on EKS, GKE, AKS, OpenShift, Rancher, all CNIs, all storage providers, or all Kubernetes 1.36 environments. Operators should validate in their own environment.

## Not evidenced

The following are **not** actively tested or evidenced:

- Cloud Kubernetes (EKS, GKE, AKS)
- Kubernetes distributions (OpenShift, Rancher, k0s, etc.)
- Kubernetes versions other than the specific validated versions (v1.34.9, v1.36.1, v1.36.2+k3s1)
- Network plugins other than those in the release-specific section above
  (Calico v3.28.2 on kind v1.36.1 — validated 2026-10-29, see top;
  Cilium, Flannel standalone, ... untested)
- Storage providers (CSI drivers)
- ARM64 Kubernetes clusters
- Windows Kubernetes nodes

## Version support policy

zen-cleaner follows `client-go` compatibility: compiled against the latest stable Kubernetes API. Older API versions may work but are not actively tested. Breaking changes from upstream Kubernetes may affect behavior — report issues via GitHub.

Validated environments are tracked per Kubernetes minor version in `docs/evidence/kubernetes/`.

## CRD API version

| CRD Version | Status | Notes |
|-------------|--------|-------|
| `v1alpha1` | ✅ Current | May change; not stable API |
| `v1beta1` | 🔜 Planned | No timeline |
| `v1` | 🔜 Planned | No timeline |

## See also

- `go.mod` — exact dependency versions
- `.github/workflows/ci.yml` — CI test matrix
- `docs/VERSION_COMPATIBILITY.md` — additional version migration documentation
