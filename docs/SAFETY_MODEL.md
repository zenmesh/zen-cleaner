# Zen Cleaner — Safety Model (RC1)

SUPPORT2-033. Zen Cleaner is destructive by nature: its whole purpose is to
delete Kubernetes objects. This document is the authoritative statement of the
laws that make that safe. Everything here is enforced in code; the tests named
at the end keep the code and this document from drifting.

## 1. The safety gate

Every deletion passes a single choke point, `performResourceDeletion`
(pkg/controller/reconciler_helpers.go), which runs the safety gate from
`pkg/safety` and a live UID precondition. There are no other DELETE call
sites. A second, evaluation-time pass of the same gate runs while building
the deletion plan so refused objects are visible in policy status, not just
in logs.

The gate is fail-safe: an object that cannot be understood (no name, no kind,
nil) is refused with reason `unknown`.

## 2. Laws

| # | Law | Refusal reason | Enforced at |
|---|-----|----------------|-------------|
| 1 | Infrastructure namespaces (`kube-system`, `kube-public`, `kube-node-lease`, plus k3s add-on namespaces) can never be cleanup targets or sources of deletions. | `protected_namespace` | validation + policy-target gate + object gate |
| 2 | The controller's own namespace is protected. | `protected_namespace` | object gate |
| 3 | Operators can add protected namespaces with `ZEN_CLEANER_PROTECTED_NAMESPACES` (CSV). | `protected_namespace` | object gate |
| 4 | Hard-protected kinds can never be targets: `Namespace`, `PersistentVolume`, `PersistentVolumeClaim`, `StorageClass`, `CustomResourceDefinition`, `ValidatingWebhookConfiguration`, `MutatingWebhookConfiguration`. Data-bearing volumes are unconditionally out of scope. | `protected_kind` | validation + policy-target gate + object gate |
| 5 | The exclusion label/annotation `zen-cleaner.zen-mesh.io/exclude: "true"` makes an object permanently invisible to every policy. | `excluded_label` | object gate |
| 6 | Workload kinds (`Pod`, `Job`, `CronJob`) require the explicit policy opt-in `behavior.allowWorkloadDeletion: true`. | `workload_managed` | object gate |
| 7 | Even with the opt-in, a Pod is only deletable in a terminal phase (`Succeeded`/`Failed`). A pod whose phase cannot be proven is refused. | `active_workload` | object gate |
| 8 | A deletion decision made on an informer-cache snapshot is re-verified against the API server immediately before DELETE. If the live UID differs (same name, replacement object), the deletion is refused. If the object is gone, the operation is an idempotent no-op. | `stale_uid` | UID precondition |
| 9 | The emergency stop `ZEN_CLEANER_DELETE_ENABLED=false` disables every deletion; candidates are logged and counted with reason `deletes_disabled`. | `deletes_disabled` | object gate |
| 10 | A policy that fails runtime validation drives no cleanup. Admission is the first validation line; the reconcile loop re-validates so a policy created while the webhook was down cannot bypass the rules. | `policy_invalid` | reconcile gate |

Any ambiguity denies.

## 3. Policy validation rules (admission and runtime)

In addition to the object laws above, policies themselves are validated:

- `targetResource.namespace` must be explicit (`""` is rejected — it
  historically meant cluster-wide, an unsafe implicit default).
- A policy may not target a protected namespace or a hard-protected kind.
- A cluster-wide target (`namespace: "*"`) requires a non-empty
  `labelSelector` (matchLabels or matchExpressions). An unbounded
  match-everything cluster policy is rejected as `unsafe broad scope`.
- `behavior.finalizer` is rejected outright: the field has never been
  implemented and accepting it silently was a safety trap.
- `propagationPolicy` is restricted to Foreground/Background/Orphan;
  gracePeriodSeconds must be non-negative; rate and batch values
  non-negative.
- TTL must specify at least one mode; TTL evaluation itself is fail-closed
  (missing fields or unknown mappings mean "not eligible").

## 4. Bounded deletion rate

Deletion velocity is bounded per policy (`behavior.maxDeletionsPerSecond`,
token bucket, burst == rate) with a controller default of 10/s, and per-batch
by `behavior.batchSize`. There is no unbounded delete path.

## 5. Emergency disable procedure

```bash
kubectl -n <release-namespace> set env deploy/zen-cleaner \
  ZEN_CLEANER_DELETE_ENABLED=false
kubectl -n <release-namespace> rollout status deploy/zen-cleaner
```

With the stop engaged the controller keeps evaluating policies and logging
what it would delete (`zen_cleaner_dry_run_candidates_total` continues, every
refusal is counted as `deletes_disabled`), so you keep full observability
while all destructive action is suspended. Re-enable with
`ZEN_CLEANER_DELETE_ENABLED=true`.

## 6. Keeping the document honest

- `pkg/safety/safety_test.go` — the safety matrix as a table test.
- `pkg/safety/fuzz_test.go` — fuzz invariants (protected kinds/namespaces are
  never allowed, for any input).
- `pkg/controller/uid_precondition_test.go` — stale-UID/refusal/no-op laws.
- `pkg/validation/*_test.go` — validation law table tests.
- `deploy/rbac_parity_test.go` — the RBAC ruleset is exactly what the
  controller needs (read+delete only on targets; no secrets rule; no
  escalation verbs).
