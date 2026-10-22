# Zen Cleaner Helm chart

Declarative Kubernetes garbage collection: define a `ZenCleanerPolicy`, and the
controller deletes matching resources according to that policy — dry-run first,
default-deny RBAC, and an explicit protection model.

## Safe by default

The defaults are deliberately conservative. An install with no overrides:

- **allowlists only Jobs** (`rbac.targetKinds`), so no other resource class can
  be deleted even if a policy matches it;
- **confines deletion to the release namespace** (`rbac.namespaceScope: true`);
- **refuses workload kinds** (Pod, Job, CronJob) unless you opt in with
  `behavior.allowWorkloadDeletion: true`;
- keeps existing namespace-scoped Role objects (the safety-protected release
  namespace) intact;
- enables a NetworkPolicy restricting egress to TCP/443 (any destination —
  this covers the Kubernetes API but is not API-only) and cluster DNS
  (UDP/TCP 53 to kube-system).

Extend deliberately, from a clone of the repository:

```bash
helm install zen-cleaner ./deploy/helm/zen-cleaner \
  --namespace zen-cleaner-system --create-namespace
```

To authorize another resource class, add it to `rbac.targetKinds` and list the
target namespace in `rbac.deleteNamespaces` (required while
`namespaceScope: true`).

## Pin the image

The chart ships a pinned version tag. For production, prefer an immutable
digest:

```bash
--set image.digest=sha256:<digest>   # the tag is then ignored
```

## CRDs

The `ZenCleanerPolicy` CRD is bundled under `crds/`. Helm installs it on
`helm install` and never deletes or upgrades it on uninstall/upgrade — apply
`deploy/crds/` manually when a release changes the CRD.

## Uninstall

`helm uninstall` removes the controller, its ServiceAccount and the RBAC objects
the chart created. It does **not** remove the CRD, and it does not delete any
cluster resources — this controller only deletes resources a policy explicitly
targets, and nothing about uninstall should trigger deletion.

Verify what remains:

```bash
kubectl get zencleanerpolicies --all-namespaces
kubectl get crd | grep cleaner.zen-mesh.io
```

## Uninstall the CRD too (only if you no longer have policies)

```bash
kubectl delete crd zencleanerpolicies.cleaner.zen-mesh.io
```

This deletes every `ZenCleanerPolicy` in the cluster. Do not run it until you
are sure none are needed.

## Requirements

- Kubernetes 1.34+ (tested on v1.34.9 and v1.36.2 on kubeadm, and k3d v1.36.x).
- The validating admission webhook requires TLS in the cluster. The Helm path
  runs with the webhook disabled, so policy validation then comes from the CRD
  schema alone. See the repository README for the cert-manager route.

## Links

- Source: https://github.com/zenmesh/zen-cleaner
- Issues: https://github.com/zenmesh/zen-cleaner/issues
- Security: https://github.com/zenmesh/zen-cleaner/blob/main/SECURITY.md
- Overview video: https://youtube.com/shorts/i4I0RDWZnFw