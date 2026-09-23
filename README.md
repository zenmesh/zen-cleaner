# Zen Cleaner

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?logo=go)](https://go.dev/)
[![CI](https://github.com/zenmesh/zen-cleaner/workflows/CI/badge.svg)](https://github.com/zenmesh/zen-cleaner/actions)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.26+-326CE5?logo=kubernetes&logoColor=white)](https://kubernetes.io/)

Kubernetes controller for **declarative resource cleanup**: define **`ZenCleanerPolicy`** objects (TTL, selectors, dry-run, rate limits) and let zen-cleaner delete matching resources safely.

**zen-cleaner** is a free and open source (Apache-2.0) **Kubernetes cleanup controller** from the **[Zen Mesh](https://zen-mesh.io)** team. It is designed for community use, self-managed Kubernetes clusters, and contributions. zen-cleaner is **complementary to but independent of** Zen Mesh — you can use zen-cleaner without Zen Mesh, and Zen Mesh does not require zen-cleaner.

- **Clean up** completed Jobs, temporary ConfigMaps, expired Secrets, evicted Pods, orphaned ReplicaSets, released PVCs, and other Kubernetes resources via declarative cleanup policies.
- **Declarative TTL policies** — fixed, field-based, mapped, and relative expiration modes.
- **Safe, rate-limited, observable** — dry-run mode, Prometheus metrics, Kubernetes events, leader election.

**Related project — Zen Mesh:** [zen-mesh.io](https://zen-mesh.io) is a webhook delivery and data-plane operations platform for private Kubernetes networks from the same team. zen-cleaner is an independent Kubernetes cleanup controller; the two projects are separate and independent.

**Security:** report vulnerabilities to **[security@zen-mesh.io](mailto:security@zen-mesh.io)** or via [GitHub Security Advisories](https://github.com/zenmesh/zen-cleaner/security) — see [SECURITY.md](SECURITY.md).

Builds require **Go 1.27+** (see `go.mod`). Published container images are built with Go **1.27**.

## Quick start

### With kubectl

From a clone of this repo (paths match the tree layout):

```bash
kubectl apply -f deploy/crds/cleaner.zen-mesh.io_zencleanerpolicies.yaml
kubectl apply -f deploy/manifests/namespace.yaml
kubectl apply -f deploy/manifests/rbac.yaml
# Point deploy/manifests/deployment.yaml at your image (build locally or use a registry tag).
kubectl apply -f deploy/manifests/deployment.yaml
```

The validating admission webhook requires **TLS** in the cluster before you rely on admission enforcement. **Recommended path:** cert-manager + Secret `zen-cleaner-webhook-cert` + `deploy/webhook/validating-webhook.yaml`. Full steps: **[docs/WEBHOOK_TLS.md](docs/WEBHOOK_TLS.md)** (includes the manual-CA flow used by `scripts/comprehensive_e2e.sh` on kind).

Apply an example policy:

```bash
kubectl apply -f examples/temp-configmap-cleanup.yaml
```

**Remote raw manifests** (replace branch/tag as needed):

```bash
kubectl apply -f https://raw.githubusercontent.com/zenmesh/zen-cleaner/main/deploy/crds/cleaner.zen-mesh.io_zencleanerpolicies.yaml
kubectl apply -f https://raw.githubusercontent.com/zenmesh/zen-cleaner/main/deploy/manifests/namespace.yaml
kubectl apply -f https://raw.githubusercontent.com/zenmesh/zen-cleaner/main/deploy/manifests/rbac.yaml
kubectl apply -f https://raw.githubusercontent.com/zenmesh/zen-cleaner/main/deploy/manifests/deployment.yaml
kubectl apply -f https://raw.githubusercontent.com/zenmesh/zen-cleaner/main/examples/temp-configmap-cleanup.yaml
```

### With Helm

Install the Helm chart straight from this repository:

```bash
helm install zen-cleaner ./deploy/helm/zen-cleaner --namespace zen-cleaner-system --create-namespace
kubectl apply -f https://raw.githubusercontent.com/zenmesh/zen-cleaner/main/examples/temp-configmap-cleanup.yaml
```

### Minimal policy example

```yaml
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: cleanup-temp-configmaps
spec:
  targetResource:
    apiVersion: v1
    kind: ConfigMap
    labelSelector:
      matchLabels:
        temporary: "true"
  ttl:
    secondsAfterCreation: 3600
  behavior:
    maxDeletionsPerSecond: 10
```

## Why zen-cleaner?

Kubernetes only provides built-in TTL support for **Jobs**. For ConfigMaps, Secrets, Pods, CRDs, and everything else, you either write bespoke controllers or operate without automation. zen-cleaner fills that gap with plain YAML policies.

## Powerful TTL options

zen-cleaner supports **fixed**, **field-based**, **mapped**, and **relative** TTL modes — see [User Guide](docs/USER_GUIDE.md) for details.

### Fixed TTL

```yaml
ttl:
  secondsAfterCreation: 604800
```

### Field-based TTL

```yaml
ttl:
  fieldPath: "spec.ttlSeconds"
```

### Mapped TTL

```yaml
ttl:
  fieldPath: "spec.severity"
  mappings:
    CRITICAL: 1814400
    HIGH: 1209600
    MEDIUM: 604800
    LOW: 259200
  default: 604800
```

### Relative TTL

```yaml
ttl:
  relativeTo: "status.lastProcessedAt"
  secondsAfter: 86400
```

## Key benefits

- Works with Kubernetes resources matched by label/field selectors
- Policies as CRDs — no custom binaries per workload
- Rate limiting, metrics, events, leader election
- Dry-run mode to validate behavior before destructive deletes
- Observable via Prometheus and Kubernetes events

## Use cases

- Clean up completed Jobs and temporary CI/CD ConfigMaps/Secrets
- Remove evicted Pods, orphaned ReplicaSets, released PVCs
- Namespace-scoped or cluster-scoped policies with selectors

## Documentation

- **[Webhook TLS (production)](docs/WEBHOOK_TLS.md)**: cert-manager vs manual CA — matches how we test on kind
- **[Linting](docs/LINTING.md)**: golangci-lint scope and known debt
- **[KEP-style design](docs/KEP_GENERIC_GARBAGE_COLLECTION.md)**: background and API notes
- **[API Reference](docs/API_REFERENCE.md)**
- **[User Guide](docs/USER_GUIDE.md)**
- **[Operator Guide](docs/OPERATOR_GUIDE.md)**
- **[Metrics](docs/METRICS.md)**
- **[Security (operations)](docs/SECURITY.md)** and [SECURITY.md](SECURITY.md) (disclosure policy)
- **[Secret / TLS storage](docs/SECRET_MANAGEMENT.md)**
- **[Version compatibility](docs/VERSION_COMPATIBILITY.md)**
- **[Architecture](docs/ARCHITECTURE.md)**
- **[Examples](examples/)**
- **[Contributing](CONTRIBUTING.md)**
- **[Governance](GOVERNANCE.md)** · **[Maintainers](MAINTAINERS.md)** · **[Releasing](RELEASING.md)** · **[Adopters](ADOPTERS.md)**

## Features

- Resource support (validated: Pod, ReplicaSet, ConfigMap, Secret, Job; CRDs not validated)
- Four TTL modes: fixed, field-based, mapped, relative
- Label / field selectors and conditions
- Rate limiting and dry-run
- Prometheus metrics, Kubernetes events, leader election
- Test suite with CI and optional kind e2e (`make e2e-kind`)

## Status

zen-cleaner is a public repository available for community use, feedback, and contributions. It is actively maintained as open source under the Apache-2.0 license. This is not an official product launch — zen-cleaner is a free community tool built by the Zen Mesh team.

## Public surface

- **Source of truth:** this GitHub repository — all documentation, evidence, and issue tracking live here
- **Public trust & evidence:** [docs/evidence/README.md](docs/evidence/README.md)
- **Claims and maturity:** [docs/claims.md](docs/claims.md)
- **AI context:** [llms.txt](llms.txt)
- **No hosted website:** zen-cleaner does not have a dedicated website; `zen-mesh.io` is a parked/legacy domain that is not used for zen-cleaner

## From the Zen Mesh team

**zen-cleaner** is a free, independent Apache-2.0 OSS project from the same team that builds **Zen Mesh**. The two projects are **separate and independent** — zen-cleaner does not require Zen Mesh, and Zen Mesh does not require zen-cleaner.

| Project | Description | License |
|---------|-------------|---------|
| [zen-cleaner](https://github.com/zenmesh/zen-cleaner) | Free OSS Kubernetes cleanup controller (this project) | Apache-2.0 |
| [Zen Mesh](https://zen-mesh.io) | Webhook delivery and data-plane operations platform | Separate project |

## References

- [Kubernetes TTL-after-finished](https://kubernetes.io/docs/concepts/workloads/controllers/ttlafterfinished/)
- [Kubernetes Enhancement Proposals](https://github.com/kubernetes/enhancements)

## Links

- **GitHub Repository**: [github.com/zenmesh/zen-cleaner](https://github.com/zenmesh/zen-cleaner)
- **Documentation Index**: [docs/INDEX.md](docs/INDEX.md)
- **Helm**: `deploy/helm/zen-cleaner` chart in this repository
- **Explainer video**: [YouTube](https://www.youtube.com/watch?v=P8afhcgjWVQ&list=PL1AGc_sKXJBdInu0yffTJxN828oaCuqwx)
- **Zen Mesh Inc.**: [zen-mesh.io](https://zen-mesh.io)
