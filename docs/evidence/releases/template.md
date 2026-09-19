# Release Evidence — vX.Y.Z

> Copy this template for each release. Replace placeholders. Delete sections that do not apply.

## Metadata

- **Version:** vX.Y.Z
- **Commit:** `<40-character-sha>`
- **Date:** YYYY-MM-DD
- **Go version:** 1.26.x
- **Kubernetes versions tested:**
  - k8s v1.31.x (envtest)
  - k8s v1.30.x (envtest)
  - k8s v1.29.x (envtest)
  - (add others if tested on real clusters)

## Tests run

- [ ] `go test ./...` — all pass
- [ ] `make test-unit` — race + coverage ≥ 65%
- [ ] `make test-integration` — integration tests pass
- [ ] `make e2e-kind` — end-to-end tests pass on kind
- [ ] (add additional test suites)

## Vulnerability scan status

- **govulncheck:** ✅ PASS / ❌ VULNERABILITIES FOUND
- **gosec:** ✅ PASS / ❌ ISSUES FOUND
- **Trivy (container):** ✅ PASS / ❌ ISSUES FOUND (non-blocking in CI)
- **Dependabot:** ✅ Up to date / ⚠️ Alerts open

## Image/build evidence

- **Image:** `zenmesh/zen-cleaner:vX.Y.Z`
- **Architectures:** linux/amd64, linux/arm64
- **Base image SHA:** `<sha256>`
- **Image digest:** `<sha256>`

## SBOM/provenance status

- **SBOM:** ✅ Generated (CycloneDX) / ❌ Not generated
- **SLSA provenance:** ✅ Published / ❌ Not published
- **Image signature (cosign):** ✅ Signed / ❌ Not signed

## Known limitations

- (list any known issues or regressions)
- (link to GitHub issues if applicable)

## Maintainer sign-off

- **Tested by:** `<name>`
- **Date:** YYYY-MM-DD
- **Approved by:** `<name>`
- **Date:** YYYY-MM-DD

## Artifacts

- Helm chart: `docs/zen-cleaner-vX.Y.Z.tgz`
- Container image: `docker.io/zenmesh/zen-cleaner:vX.Y.Z`
- Release tag: `vX.Y.Z`
