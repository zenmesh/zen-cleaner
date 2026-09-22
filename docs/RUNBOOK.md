# Zen Cleaner — Operator Runbook (RC1)

## Architecture in one paragraph

`zen-cleaner` is a two-replica controller (client-go leader election on a
`Lease`, 15s/10s/5s). Only the leader reconciles; standbys serve health
endpoints. Policies (`ZenCleanerPolicy`, group `cleaner.zen-mesh.io/v1alpha1`,
cluster-scoped CRD) select target resources by GVR + namespace +
labelSelector + optional conditions, evaluate a TTL (fixed / field-path /
mapped / relative-to), and delete expired objects through a safety gate
(`docs/SAFETY_MODEL.md`) with per-policy token-bucket rate limiting. An
admission webhook validates and defaults policies. Metrics on `:8080`,
health on `:8081` (`/healthz /readyz /startup /leaderz`), webhook on `:9443`.

## Deploy / upgrade

```bash
# from-zero install (immutable image tag required, :latest refused)
cd zen-cleaner
CLEANER_IMAGE=ghcr.io/zenmesh/zen-cleaner:<tag> \
WEBHOOK_NAMESPACE=zen-cleaner-system bash deploy/install.sh
kubectl -n zen-cleaner-system rollout status deploy/zen-cleaner
```

The installer applies CRDs, RBAC, webhook TLS bootstrap (self-signed, bound to
the in-cluster webhook service DNS names; no cert-manager required), the
Deployment (2 replicas, PDB when replicas>1 via Helm), and the admission
configs with the generated CA bundle. Helm is the alternative for released
installs (`helm upgrade --install` with a digest-pinned image); the Helm
chart runs `--enable-webhook=false` by design.

### Rollback

Images are immutable tags; rollback is redeploying the previous tag with the
same installer/Helm flow. The policy CRD schema has only ever been extended
(accepting old policies unchanged), so controller rollback within v1alpha1 is
schema-compatible. Verify after rollback: `kubectl -n zen-cleaner-system
rollout status deploy/zen-cleaner` and one policy status updating
(`lastGCRun` advancing).

## Emergency disable

```bash
kubectl -n zen-cleaner-system set env deploy/zen-cleaner \
  ZEN_CLEANER_DELETE_ENABLED=false
kubectl -n zen-cleaner-system rollout status deploy/zen-cleaner
```

Everything keeps evaluating and logging; nothing is deleted
(`zen_cleaner_safety_refusals_total{reason="deletes_disabled"}` climbs).
Re-enable with `=true`.

## Answering operator questions

| Question | Where to look |
|---|---|
| Is Cleaner healthy? | `:8081/healthz`, `:8081/readyz` (leader), pods Ready |
| Is it the leader? | `:8081/leaderz` per pod; lease `zen-cleaner-leader-election` |
| Which policies are active? | `kubectl get zencleanerpolicies -A` (`status.phase`, `Ready` condition) |
| How many candidates? | `status.resourcesMatched/Pending`; `zen_cleaner_candidates_considered_total` |
| Why was an object excluded? | `status.lastCycleRefusals` (bounded reason classes); `zen_cleaner_safety_refusals_total{reason}` |
| What was deleted? | `status.resourcesDeleted`; `zen_cleaner_resources_deleted_total{reason}`; Warning events |
| What failed? | `zen_cleaner_errors_total{error_type}`; `zen_cleaner_api_errors_total{class}`; events |
| Is the emergency stop on? | `zen_cleaner_safety_refusals_total{reason="deletes_disabled"}` > 0, or read the Deployment env |

## Troubleshooting

- **Pods NotReady after install**: webhook cert Secret missing → re-run
  `deploy/gen-webhook-cert.sh` (it is idempotent) and restart pods.
- **Policies stuck in `Error` with `PolicyInvalid`**: the policy fails
  runtime validation (see `docs/SAFETY_MODEL.md` §3) — fix the spec; typical
  causes: protected namespace/kind target, cluster-wide scope without a
  selector, `behavior.finalizer` set.
- **Objects not deleted**: check `status.lastCycleRefusals` first — the
  commonest cause is the exclusion label `zen-cleaner.zen-mesh.io/exclude:
  "true"` or TTL not yet expired (`resourcesPending`).
- **Leader flapping during an API outage**: expected. Leader election
  (15s/10s/5s) yields the lease when the API server is unreachable; the pod
  exits cleanly and re-enters election when connectivity returns. No cleanup
  runs while the API is down (fail-safe).
- **Two leaders / duplicate deletions**: impossible by construction — all
  reconcile work runs inside the leader-election callback; if you suspect it,
  capture the lease `holderIdentity` and both pods' "Started leading" log
  lines and file an issue.

## Alerts (Prometheus rules shipped in deploy/prometheus/)

`deletion_failed` rate, policies in `Error` phase, deletion duration. Add:
`safety_refusals_total{reason="stale_uid"}` growth (cache staleness signal)
and `api_errors_total` burn rate.
