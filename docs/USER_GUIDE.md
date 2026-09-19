# User Guide

zen-cleaner makes it easy to automatically clean up Kubernetes resources based on time-to-live (TTL) policies. This guide shows you how to get started and create effective cleanup policies.

## Why Use zen-cleaner?

- **Save Time**: No more manual cleanup scripts or CronJobs
- **Reduce Costs**: Automatically remove unused resources
- **Improve Security**: Clean up temporary secrets and configs
- **Better Organization**: Keep your clusters clean and organized
- **Rate-Limited & Observable**: Built-in rate limiting and observability

## Table of Contents

- [Quick Start](#quick-start)
- [Creating Cleanup Policies](#creating-gc-policies)
- [TTL Configuration](#ttl-configuration)
- [Selectors](#selectors)
- [Conditions](#conditions)
- [Behavior Configuration](#behavior-configuration)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Quick Start

### 1. Install the Zen Cleaner Controller

```bash
# Install CRD
kubectl apply -f deploy/crds/

# Install controller
kubectl apply -f deploy/manifests/
```

### 2. Create Your First Policy

```yaml
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: temp-configmap-cleanup
  namespace: default
spec:
  targetResource:
    apiVersion: v1
    kind: ConfigMap
    labelSelector:
      matchLabels:
        temp: "true"
  ttl:
    secondsAfterCreation: 3600  # 1 hour
  behavior:
    maxDeletionsPerSecond: 10
```

### 3. Verify Policy Status

```bash
kubectl get zencleanerpolicies
kubectl describe zencleanerpolicy temp-configmap-cleanup
```

---

## Creating Cleanup Policies

A `ZenCleanerPolicy` defines:
- **What** resources to clean up (targetResource)
- **When** to clean them up (TTL)
- **Under what conditions** (conditions)
- **How** to clean them up (behavior)

### Basic Policy Structure

```yaml
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: <policy-name>
  namespace: <namespace>
spec:
  targetResource:
    apiVersion: <api-version>
    kind: <kind>
  ttl:
    # TTL configuration (see below)
  conditions:
    # Optional conditions (see below)
  behavior:
    # Optional behavior configuration (see below)
```

---

## TTL Configuration

The `ttl` field is **zen-cleaner's most powerful feature**. It defines when resources should be deleted, and offers four flexible options to match any use case:

### Option 1: Fixed TTL ⏱️

**Simple and straightforward**—delete all matching resources after a fixed time period:

```yaml
ttl:
  secondsAfterCreation: 604800  # 7 days
```

**Use case**: Temporary resources, test artifacts, or any resource with a standard retention period.

### Option 2: Field-Based TTL 📋

**Resource-controlled TTL**—extract TTL directly from resource fields. Let resources define their own lifetime:

```yaml
ttl:
  fieldPath: "spec.ttlSeconds"  # Path to TTL field in resource
```

**Use case**: Resources that already define their TTL (like custom CRDs), or when you want resources to control their own cleanup.

**Example**: A custom resource with `spec.ttlSeconds: 3600` will be deleted after 1 hour.

### Option 3: Mapped TTL 🗺️

**Value-based TTL mapping**—different TTLs based on resource field values. Perfect for severity-based or priority-based retention:

```yaml
ttl:
  fieldPath: "spec.severity"
  mappings:
    CRITICAL: 1814400  # 3 weeks
    HIGH: 1209600      # 2 weeks
    MEDIUM: 604800     # 1 week
    LOW: 259200        # 3 days
  default: 604800      # Default if no match
```

**Use case**: 
- Severity-based retention (critical data kept longer)
- Environment-based retention (prod vs staging)
- Priority-based cleanup (high-priority resources retained longer)

**Example**: Resources with `spec.severity: CRITICAL` are kept for 3 weeks, while `LOW` severity resources are deleted after 3 days.

### Option 4: Relative TTL ⏰

**Activity-based TTL**—clean up resources relative to another timestamp field. Perfect for "last activity" scenarios:

```yaml
ttl:
  relativeTo: "status.lastProcessedAt"
  secondsAfter: 86400  # 1 day after lastProcessedAt
```

**Use case**:
- Clean up resources after last activity/processing
- Retention based on last update time
- Time-based cleanup relative to status changes

**Example**: A resource with `status.lastProcessedAt: 2026-05-07T10:00:00Z` will be deleted on `2026-05-08T10:00:00Z` (1 day later).

### Why These TTL Options Matter

Unlike simple annotation-based solutions, zen-cleaner's TTL system gives you:
- **Flexibility**: Choose the right TTL mode for each use case
- **Resource Control**: Let resources define their own TTL when appropriate
- **Intelligence**: Different retention policies based on resource characteristics
- **Activity Awareness**: Clean up based on actual resource activity, not just creation time

**This is what makes zen-cleaner shine**—you're not limited to "delete after X days." You can build sophisticated, context-aware cleanup policies that adapt to your actual needs.

---

## Selectors

Use selectors to filter which resources a policy applies to.

### Label Selector

```yaml
targetResource:
  apiVersion: v1
  kind: Pod
  labelSelector:
    matchLabels:
      app: my-app
      environment: staging
    matchExpressions:
      - key: version
        operator: In
        values: ["v1", "v2"]
```

### Field Selector

```yaml
targetResource:
  apiVersion: v1
  kind: ConfigMap
  fieldSelector:
    matchFields:
      metadata.namespace: "zen-system"
```

**Performance Note:** Field selectors are evaluated in-memory only (not pushed to the API server). Unlike label selectors which reduce API server load, field selectors filter resources after they're fetched. For better performance with large resource sets, prefer label selectors when possible.

### Namespace Scope

```yaml
targetResource:
  apiVersion: v1
  kind: ConfigMap
  namespace: "zen-system"  # Specific namespace
  # OR
  namespace: "*"           # All namespaces
```

---

## Conditions

Conditions define additional requirements that must be met before deletion.

### Phase Condition

Only delete resources in specific phases:

```yaml
conditions:
  phase: ["Succeeded", "Failed"]  # Only delete completed pods
```

### Label Condition

Only delete if resource has specific labels:

```yaml
conditions:
  hasLabels:
    - key: processed
      value: "true"
    - key: archived
      operator: Exists  # Label exists (any value)
```

### Annotation Condition

Only delete if resource has specific annotations:

```yaml
conditions:
  hasAnnotations:
    - key: cleanup-allowed
      value: "true"
```

### Field Conditions (AND Logic)

Complex conditions using field values:

```yaml
conditions:
  and:
    - fieldPath: "status.processed"
      operator: Equals
      value: "true"
    - fieldPath: "spec.severity"
      operator: In
      values: ["LOW", "INFO"]
```

**Supported Operators:**
- `Equals` - Field equals value
- `NotEquals` - Field does not equal value
- `In` - Field value is in list
- `NotIn` - Field value is not in list

---

## Behavior Configuration

Control how the Zen Cleaner controller executes deletions.

### Rate Limiting

```yaml
behavior:
  maxDeletionsPerSecond: 10  # Max deletions per second
```

### Batch Processing

```yaml
behavior:
  batchSize: 50  # Process resources in batches
```

### Dry Run

Test policies without actually deleting:

```yaml
behavior:
  dryRun: true  # Log deletions but don't delete
```

### Deletion Options

```yaml
behavior:
  gracePeriodSeconds: 30  # Grace period before force deletion
  propagationPolicy: Foreground  # Foreground, Background, or Orphan
```

---

## Examples

### Example 1: Temporary ConfigMap Cleanup

Delete temporary ConfigMaps after 1 hour:

```yaml
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: temp-configmap-cleanup
spec:
  targetResource:
    apiVersion: v1
    kind: ConfigMap
    labelSelector:
      matchLabels:
        temp: "true"
  ttl:
    secondsAfterCreation: 3600  # 1 hour
  behavior:
    maxDeletionsPerSecond: 20
```

### Example 3: Test Pod Cleanup

Delete completed test pods after 30 minutes:

```yaml
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: test-pod-cleanup
spec:
  targetResource:
    apiVersion: v1
    kind: Pod
    labelSelector:
      matchLabels:
        test: "true"
  ttl:
    secondsAfterCreation: 1800  # 30 minutes
  conditions:
    phase: ["Succeeded", "Failed"]
  behavior:
    maxDeletionsPerSecond: 5
    gracePeriodSeconds: 10
```

---

## Troubleshooting

### Policy Not Working

1. **Check policy status:**
   ```bash
   kubectl get zencleanerpolicy <name> -o yaml
   kubectl describe zencleanerpolicy <name>
   ```

2. **Check controller logs:**
   ```bash
   kubectl logs -n zen-cleaner-system -l app=zen-cleaner
   ```

3. **Verify resources match selectors:**
   ```bash
   kubectl get <resource-kind> --selector=<label-selector>
   ```

### Resources Not Being Deleted

- **Check TTL:** Verify resources are actually expired
- **Check conditions:** Ensure resources meet all conditions
- **Check dry-run:** Make sure `dryRun: false` (or not set)
- **Check RBAC:** Ensure controller has delete permissions

### Too Many Deletions

- **Reduce rate:** Lower `maxDeletionsPerSecond`
- **Increase batch size:** Adjust `batchSize`
- **Add conditions:** Be more selective with conditions

### Performance Issues

- **Monitor metrics:** Check `/metrics` endpoint
- **Reduce cleanup interval:** Adjust controller sync interval
- **Optimize selectors:** Use more specific label/field selectors

---

## Best Practices

1. **Start with dry-run:** Always test policies with `dryRun: true` first
2. **Use specific selectors:** Narrow down resources with label/field selectors
3. **Set appropriate TTLs:** Balance retention needs with storage costs
4. **Monitor metrics:** Watch deletion rates and errors
5. **Use conditions:** Add conditions to prevent accidental deletions
6. **Test in staging:** Validate policies in non-production first

---

## See Also

- [Operator Guide](OPERATOR_GUIDE.md) - Installation and configuration
- [API Reference](API_REFERENCE.md) - Complete API documentation
- [Metrics Documentation](METRICS.md) - Prometheus metrics
- [Examples](../examples/) - More example policies

