# zen-cleaner — Canonical Documentation Map

## Quick start

- [README](../README.md) — overview, install, usage
- [INDEX](INDEX.md) — documentation index
- [User Guide](USER_GUIDE.md) — creating and managing policies

## Reference

- [API Reference](API_REFERENCE.md) — CRD field reference
- [Architecture](ARCHITECTURE.md) — controller design and flow
- [RBAC](RBAC.md) — permissions and security model
- [Metrics](METRICS.md) — Prometheus metrics reference
- [CLI Reference](https://github.com/zenmesh/zen-cleaner) — controller flags (see `--help`)

## Development

- [Development Guide](DEVELOPMENT.md) — contributing, building, testing
- [Testing](TESTING.md) — test patterns and coverage
- [Linting](LINTING.md) — code style and lint rules
- [CI/CD](CI_CD.md) — pipeline and quality gates
- [Project Structure](PROJECT_STRUCTURE.md) — repo layout
- [License Headers](LICENSE_HEADERS.md) — Apache-2.0 header policy

## Operations

- [Operator Guide](OPERATOR_GUIDE.md) — install, configure, maintain
- [Security (operations)](SECURITY.md) — pod security, audit, RBAC
- [Leader Election](LEADER_ELECTION.md) — HA configuration
- [Webhook TLS](WEBHOOK_TLS.md) — admission webhook TLS
- [Secret Management](SECRET_MANAGEMENT.md) — webhook certificate management
- [Image Security](IMAGE_SECURITY.md) — base image pinning and scanning
- [Disaster Recovery](DISASTER_RECOVERY.md) — backup, restore, emergency stop
- [Benchmarks](BENCHMARKS.md) — performance characteristics
- [Optimization Opportunities](OPTIMIZATION_OPPORTUNITIES.md) — known improvements

## Public trust and evidence

- [llms.txt](../llms.txt) — AI discovery index
- [Claims and Maturity](claims.md) — what zen-cleaner does and does not do
- [Evidence Index](evidence/README.md) — catalog of tested claims and gaps
- [K8s v1.36 Evidence (kind)](evidence/kubernetes/v1.36/kind.md) — kind + K8s v1.36.1
- [K8s v1.36 Evidence (k3d)](evidence/kubernetes/v1.36/k3d.md) — k3d + K3s v1.36.2+k3s1
- [K8s v1.36 Evidence (kubeadm, PASS)](evidence/kubernetes/v1.36/kubeadm.md) — kubeadm + K8s v1.36.x (CRD/API + runtime + cleanup PASS, containerd 2.2.5)
- [K8s v1.34 Evidence (kubeadm, PASS)](evidence/kubernetes/v1.34/kubeadm.md) — kubeadm + K8s v1.34.9 (CRD/API + runtime + cleanup PASS, containerd 2.2.5)
- [Release Evidence](evidence/releases/README.md) — per-release test results
- [Kubernetes Compatibility](compatibility/kubernetes.md) — evidenced K8s version support

## Security

- [Security Policy](../SECURITY.md) — vulnerability reporting
- [Threat Model](security/threat-model.md) — controller threat model
- [Supply Chain](security/supply-chain.md) — build, scan, provenance

## AI interfaces

- [AI Status](ai/status.json) — maturity JSON
- [AI Evidence Index](ai/evidence-index.json) — evidence index JSON

## Versioning

- [Version Compatibility](VERSION_COMPATIBILITY.md) — K8s version matrix, CRD migration

## Releases

- [Release Guide](RELEASE.md) — release process
- [Helm chart](../deploy/helm/zen-cleaner) — Helm chart source

## Meta

- [KEP Design Document](KEP_GENERIC_GARBAGE_COLLECTION.md) — upstream KEP proposal
- [Governance](../GOVERNANCE.md) — project governance
- [Maintainers](../MAINTAINERS.md) — maintainers list
- [Contributing](../CONTRIBUTING.md) — contribution guide
- [Code of Conduct](../CODE_OF_CONDUCT.md)
- [License](../LICENSE) — Apache-2.0
