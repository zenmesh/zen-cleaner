# zen-cleaner Deployment

zen-cleaner is a standalone Kubernetes cleanup controller. It is
installed and configured entirely through generic Kubernetes mechanisms:
the `ZenCleanerPolicy` CRD, RBAC, and Helm values. It has no
knowledge of any specific application architecture placed on top of it.

## Install

### 1. CRDs

```bash
kubectl apply -f deploy/crds/cleaner.zen-mesh.io_zencleanerpolicies.yaml
```

### 2. Controller

Helm:

```bash
helm upgrade --install zen-cleaner deploy/helm/zen-cleaner \
  --namespace zen-cleaner-system --create-namespace
```

or the install script:

```bash
./install.sh --method kubectl   # or --method helm
```

## Configuration

| Value | Purpose |
|---|---|
| `image.repository/tag/digest` | Controller image |
| `rbac.targetKinds` | Explicit allowlist of resource classes the controller may collect (default deny; no wildcards) |
| `rbac.namespaceScope` | `true` = deletion confined to the release namespace (Role); `false` = cluster-wide deletion (ClusterRole). Reads are always cluster-wide. |
| `leaderElection.enabled` | controller-runtime leader election |
| `networkPolicy.enabled` | Egress-restricted NetworkPolicy (Kubernetes API over 443 via CIDR — the API endpoint is host-network and cannot be selected by namespace — plus DNS to kube-system) |

Scope the controller by granting only the resource classes it should manage
(`rbac.targetKinds`) and, with `rbac.namespaceScope: true`, by installing one
release per namespace so deletions stay inside each release's namespace.

## Safety

- Default deny: an unknown GVK is never eligible for cleanup
- Explicit resource-class allowlist via `rbac.targetKinds`
- Policy-expressed protection: selection conditions (`conditions.hasLabels`,
  `hasAnnotations`, field conditions, with `NotIn`/`NotEquals` exclusion)
  keep protected objects out of the delete set
- Paused policies (`spec.paused`) are skipped entirely
- Dry-run: `behavior.dryRun: true` selects/plans without mutation
- Bounded batch: `behavior.maxDeletionsPerSecond` caps deletion rate
- Finalizers respected: blocked deletions are recorded, never stripped
- Native Kubernetes garbage collection is preferred where sufficient (TTL-after-finished,
  ownerReferences)

## Authority boundary

zen-cleaner deletes only the Kubernetes resources its policies name within its
RBAC grant. It never touches anything else: application databases, secrets
outside its grant, or resources in namespaces it cannot reach.
