# Version Compatibility

This document summarizes zen-cleaner's Kubernetes version compatibility based on
validated evidence. See `docs/evidence/kubernetes/` for full validation details.

## Validated Environments

| Environment | K8s Version | Result | Evidence |
|-------------|-------------|--------|----------|
| kind | v1.36.1 | PASS | `docs/evidence/kubernetes/v1.36/kind.md` |
| k3d (K3s) | v1.36.2+k3s1 | PASS | `docs/evidence/kubernetes/v1.36/k3d.md` |
| kubeadm | v1.36.2 | PASS | `docs/evidence/kubernetes/v1.36/kubeadm.md` |

This does **not** imply validation on EKS, GKE, AKS, OpenShift, Rancher, all CNIs,
all storage providers, or all Kubernetes 1.36 environments. Operators should validate
in their own environment.

## Compilation Compatibility

zen-cleaner is built with `client-go v0.35` targeting Kubernetes 1.31.x API surface.
Compilation compatibility does not guarantee runtime correctness on any specific
Kubernetes version.

## Minimum Requirements

- **CRD API**: v1 (`apiextensions.k8s.io/v1`)
- **RBAC**: v1 (`rbac.authorization.k8s.io/v1`)
- **Go**: 1.26+ (see `go.mod`)

## Version Support Policy

zen-cleaner follows `client-go` compatibility: compiled against the latest stable
Kubernetes API. Older API versions may work but are not actively tested. Breaking
changes from upstream Kubernetes may affect behavior — report issues via GitHub.

### Deprecated Features

| Feature | Deprecated In | Removed In | Replacement |
|---------|--------------|------------|-------------|
| - | - | - | - |

---

## CRD Version Migration Guide

### CRD Version History

| Version | Status | Kubernetes Version | Notes |
|---------|--------|-------------------|-------|
| v1alpha1 | ✅ Current | 1.23+ | Initial release, may have breaking changes |
| v1beta1 | 🔜 Planned | TBD | API stabilized, no breaking changes |
| v1 | 🔜 Planned | TBD | Stable API, long-term support |

### Migration Path: v1alpha1 → v1beta1

**When**: After API stabilization and community feedback

**Breaking Changes**: None expected

**Migration Steps**:

1. **Backup Current Policies**:
   ```bash
   kubectl get zencleanerpolicies --all-namespaces -o yaml > policies-backup.yaml
   ```

2. **Update CRD**:
   ```bash
   kubectl apply -f deploy/crds/cleaner.zen-mesh.io_zencleanerpolicies-v1beta1.yaml
   ```

3. **Verify Policies**:
   ```bash
   kubectl get zencleanerpolicies --all-namespaces
   ```

4. **Update Policy Definitions** (if needed):
   ```bash
   # Update apiVersion in policy YAML files
   sed -i 's/apiVersion: cleaner.zen-mesh.io\/v1alpha1/apiVersion: cleaner.zen-mesh.io\/v1beta1/g' policies/*.yaml
   kubectl apply -f policies/
   ```

5. **Test Functionality**:
   ```bash
   # Verify controller still works
   kubectl logs -n zen-cleaner-system deployment/zen-cleaner
   ```

### Migration Path: v1beta1 → v1

**When**: Future API stability pass (not currently scheduled)

**Breaking Changes**: None planned (subject to API stability policy)

**Migration Steps**:

Same as v1alpha1 → v1beta1, but with v1 CRD and API version.

### Rollback Procedure

If migration causes issues:

```bash
# 1. Scale down controller
kubectl scale deployment zen-cleaner -n zen-cleaner-system --replicas=0

# 2. Restore previous CRD version
kubectl apply -f deploy/crds/cleaner.zen-mesh.io_zencleanerpolicies-v1alpha1.yaml

# 3. Restore policies from backup
kubectl apply -f policies-backup.yaml

# 4. Scale controller back up
kubectl scale deployment zen-cleaner -n zen-cleaner-system --replicas=2
```

---

## Migration from Other Solutions

### Migration from k8s-ttl-controller

#### Overview

k8s-ttl-controller uses annotations for TTL configuration, while zen-cleaner uses declarative CRDs.

#### Key Differences

| Feature | k8s-ttl-controller | zen-cleaner |
|---------|-------------------|--------|
| Configuration | Annotations | CRDs |
| Policy Management | Per-resource | Centralized |
| Selectors | Limited | Full Kubernetes selectors |
| Conditions | No | Yes |
| Rate Limiting | No | Yes |
| Metrics | Limited | Comprehensive |

#### Migration Steps

1. **Audit Current Usage**:
   ```bash
   # Find all resources with TTL annotations
   kubectl get all --all-namespaces -o json | \
     jq '.items[] | select(.metadata.annotations["ttl-controller.k8s.io/ttl"]) | 
         {namespace: .metadata.namespace, name: .metadata.name, 
          kind: .kind, ttl: .metadata.annotations["ttl-controller.k8s.io/ttl"]}'
   ```

2. **Create Equivalent Policies**:
   ```yaml
   # Example: Convert annotation-based TTL to policy
   # Before (k8s-ttl-controller):
   # metadata:
   #   annotations:
   #     ttl-controller.k8s.io/ttl: "3600"
   
   # After (zen-cleaner):
   apiVersion: cleaner.zen-mesh.io/v1alpha1
   kind: ZenCleanerPolicy
   metadata:
     name: configmap-ttl-policy
   spec:
     targetResource:
       apiVersion: v1
       kind: ConfigMap
       labelSelector:
         matchLabels:
           ttl-enabled: "true"
     ttl:
       secondsAfterCreation: 3600
   ```

