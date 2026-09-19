# Operator Guide

This guide is for operators who need to install, configure, and maintain the Zen Cleaner controller.

## Table of Contents

- [Installation](#installation)
- [Configuration](#configuration)
- [Monitoring](#monitoring)
- [Troubleshooting](#troubleshooting)
- [Upgrading](#upgrading)
- [Security](#security)

---

## Installation

### Prerequisites

- Kubernetes cluster 1.31.x+ (compiled target; validated on v1.34.9, v1.36.x)
- kubectl configured
- Cluster admin permissions (for CRD installation)

### Step 1: Install CRD

```bash
kubectl apply -f deploy/crds/cleaner.zen-mesh.io_zencleanerpolicies.yaml
```

Verify installation:

```bash
kubectl get crd zencleanerpolicies.cleaner.zen-mesh.io
```

### Step 2: Install Controller

```bash
# Create namespace
kubectl apply -f deploy/manifests/namespace.yaml

# Install RBAC (includes leader election and event permissions)
kubectl apply -f deploy/manifests/rbac.yaml

# Install Deployment (2 replicas with leader election; multi-node HA runtime not validated)
kubectl apply -f deploy/manifests/deployment.yaml

# Install Service
kubectl apply -f deploy/manifests/service.yaml
```

**Note**: The deployment is configured with 2 replicas and leader election.
Only the leader instance will actively process policies.
Multi-node HA runtime behavior has not been validated.

Or use kustomize:

```bash
kubectl apply -k deploy/manifests/
```

### Step 3: Verify Installation

```bash
# Check controller is running (should see 2 pods)
kubectl get pods -n zen-cleaner-system

# Check leader election lease (only one leader)
kubectl get lease zen-cleaner-leader-election -n zen-cleaner-system

# Check logs (only leader will show active processing)
kubectl logs -n zen-cleaner-system -l app=zen-cleaner

# Check metrics endpoint
kubectl port-forward -n zen-cleaner-system svc/zen-cleaner-metrics 8080:8080
curl http://localhost:8080/metrics

# Check events (after creating a policy)
kubectl get events -n zen-cleaner-system --field-selector involvedObject.kind=ZenCleanerPolicy
```

---

## Configuration

### Environment Variables

The controller supports the following environment variables:

- `METRICS_ADDR` - Metrics server address (default: `:8080`)
- `KUBECONFIG` - Path to kubeconfig file (for local development)
- `POD_NAMESPACE` - Namespace for leader election (auto-detected from service account)
- `POD_NAME` - Pod name for leader election identity (auto-detected)

### Command Line Flags

```bash
--kubeconfig=""                    # Path to kubeconfig file
--master=""                        # Kubernetes API server address
--metrics-addr=":8080"             # Metrics server address
--leader-election=true              # Enable leader election for HA (default: true)
--leader-election-namespace=""     # Namespace for leader election lease (default: POD_NAMESPACE)
```

### Resource Limits

#### Default Resource Configuration

Default resource limits in deployment:

```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi
```

#### Resource Requirements by Scale

> **Note**: The following resource numbers are **estimates based on typical controller-runtime patterns**, not validated benchmarks. Performance under load has not been validated. See `docs/claims.md` for validation boundaries.

**Small Scale (< 10 policies, < 1,000 resources)**
- **CPU Request**: 100m
- **CPU Limit**: 500m
- **Memory Request**: 128Mi
- **Memory Limit**: 512Mi
- **Expected Usage**: ~5-15m CPU, ~45-65MB memory

**Medium Scale (10-50 policies, 1,000-10,000 resources)**
- **CPU Request**: 200m
- **CPU Limit**: 1000m (1 CPU)
- **Memory Request**: 256Mi
- **Memory Limit**: 1Gi
- **Expected Usage**: ~15-50m CPU, ~65-120MB memory

**Large Scale (50-100 policies, 10,000-100,000 resources)**
- **CPU Request**: 500m
- **CPU Limit**: 2000m (2 CPU)
- **Memory Request**: 512Mi
- **Memory Limit**: 2Gi
- **Expected Usage**: ~50-150m CPU, ~120-300MB memory

**Very Large Scale (> 100 policies, > 100,000 resources)**
- **CPU Request**: 1000m (1 CPU)
- **CPU Limit**: 4000m (4 CPU)
- **Memory Request**: 1Gi
- **Memory Limit**: 4Gi
- **Expected Usage**: ~150-400m CPU, ~300-800MB memory

#### Resource Calculation Guidelines

**CPU Requirements:**
- Base: 50m per controller instance
- Per Policy: ~1-2m per active policy
- Per Resource: ~0.01m per 1,000 resources being watched
- Deletion Rate: ~5m per 100 deletions/second

**Memory Requirements:**
- Base: 40MB per controller instance
- Per Policy: ~2-5MB per policy (informer overhead)
- Per Resource: ~0.1KB per resource in cache
- Informer Cache: ~50-100MB for 10,000 resources

**Example Calculation:**
- 20 policies, 5,000 resources per policy (100,000 total)
- CPU: 50m (base) + 20×2m (policies) + 100×0.01m (resources) + 20×5m (deletions) = ~170m
- Memory: 40MB (base) + 20×3MB (policies) + 100×0.1KB (cache) = ~100MB
- **Recommended**: CPU Request 200m, Limit 1000m; Memory Request 256Mi, Limit 1Gi

#### Adjusting Resources

Adjust resources based on:
- **Number of policies**: More policies = more CPU/memory
- **Number of resources being watched**: More resources = more memory for cache
- **Deletion rate**: Higher rates = more CPU
- **Concurrent evaluations**: Higher concurrency = more CPU
- **Batch size**: Larger batches = more memory temporarily

#### Monitoring Resource Usage

Monitor these metrics to determine if resources need adjustment:

```bash
# Check current resource usage
kubectl top pods -n zen-cleaner-system -l app=zen-cleaner

# Monitor memory usage via metrics
curl http://localhost:8080/metrics | grep go_memstats

# Check for OOMKills
kubectl describe pod -n zen-cleaner-system -l app=zen-cleaner | grep -i oom
```

**Signs that resources need adjustment:**
- CPU throttling (check `container_cpu_cfs_throttled_seconds_total`)
- Memory pressure (check `container_memory_working_set_bytes`)
- OOMKills (pod restarts)
- Slow policy evaluation (check `zen_cleaner_evaluation_duration_seconds`)
- High error rates (check `zen_cleaner_errors_total`)

#### Vertical Pod Autoscaler (VPA) Support

For automatic resource adjustment, you can use Kubernetes VPA:

```yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: zen-cleaner-vpa
  namespace: zen-cleaner-system
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: zen-cleaner
  updatePolicy:
    updateMode: "Auto"  # or "Off" for recommendations only
  resourcePolicy:
    containerPolicies:
    - containerName: zen-cleaner
      minAllowed:
        cpu: 100m
        memory: 128Mi
      maxAllowed:
        cpu: 4000m
        memory: 4Gi
      controlledResources: ["cpu", "memory"]
```

**VPA Modes:**
- **Auto**: Automatically adjusts resources (requires VPA admission controller)
- **Recreate**: Restarts pods with new resources
- **Off**: Only provides recommendations (safest for production)

**Note**: VPA requires the VPA admission controller to be installed in your cluster. For production, consider using "Off" mode to get recommendations without automatic changes.

---

## Monitoring

### Metrics

The controller exposes Prometheus metrics on port 8080:

```bash
# Port forward to access metrics
kubectl port-forward -n zen-cleaner-system svc/zen-cleaner-metrics 8080:8080

# View metrics
curl http://localhost:8080/metrics
```

Key metrics to monitor:
- `zen_cleaner_policies_total` - Number of policies
- `zen_cleaner_resources_deleted_total` - Deletion rate
- `zen_cleaner_errors_total` - Error rate
- `zen_cleaner_deletion_duration_seconds` - Deletion performance

### Health Checks

- `/healthz` - Liveness probe
- `/readyz` - Readiness probe

### Logging

Controller logs include:
- Policy evaluation events
- Resource deletion events
- Leader election events
- Errors and warnings

View logs:

```bash
# View all pods
kubectl logs -n zen-cleaner-system -l app=zen-cleaner -f

# View leader only
kubectl logs -n zen-cleaner-system -l app=zen-cleaner | grep "leader"
```

### Events

The controller emits Kubernetes events for:
- Policy lifecycle (created, updated, deleted)
- Policy evaluation results
- Resource deletions
- Errors

View events:

```bash
# View all cleanup events
kubectl get events -n zen-cleaner-system --field-selector involvedObject.kind=ZenCleanerPolicy

# View events for specific policy
kubectl describe zencleanerpolicy <policy-name> -n <namespace>
```

---

## Troubleshooting

### Controller Not Starting

1. **Check CRD installation:**
   ```bash
   kubectl get crd zencleanerpolicies.cleaner.zen-mesh.io
   ```

2. **Check RBAC:**
   ```bash
   kubectl get clusterrole zen-cleaner
   kubectl get clusterrolebinding zen-cleaner
   ```

3. **Check pod status:**
   ```bash
   kubectl describe pod -n zen-cleaner-system -l app=zen-cleaner
   ```

### Policies Not Working

1. **Check policy status:**
   ```bash
   kubectl get zencleanerpolicies --all-namespaces
   kubectl describe zencleanerpolicy <name> -n <namespace>
   ```

2. **Check controller logs:**
   ```bash
   kubectl logs -n zen-cleaner-system -l app=zen-cleaner | grep <policy-name>
   ```

3. **Verify resources match:**
   ```bash
   # Check if resources match selectors
   kubectl get <resource-kind> --selector=<label-selector>
   ```

### High Resource Usage

1. **Reduce number of policies**
2. **Optimize selectors** - Use more specific label/field selectors
3. **Increase resource limits** if needed
4. **Scale horizontally** (if supported)

### Deletion Issues

1. **Check RBAC permissions:**
   ```bash
   kubectl auth can-i delete <resource-kind> --as=system:serviceaccount:zen-cleaner-system:zen-cleaner
   ```

2. **Check for finalizers** blocking deletion
3. **Check resource quotas**
4. **Review error logs**

---

## Upgrading

### Backup Policies

Before upgrading, backup existing policies:

```bash
kubectl get zencleanerpolicies --all-namespaces -o yaml > policies-backup.yaml
```

### Upgrade Steps

1. **Backup policies** (see above)
2. **Update CRD** (if changed):
   ```bash
   kubectl apply -f deploy/crds/
   ```
3. **Update controller:**
   ```bash
   kubectl set image deployment/zen-cleaner zen-cleaner=zenmesh/zen-cleaner:<new-version> -n zen-cleaner-system
   ```
4. **Verify upgrade:**
   ```bash
   kubectl rollout status deployment/zen-cleaner -n zen-cleaner-system
   kubectl logs -n zen-cleaner-system -l app=zen-cleaner
   ```

### Rollback

If upgrade fails:

```bash
kubectl rollout undo deployment/zen-cleaner -n zen-cleaner-system
```

---

## Security

### RBAC

The controller requires:
- **Read/Write** access to `ZenCleanerPolicy` CRDs
- **Read/Delete** access to all resources (for cleanup operations)
- **Read** access to namespaces

### Service Account

The controller runs as a non-root user (UID 65534) with minimal permissions.

### Network Policies

If using network policies, allow:
- Controller → API server (all ports)
- Prometheus → Controller metrics (port 8080)

### Admission Webhooks

zen-cleaner ships a **validating** webhook for policies; it requires **TLS** and a correct **`ValidatingWebhookConfiguration`** (`caBundle` or cert-manager injection). Use **[WEBHOOK_TLS.md](WEBHOOK_TLS.md)** for the recommended production path (cert-manager) and a manual-CA option (including the kind e2e script).

---

## Performance Tuning

### For Large Clusters

1. **Increase informer resync interval**
2. **Use more specific selectors**
3. **Batch deletions** (already built-in)
4. **Monitor API server load**

### For High Deletion Rates

1. **Increase `maxDeletionsPerSecond`**
2. **Increase `batchSize`**
3. **Monitor API server rate limits**
4. **Consider API server scaling**

---

## Backup and Recovery

### Backup Policies

```bash
# Backup all policies
kubectl get zencleanerpolicies --all-namespaces -o yaml > policies-backup.yaml

# Backup specific namespace
kubectl get zencleanerpolicies -n <namespace> -o yaml > policies-<namespace>.yaml
```

### Restore Policies

```bash
kubectl apply -f policies-backup.yaml
```

---

## Uninstallation

### Remove Policies

```bash
kubectl delete zencleanerpolicies --all-namespaces --all
```

### Remove Controller

```bash
kubectl delete -f deploy/manifests/
```

### Remove CRD

```bash
kubectl delete crd zencleanerpolicies.cleaner.zen-mesh.io
```

**Warning:** Removing the CRD will delete all policies!

---

## See Also

- [User Guide](USER_GUIDE.md) - How to use cleanup policies
- [Metrics Documentation](METRICS.md) - Monitoring and metrics
- [API Reference](API_REFERENCE.md) - Complete API documentation

