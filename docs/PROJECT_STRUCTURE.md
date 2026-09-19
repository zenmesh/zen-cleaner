# zen-cleaner Project Structure

## Overview

`zen-cleaner` is a Kubernetes controller for automatic cleanup of Kubernetes resources (validated on Pod, ReplicaSet, ConfigMap, Secret, Job). It is available as free open-source software (Apache-2.0) from the [Zen Mesh](https://zen-mesh.io) team.

## Ecosystem Boundary

### zen-cleaner (this project)

- **Free OSS Kubernetes cleanup controller**
- Declarative cleanup policies with TTL, selectors, conditions, and rate limiting
- Works with any Kubernetes resource type (validated: Pod, ReplicaSet, ConfigMap, Secret, Job)
- Fully independent — no Zen Mesh required
- Apache-2.0 licensed for community use, self-managed clusters, and contributions

### Zen Mesh

- **Webhook delivery and data-plane operations platform**
- Private-network webhook delivery, edge/data-plane operations, logs, evidence, and operational truth
- Designed for teams that need cryptographically verified webhook delivery behind NAT, VPN, or firewalls
- Separate from zen-cleaner; Zen Mesh does not depend on zen-cleaner and zen-cleaner does not depend on Zen Mesh

### Relationship

Both projects are built by **Zen Mesh Inc.** and share engineering practices and infrastructure, but they serve different needs. zen-cleaner is useful independently on any Kubernetes cluster (validated on kind, k3d, kubeadm) and does not require a Zen Mesh subscription.

## Project Goals

1. **Provide Value**: Solve the real-world problem of resource cleanup in Kubernetes
2. **Reliable**: Build a well-tested, observable, and maintainable controller
3. **Community Driven**: Open source project that welcomes contributions and feedback
4. **Future**: If widely adopted, may be proposed as a Kubernetes Enhancement Proposal (KEP)

## Project Structure

```
zen-cleaner/
├── docs/
│   ├── KEP_GENERIC_GARBAGE_COLLECTION.md  # KEP document
│   ├── IMPLEMENTATION_ROADMAP.md          # Implementation plan
│   └── PROJECT_STRUCTURE.md               # This file
├── cmd/
│   └── zen-cleaner/                     # Main controller binary
├── pkg/
│   ├── controller/                        # Zen Cleaner controller implementation
│   ├── api/                               # ZenCleanerPolicy CRD
│   └── validation/                        # GVR and policy validation
├── deploy/
│   ├── crds/                              # CRD definitions
│   └── manifests/                         # Deployment manifests
├── examples/                               # Example cleanup policies
├── test/                                   # Integration tests
└── README.md                               # Project overview
```

## Development Status

### Current Status

- ✅ Full-featured Zen Cleaner controller implementation
- ✅ Comprehensive CRD with flexible TTL and condition support
- ✅ Production-oriented features (rate limiting, metrics, HA)
- ✅ Extensive test coverage (~65% overall unit tests; `make coverage`)
- ✅ Complete documentation
- ✅ Open source and actively maintained

### Future Considerations

If zen-cleaner gains significant community adoption and proves valuable to the Kubernetes ecosystem, it may be proposed as a Kubernetes Enhancement Proposal (KEP) to potentially become part of upstream Kubernetes. However, this is not the primary goal—the focus is on providing a useful, community-driven solution.

## Key Principles

1. **Kubernetes-Native**: Uses standard Kubernetes patterns
2. **Generic**: Works with any Kubernetes resource (validated: Pod, ReplicaSet, ConfigMap, Secret, Job)
3. **Community-Driven**: Open source, community feedback welcome
4. **Observable**: Built-in rate limiting, metrics, and observability

## Next Steps

1. Review and refine KEP document
2. Gather initial community feedback
3. Implement PoC when ready
4. Continue open source development
5. Submit KEP after validation

