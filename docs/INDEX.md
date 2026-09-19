# zen-cleaner Documentation

Welcome to the **zen-cleaner** documentation.

zen-cleaner is a free Apache-2.0 **Kubernetes cleanup controller** that provides declarative cleanup policies for Kubernetes resources. Define TTL-based, selector-based, and condition-based policies for Kubernetes resources (validated on Pod, ReplicaSet, ConfigMap, Secret, Job).

## Where this fits with Zen Mesh

**zen-cleaner** is an open-source community project from the **[Zen Mesh](https://zen-mesh.io)** team. Zen Mesh is a webhook delivery and data-plane operations platform for private Kubernetes networks.

- **zen-cleaner** is a **free, independent OSS Kubernetes cleanup controller** — useful on its own, no Zen Mesh required.
- **Zen Mesh** is a platform for webhook/edge/data-plane delivery, logs, evidence, and operational truth.
- The projects are **complementary but separate**. zen-cleaner does not require Zen Mesh, and Zen Mesh does not require zen-cleaner.

## Getting Started

- [Installation Guide](DEVELOPMENT.md#installation)
- [Quick Start](DEVELOPMENT.md#quick-start)

## User Documentation

- [User Guide](USER_GUIDE.md) — how to create and manage cleanup policies
- [Operator Guide](OPERATOR_GUIDE.md) — deploying and operating zen-cleaner in production
- [Architecture](ARCHITECTURE.md) — system design and reconciliation model
- [API Reference](API_REFERENCE.md) — full CRD and policy specification
- [Project Structure](PROJECT_STRUCTURE.md) — codebase layout and ecosystem boundary

## Development

- [Development Setup](DEVELOPMENT.md)
- [Contributing Guidelines](../CONTRIBUTING.md)
- [Release Process](RELEASE.md)

## Public Trust & Evidence

- [Claims and Maturity](claims.md) — what zen-cleaner does and does not do
- [Evidence Index](evidence/README.md) — catalog of tested claims and gaps
- [Kubernetes Compatibility](compatibility/kubernetes.md) — evidenced K8s version support

## Security

- [Security Policy](../SECURITY.md) — vulnerability disclosure
- [Threat Model](security/threat-model.md) — controller threat model
- [Supply Chain Security](security/supply-chain.md) — build, scan, provenance

## AI Interfaces

- [AI Status](ai/status.json) — maturity JSON
- [AI Evidence Index](ai/evidence-index.json) — evidence index JSON

## Resources

- [GitHub Repository](https://github.com/zenmesh/zen-cleaner)
- [README](../README.md) — project overview and quick start
- [Canonical Docs Map](CANONICAL_DOCS_MAP.md) — full document index
- [Zen Mesh](https://zen-mesh.io) — platform from the same team
- [Zen Mesh Documentation](https://docs.zen-mesh.io)
- [Code of Conduct](../CODE_OF_CONDUCT.md)
- [Changelog](../CHANGELOG.md)

