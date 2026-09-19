# zen-cleaner — Evidence Index

This index catalogs what is evidenced about zen-cleaner. Each category states whether evidence is **present**, **missing**, or **template-only**.

## CI/test evidence

| Evidence | Status | Location |
|----------|--------|----------|
| Unit tests | ✅ Present | `go test ./...` in CI (see `.github/workflows/ci.yml`) |
| Integration tests | ✅ Present | `test/integration/` run in CI |
| Coverage gate (65%) | ✅ Present | `make coverage` enforced in CI |
| Race detection | ✅ Present | `go test -v -race` in CI |
| E2E (kind) | ⚠️ Template-only | `make e2e-kind` available, not run in CI |

## Vulnerability scanning evidence

| Evidence | Status | Location |
|----------|--------|----------|
| govulncheck | ✅ Present (passing) | CI security job |
| gosec | ✅ Present | CI security job |
| Trivy (container) | ✅ Present | CI security job (non-blocking) |
| Dependabot alerts | ✅ Present | `.github/dependabot.yml` |

## Vulnerability remediation evidence

| Finding | Remediation | Evidence |
|---------|-------------|----------|
| GO-2026-4918 (http2 infinite loop) | `golang.org/x/net` v0.47.0 → v0.55.0 | `go.mod`, govulncheck PASS |
| GO-2026-5026 (idna ASCII-only bypass) | `golang.org/x/net` v0.47.0 → v0.55.0 | `go.mod`, govulncheck PASS |
| GO-2026-5039 (net/textproto injection) | Go toolchain 1.26.0 → 1.26.4 | `.github/workflows/*.yml`, govulncheck PASS |
| GO-2026-5038 (mime quadratic decode) | Go toolchain 1.26.0 → 1.26.4 | `.github/workflows/*.yml`, govulncheck PASS |
| GO-2026-5037 (x509 hostname parsing) | Go toolchain 1.26.0 → 1.26.4 | `.github/workflows/*.yml`, govulncheck PASS |
| GO-2026-4971 (net NUL byte panic) | Go toolchain 1.26.0 → 1.26.4 | `.github/workflows/*.yml`, govulncheck PASS |
| 9 additional stdlib vulns (x509, tls, os, url) | Go toolchain 1.26.0 → 1.26.4 | `.github/workflows/*.yml`, govulncheck PASS |

## Kubernetes compatibility evidence

| Evidence | Status | Notes |
|----------|--------|-------|
| client-go version | ✅ Present | `v0.35` — compatible with K8s 1.31.x API |
| CRD API version | ✅ Present | `apiextensions.k8s.io/v1` (via controller-runtime) |
| Tested on cloud K8s | ❌ Missing | No EKS/GKE/AKS test results published |
| Tested on kind (K8s v1.36.1) | ✅ Present | `docs/evidence/kubernetes/v1.36/kind.md` — CRD + runtime + real deletion matrix |
| Tested on k3d (K3s v1.36.2+k3s1) | ✅ Present | `docs/evidence/kubernetes/v1.36/k3d.md` — CRD + runtime + real deletion matrix |
| Tested on kubeadm (K8s v1.36.2) | ✅ Present | `docs/evidence/kubernetes/v1.36/kubeadm.md` — CRD/API + runtime + cleanup (containerd 2.2.5) |
| Tested on kubeadm (K8s v1.34.9) | ✅ Present | `docs/evidence/kubernetes/v1.34/kubeadm.md` — CRD/API + runtime + cleanup (containerd 2.2.5) |
| kind/k3d (K8s v1.34.x) | ❌ Not tested | Not validated on v1.34 beyond kubeadm |

## Release evidence

| Evidence | Status | Location |
|----------|--------|----------|
| git tags | ✅ Present | `v0.0.1-alpha` |
| Helm chart | ✅ Present | `docs/zen-cleaner-0.0.1-alpha.tgz` |
| Container image | ✅ Present | `zenmesh/zen-cleaner:0.0.1-alpha` |
| Release notes | ⚠️ Template-only | Template in `docs/evidence/releases/template.md` |
| SBOM | ❌ Missing | Not generated |
| SLSA provenance | ❌ Missing | Not published |
| Signed images | ❌ Missing | Not signed (no cosign) |

## CRD/API compatibility evidence

| Evidence | Status | Notes |
|----------|--------|-------|
| CRD manifest | ✅ Present | `deploy/crds/cleaner.zen-mesh.io_zencleanerpolicies.yaml` |
| API reference | ✅ Present | `docs/API_REFERENCE.md` |
| OpenAPI schema | ✅ Present | Embedded in CRD |
| Conversion webhook | ❌ Missing | No CRD version conversion |

## Security/supply-chain evidence

| Evidence | Status | Notes |
|----------|--------|-------|
| Base image pinned by SHA | ✅ Present | `Dockerfile` |
| Non-root container | ✅ Present | Security context in manifests |
| Read-only root FS | ✅ Present | Security context in manifests |
| Security policy | ✅ Present | `SECURITY.md` |
| Threat model | ⚠️ Template-only | `docs/security/threat-model.md` |
| Supply chain doc | ⚠️ Template-only | `docs/security/supply-chain.md` |

## Controller behavior evidence

| Evidence | Status | Notes |
|----------|--------|-------|
| Leader election unit tests | ✅ Tested | `internal/election/` tests |
| Leader election safety (2+3 replicas) | ✅ Validated | `docs/evidence/kubernetes/v1.36/leader-election.md` — multi-replica failover, no-double-delete |
| Leader election safety (kind) | ✅ PASS | 2 and 3 replicas, leader failover, no cross-leader conflicts |
| Rate limiting | ✅ Tested | `pkg/controller/rate_limiter_test.go` |
| TTL evaluation | ✅ Tested | `internal/ttl/` tests |
| Policy reconciliation | ✅ Tested | `pkg/controller/` tests |
| Error handling | ✅ Tested | `pkg/errors/` tests |

## Known gaps

| Gap | Impact | Tracker |
|-----|--------|---------|
| No cloud K8s E2E | Compatibility with specific providers unverified | — |
| No SBOM/provenance | Supply chain attestation absent | — |
| No fuzz testing | Edge case resilience unmeasured | — |
| No penetration test | Security boundary resilience unmeasured | — |
| No performance benchmarks in CI | Regression detection absent | `test/benchmarks/` exist but not in CI |

## Remediation history

| Date | Type | Summary | Commit |
|------|------|---------|--------|
| 2026-06-29 | Dependency | `golang.org/x/net` v0.47.0→v0.55.0 (fixes GO-2026-4918, GO-2026-5026) | _(current)_ |
| 2026-06-29 | Toolchain | Go 1.26.0→1.26.4 (fixes 12 stdlib CVEs across net/textproto, mime, x509, tls, os, url) | _(current)_ |
