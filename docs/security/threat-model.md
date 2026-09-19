# zen-cleaner — Threat Model

This document outlines the threat model for zen-cleaner as a Kubernetes controller that deletes resources based on `ZenCleanerPolicy` CRDs.

## Trust assumptions

- zen-cleaner runs inside a Kubernetes cluster with access to the Kubernetes API
- Operators who create `ZenCleanerPolicy` resources have RBAC permissions to do so
- The Kubernetes API server and etcd are trusted (controller does not authenticate API calls independently)
- Network access to the controller's metrics and health endpoints is restricted by cluster network policy

## Assets

| Asset | Description |
|-------|-------------|
| Kubernetes resources | Any resource a policy targets for deletion |
| Policy configuration | `ZenCleanerPolicy` CRDs defining TTL, selectors, conditions |
| Controller identity | ServiceAccount used by the controller |
| Webhook TLS key | Private key for admission webhook (if enabled) |

## Threats

### T1: Accidental deletion of valuable resources

An operator creates a policy with overly broad selectors, short TTL, or missing conditions.

**Mitigations:**
- Dry-run mode (`spec.dryRun: true`) to preview deletions
- Rate limiting limits blast radius per evaluation cycle
- Kubernetes events emitted before deletion
- Audit logs capture deletion decisions
- RBAC controls who can create/modify policies

### T2: Privilege escalation via policy creation

A user with `create zencleanerpolicies` RBAC can cause the controller to delete resources the user could not delete directly.

**Mitigations:**
- RBAC for `zencleanerpolicies` should be restricted to cluster admins
- The controller uses its own ServiceAccount — not the caller's identity
- Webhook validation (if configured) can enforce policy-level constraints
- See `docs/RBAC.md` for detailed permission model

### T3: Controller ServiceAccount abuse

If the controller's ServiceAccount is compromised, the attacker can delete any resource type the controller has delete permissions for.

**Mitigations:**
- Least-privilege RBAC for controller ServiceAccount
- Restricted Pod Security Standards (non-root, read-only root FS)
- Network policies limit pod communication
- Leader election prevents duplicate controller instances

### T4: Admission webhook bypass

If webhook TLS is misconfigured or `failurePolicy: Ignore` is set, policy validation can be bypassed.

**Mitigations:**
- `failurePolicy: Fail` is the default
- Webhook TLS documented in `docs/WEBHOOK_TLS.md`
- Certificate rotation procedures documented

### T5: Denial of service via policy churn

Rapid creation/deletion of policies could overwhelm the controller.

**Mitigations:**
- Rate limiting per policy evaluation cycle
- Leader election ensures only one active controller
- Configurable reconciliation interval
- Prometheus metrics for monitoring controller health

## Non-goals (out of scope)

- Network-level threats (MITM on API server connections — handled by Kubernetes API TLS)
- Container runtime escapes (handled by Kubernetes Pod security)
- Supply chain attacks on base images — see `docs/security/supply-chain.md`
- Multi-tenant isolation in a shared cluster (controller runs with cluster-level RBAC)

## See also

- `docs/RBAC.md` — detailed RBAC permissions
- `docs/SECURITY.md` — operations security guide
- `docs/WEBHOOK_TLS.md` — webhook TLS configuration
- `SECURITY.md` — vulnerability reporting policy
