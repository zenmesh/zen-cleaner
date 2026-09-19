# Kubernetes v1.36 Validation Evidence

This directory contains validation evidence for zen-cleaner on Kubernetes 1.36 environments.
Each subdirectory or file represents a specific cluster provisioning method.

## Summary

| Method | K8s Version | Status | Scope |
|--------|-------------|--------|-------|
| [kind](kind.md) | v1.36.1 | PASS | CRD + runtime + real deletion (full matrix: 4 TTL modes × 5 resource kinds) |
| [k3d (K3s)](k3d.md) | v1.36.2+k3s1 | PASS | CRD + runtime + real deletion (full matrix: 4 TTL modes × Pod + ReplicaSet) |
| [kubeadm](kubeadm.md) | v1.36.2 | PASS | CRD/API + negative + RBAC + controller runtime + cleanup behavior (containerd 2.2.5 upgraded from Debian's 1.7.24) |

## Scope

Validated environments are **not** a guarantee of correctness across all
Kubernetes 1.36 distributions. Operators should validate in their own
environment.

### What was validated

- Controller startup and leader election
- CRD registration (`ZenCleanerPolicy`)
- Policy creation and status reporting
- Real (non-dry-run) deletion: 4 TTL modes × multiple resource kinds on kind and k3d
- Resource matching via label selectors
- Controller crash-loop resilience
- 3 bugs fixed: evaluation service singleton (per-GVR keyed), Relative TTL deletion, field path escaped dots

### What was NOT validated

- EKS, GKE, AKS, OpenShift, Rancher, or any managed Kubernetes distribution
- All CNI plugins
- All storage providers
- Webhook admission (certificate setup is environment-dependent)
- Performance under load
- Network policies or restrictions

## Repo

- **Repo**: `zenmesh/zen-cleaner`
- **Commit**: `a07dd9a`
- **Controller image**: `zenmesh/zen-cleaner:v0.0.1-alpha-a07dd9a` (build from commit, statically linked, scratch base)
- **Go**: 1.26.4
- **client-go**: v0.35.0
- **controller-runtime**: v0.19.0
