> ⚠ **STALE-CODE / SUPERSEDED BY THE CURRENTNESS PROJECTION** (docsai staleness CI, 2026-10-08): this document's last update (2026-09-19) predates the repository's latest code change (2026-10-06) by ~17 days. The CURRENT status projection lives at zen-mgmt `generated/docs-portal/CURRENTNESS.md`; verify against the source — tracked on the docs drift register.

# Leader Election for zen-cleaner

zen-cleaner uses **client-go leader election** for high availability.

## Overview

- Uses Kubernetes Lease API via client-go
- Configurable via flags (can be disabled for single-replica deployments)
- Only the leader pod runs the Zen Cleaner controller reconciler

## Architecture

**Leader Responsibilities:**
- Runs all ZenCleanerPolicy reconcilers
- Processes policy evaluations
- Manages resource deletions

**Follower Pods:**
- Do NOT run reconcilers (waits for leader election)
- Serve webhooks (if enabled) - load-balanced across pods

## Configuration

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--leader-election` | `true` | Enable/disable leader election |
| `--leader-election-id` | `zen-cleaner-leader-election` | Election lock name |
| `--leader-election-namespace` | `default` | Namespace for the lease lock |

### Enable Leader Election (Recommended for HA)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: zen-cleaner
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: zen-cleaner
        image: zenmesh/zen-cleaner:latest
        args:
        - --leader-election=true
        - --leader-election-namespace=zen-cleaner-system
        env:
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
```

### Disable Leader Election (Single Replica Only)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: zen-cleaner
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: zen-cleaner
        image: zenmesh/zen-cleaner:latest
        args:
        - --leader-election=false
```

**Warning**: Disabling leader election is only safe for single-replica deployments. Multiple replicas will all attempt to reconcile, causing duplicate deletions.

## Verify Leader Status

Check which pod is leader:

```bash
kubectl get leases -n <namespace>
```

The holder identity shows the pod name of the current leader.

## Implementation

zen-cleaner uses the client-go leaderelection package:
- `internal/election/election.go` - Leader election runner
- Uses Lease resource for lock
- Automatic failover on leader loss