# Security-stack integration examples

Policies validated end-to-end (2026-11-14, zen-cleaner 0.1.20, k3s v1.35.5 disposable
cluster) against the REAL operators installed from their public charts: trivy-operator 0.38.1
(app 0.36.1) and Kyverno 1.20.0 (chart 3.10.0). Every example starts with `dryRun: true` —
review the candidate list, then flip to `dryRun: false`.

| File | Targets | TTL mode |
|---|---|---|
| trivy-stale-reports.yaml | VulnerabilityReport (aquasecurity.github.io) | relative — 7d after `report.updateTimestamp` |
| kyverno-policyexception-expiry.yaml | PolicyException (policies.kyverno.io/v1) | fixed — 48h after creation |
| kyverno-policyreport-prune.yaml | Labeled disposable PolicyReport (wgpolicyk8s.io/v1alpha2) | fixed — 1h after creation, label-narrowed |
| falco-forensic-pods.yaml | Pod (quarantine-labeled) | fixed — 2h after creation, terminal-phase only |

Install RBAC must list every targeted kind explicitly (the chart refuses partially-set
indexes), e.g.:

```bash
helm upgrade --install zen-cleaner \
  oci://ghcr.io/zenmesh/charts/zen-cleaner --version 0.1.20 \
  --namespace zen-cleaner-system --create-namespace \
  --set rbac.targetKinds[0].group=batch --set rbac.targetKinds[0].resource=jobs \
  --set-string rbac.targetKinds[1].group= --set-string rbac.targetKinds[1].resource=configmaps \
  --set rbac.targetKinds[2].group= --set rbac.targetKinds[2].resource=pods \
  --set rbac.targetKinds[3].group=aquasecurity.github.io --set rbac.targetKinds[3].resource=vulnerabilityreports \
  --set rbac.targetKinds[4].group=policies.kyverno.io --set rbac.targetKinds[4].resource=policyexceptions \
  --set rbac.targetKinds[5].group=wgpolicyk8s.io --set rbac.targetKinds[5].resource=policyreports \
  --set rbac.deleteNamespaces[0]=default
```

Truthful caveats (verified, not assumed):

- **Pod deletion is phase-gated by design**: `Running` pods are always refused
  (`workload_managed` without the opt-in; `active_workload` even with it). Only
  Succeeded/Failed pods are deletable — make forensic capture containers exit when done.
- **CRD support scope**: zen-cleaner's API accepts CRDs, and these Trivy/Kyverno kinds were
  exercised end-to-end on k3s v1.35.5 with the chart's RBAC path — the wider claim "CRDs not
  validated" still applies to kinds other than the ones listed here.
- **PolicyException on Kyverno 1.20**: use `policies.kyverno.io/v1` (the legacy kyverno.io/v2
  API no longer accepts creates on 1.20). Modern Kyverno also offers native `spec.expiresAt` —
  zen-cleaner is for older versions or as a belt-and-suspenders backstop.
- **PolicyReport creation-age pruning sweeps active reports**: the wgpolicyk8s PolicyReport
  schema carries NO update-timestamp field (verified against the live CRD installed by
  Kyverno 1.20), and Kyverno's reports-controller updates reports IN PLACE — so creation age
  cannot distinguish active from stale. The shipped example is therefore label-narrowed to
  deliberately labeled disposable reports; do not remove the labelSelector for namespace-wide
  pruning.
- **Falcosidekick does not label pods**: its outputs (webhook, slack, loki, elasticsearch,
  kafka, opsgenie, pagerduty, policyreport, ...) deliver events; none applies a Kubernetes
  label to the offending pod. The quarantine label in falco-forensic-pods.yaml is applied by
  EXTERNAL automation (e.g., a webhook-driven controller reacting to the Falco event) — that
  automation is a prerequisite, not part of zen-cleaner or falcosidekick.
- **Falcosidekick's policyreport output writes openreports.io/v1alpha1 `Report` objects** (a
  different kind from wgpolicyk8s PolicyReports, with its own self-pruning bounds); a
  zen-cleaner policy for those would target kind `Report` / apiVersion `openreports.io/v1alpha1`
  and was NOT exercised in this validation.
- **Trivy reports in the validation were representative CRs**, not operator-generated scans:
  the scan job could not download the vulnerability DB in the sandbox network, so the
  scan-to-report leg is NOT proven; the CRD shapes and the zen-cleaner deletion behavior on
  those kinds ARE validated.
- **Severity-mapped TTLs** map a single string FIELD on the target to a TTL budget;
  VulnerabilityReports carry severity per-vulnerability inside an array, so severity SLAs on
  Trivy reports are expressed today as one policy per severity class (labelSelector + fixed
  TTL) with the labels applied by your admission/CI tooling.
