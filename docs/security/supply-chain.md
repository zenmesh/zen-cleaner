# zen-cleaner — Supply Chain Security

This document describes zen-cleaner's supply chain security posture. As of June 2026, govulncheck passes with zero reachable vulnerabilities.

## Build chain

- **Language:** Go 1.26.4 (compiled to static binaries)
- **Base image:** `golang:1.26-alpine` (Docker multi-stage build)
- **Final image:** `alpine:3.21` — pinned by SHA digest
- **Build platform:** GitHub Actions (`ubuntu-latest`)

## Dependency management

- **Go modules:** Pinned in `go.mod` / `go.sum` with checksums
- **Dependabot:** Enabled for Go modules, Docker, and GitHub Actions
- **Vendor directory:** Not used (pure `go mod` build)

## Vulnerability scanning

| Scanner | Scope | When | Blocking |
|---------|-------|------|----------|
| govulncheck | Go module vulnerabilities | Every CI run | Yes |
| gosec | Go source code issues | Every CI run | Yes |
| Trivy | Container image vulnerabilities | Every CI run | No (report only) |
| Dependabot | Go + Docker + Actions | Daily | Alerts only |

## Published artifacts

| Artifact | Published to | Signed | SBOM | Provenance |
|----------|-------------|--------|------|------------|
| Container image | Docker Hub (`zenmesh/zen-cleaner`) | ❌ No | ❌ No | ❌ No |
| Helm chart | Artifact Hub | ❌ No | ❌ No | ❌ No |
| Source code | GitHub | ✅ git tag | N/A | N/A |

## Known gaps

- **No SBOM generation** — CycloneDX/SPDX not produced during build
- **No image signing** — cosign or similar not configured
- **No SLSA provenance** — build attestation not published
- **No vendor directory** — no pinned copy of dependencies in-repo
- **No FIPS-compliant build** — standard Go crypto only

## CI/CD trust

- Workflows defined in `.github/workflows/` — reviewed on PR
- `CODEOWNERS` requires maintainer review for workflow changes
- Dependabot PRs auto-generated for dependency updates
- No third-party CI secrets or custom runners

## Improvement roadmap

1. Generate SBOM (CycloneDX) during CI build
2. Sign container images with cosign
3. Publish SLSA provenance attestations
4. Vendor dependencies for reproducible builds
5. Add FIPS-compliant build option
