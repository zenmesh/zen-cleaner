# Kubernetes v1.34 Validation Evidence

This directory contains validation evidence for zen-cleaner on Kubernetes 1.34
environments. Full runtime + cleanup behavior has been validated with
**containerd 2.2.5** (upgraded from Debian's default 1.7.24 which caused
CP instability).

## Summary

| Method | K8s Version | Status | Scope |
|--------|-------------|--------|-------|
| [kubeadm](kubeadm.md) | v1.34.9 | PASS | CRD/API + runtime + cleanup (containerd 2.2.5) |

## What was validated

- CRD registration and install idempotency
- API resource discovery (short names, group, version)
- CRUD lifecycle: create, list, read, re-apply, delete
- Negative schema validation (wrong types, unknown fields, missing required)
- RBAC permissions (GCP read/watch, status update, leases, broad list/delete)
- Controller deployment (2 replicas, leader election, workers started)
- Runtime reconciliation (controller active, webhook listening)
- cleanup deletion behavior (disposable pod deleted, control pod untouched)
- All spec fields: targetResource, ttl, conditions, behavior

## What was NOT validated on v1.34

- kind or k3d/K3s on v1.34
- EKS, GKE, AKS, OpenShift, Rancher
- Webhook admission with TLS (insecure mode only)
- Performance under load
- HA or multi-node

## Repo

- **Repo**: `zenmesh/zen-cleaner`
- **Commit**: `a07dd9a`

## Compatibility Note

Validated with containerd 2.2.5 on Debian 13. Debian default containerd 1.7.24
is not part of this validated claim. See `kubeadm.md` for full details.
