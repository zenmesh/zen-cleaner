# zen-cleaner — Release Evidence

This directory contains release evidence for zen-cleaner. Each release should document what was tested, what was scanned, and any known limitations.

## Current release

- **Version:** `0.0.1-alpha` (pre-release)
- **Commit:** `12b31bca14da24d3bf5b0ef5062186fc768f9015` (initial alpha tag)
- **Helm chart:** `docs/zen-cleaner-0.0.1-alpha.tgz`
- **Container image:** `zenmesh/zen-cleaner:0.0.1-alpha`

## Release evidence status

| Evidence | 0.0.1-alpha | Notes |
|----------|-------------|-------|
| Unit tests | ✅ Pass | 65%+ coverage gate |
| Integration tests | ✅ Pass | Local k8s API simulation |
| Vulnerability scan | ✅ Run | govulncheck + gosec (CI) |
| Container build | ✅ Pass | Multi-arch (amd64, arm64) |
| Helm chart validation | ✅ Pass | Published to Artifact Hub |
| E2E tests | ❌ Not run | `make e2e-kind` available, not in CI |
| SBOM | ❌ Not generated | — |
| Provenance attestation | ❌ Not published | — |
| Signed images | ❌ Not signed | — |

## Template

Use `template.md` when documenting a new release.
