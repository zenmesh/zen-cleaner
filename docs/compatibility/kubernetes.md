# zen-cleaner — Kubernetes Compatibility

This document summarizes what is **evidenced** about zen-cleaner's Kubernetes compatibility. Claims here are grounded in the repo's actual dependencies, tests, and CI configuration — not assumed.

## Compilation compatibility

zen-cleaner is built with:

- `client-go v0.35` — targets Kubernetes 1.31.x API surface
- `controller-runtime v0.19` — uses envtest for integration tests
- `apiextensions-apiserver v0.31` (indirect) for CRD support
- CRD API version: `apiextensions.k8s.io/v1`

Build requirement: **Go 1.26+** (see `go.mod`).

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
- Network plugins (Calico, Cilium, Flannel)
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