3. **Migrate Resources**:
   ```bash
   # Add labels to resources that should be managed by zen-cleaner
   kubectl label configmap <name> ttl-enabled=true
   
   # Remove old annotations (optional, after verification)
   kubectl annotate configmap <name> ttl-controller.k8s.io/ttl-
   ```

4. **Test and Verify**:
   ```bash
   # Enable dry-run mode initially
   # Verify policies match expected behavior
   # Disable k8s-ttl-controller
   # Enable zen-cleaner policies
   ```

5. **Remove k8s-ttl-controller**:
   ```bash
   kubectl delete deployment k8s-ttl-controller -n <namespace>
   ```

#### Migration Script

```bash
#!/bin/bash
# migrate-from-k8s-ttl-controller.sh

# Find resources with TTL annotations
RESOURCES=$(kubectl get all --all-namespaces -o json | \
  jq -r '.items[] | select(.metadata.annotations["ttl-controller.k8s.io/ttl"]) | 
         "\(.metadata.namespace)|\(.metadata.name)|\(.kind)|\(.metadata.annotations["ttl-controller.k8s.io/ttl"])"')

# Generate policies for each resource type
echo "$RESOURCES" | while IFS='|' read -r ns name kind ttl; do
  # Create policy YAML
  cat <<EOF
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: ${kind}-ttl-migrated-$(echo $name | tr '[:upper:]' '[:lower:]')
  namespace: $ns
spec:
  targetResource:
    apiVersion: v1
    kind: $kind
    namespace: $ns
    labelSelector:
      matchLabels:
        migrated-from-k8s-ttl-controller: "true"
  ttl:
    secondsAfterCreation: $ttl
EOF
done
```

---

### Migration from Kyverno Cleanup Policies

#### Overview

Kyverno cleanup policies are similar to zen-cleaner but require Kyverno installation.

#### Key Differences

| Feature | Kyverno Cleanup | zen-cleaner |
|---------|----------------|--------|
| Dependencies | Requires Kyverno | Zero dependencies |
| Policy Language | Kyverno policy | Kubernetes CRD |
| Conditions | Rego-based | Kubernetes-native |
| Performance | Policy engine overhead | Direct controller |

#### Migration Steps

1. **Export Kyverno Policies**:
   ```bash
   kubectl get clusterpolicies -o yaml > kyverno-policies.yaml
   ```

2. **Convert to zen-cleaner Policies**:
   ```yaml
   # Before (Kyverno):
   apiVersion: kyverno.io/v1
   kind: ClusterPolicy
   metadata:
     name: cleanup-configmaps
   spec:
     rules:
     - name: cleanup-old-configmaps
       match:
         resources:
           kinds:
           - ConfigMap
       exclude:
         resources:
           namespaces:
           - kube-system
       validate:
         message: "ConfigMap will be deleted after 1 hour"
         pattern:
           metadata:
             annotations:
               cleanup.kyverno.io/ttl: "1h"
   
   # After (zen-cleaner):
   apiVersion: cleaner.zen-mesh.io/v1alpha1
   kind: ZenCleanerPolicy
   metadata:
     name: cleanup-configmaps
   spec:
     targetResource:
       apiVersion: v1
       kind: ConfigMap
       labelSelector:
         matchLabels:
           cleanup-enabled: "true"
     ttl:
       secondsAfterCreation: 3600
     conditions:
       hasLabels:
       - key: cleanup-enabled
         value: "true"
   ```

3. **Migrate Resources**:
   ```bash
   # Add labels to resources
   kubectl label configmap <name> cleanup-enabled=true
   ```

4. **Test and Remove Kyverno**:
   ```bash
   # Test zen-cleaner policies
   # Remove Kyverno cleanup policies
   kubectl delete clusterpolicy cleanup-configmaps
   ```

---

### Migration from Custom Controllers

#### Overview

Many organizations have built custom Zen Cleaner controllers. Migration to zen-cleaner provides standardization.

#### Migration Strategy

1. **Document Current Behavior**: Understand what your custom controller does
2. **Map to zen-cleaner Features**: Identify equivalent zen-cleaner policies
3. **Create Policies**: Convert custom logic to zen-cleaner policies
4. **Test Side-by-Side**: Run both controllers temporarily
5. **Migrate Gradually**: Migrate one resource type at a time
6. **Remove Custom Controller**: After full migration

#### Example Migration

```yaml
# Custom controller logic (pseudo-code):
# if resource.age > 7 days AND resource.status == "completed":
#   delete resource

# zen-cleaner equivalent:
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: completed-resource-cleanup
spec:
  targetResource:
    apiVersion: v1
    kind: Pod
  ttl:
    secondsAfterCreation: 604800  # 7 days
  conditions:
    phase: ["Succeeded", "Failed"]
```

---

## Reporting Compatibility Issues

If you encounter compatibility issues, open a GitHub issue and include:
- Kubernetes version
- Cluster environment (kind, k3d, kubeadm, cloud provider, etc.)
- Error messages and logs
- Steps to reproduce

For specific migration help, open a GitHub issue or contact maintainers.

