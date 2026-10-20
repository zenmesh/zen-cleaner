# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.2] — 2026-10-20

### Fixed
- Stable plain-version tags trigger the release workflow; release versions must match chart metadata.
- Chart publication uses the GitHub Container Registry and the repository's built-in Actions token.

### Added
- OCI Helm chart and release tarball/checksum, pinned to the exact published multi-architecture image digest.

### Changed
- Includes dependency updates merged after the 0.1.1 image release and the corrected README video links.

## [0.1.0] — 2026-10-18

First installable public release. Per SemVer §4 this is an early-development
(0.x) release: the CRD is `v1alpha1` and may still change incompatibly. See
RELEASING.md for the honest compatibility expectations.

### Added
- `main` branch protection: pull request required, five required checks
  (Lint, Test, Build, Go Vulnerability Check, Standalone OSS — credential-free
  build + private-dependency guards), resolved review conversations, no force
  pushes, no branch deletion — enforced for administrators too.
- Automatic deletion of merged PR branches; `main` is the only long-lived
  branch.
- Helm chart metadata for Artifact Hub (home/sources/maintainers/keywords,
  license, changes and links annotations) and truthful chart docs.

### Changed
- Builder image updated to a newer pinned golang digest (go 1.27.2), keeping
  the build reproducible and current with upstream security fixes.
- Chart `image.tag` default, README install examples and the evidence table
  reconciled to the released version.

### Fixed
- Documentation defects: `docs/evidence/README.md` claimed a packaged
  `docs/zen-cleaner-0.0.1-alpha.tgz` chart archive that has never existed, and
  listed the image as `zenmesh/zen-cleaner` rather than the real
  `ghcr.io/zenmesh/zen-cleaner`. Missing-evidence rows were overstated as
  failures rather than honestly marked as not yet produced.
- RELEASING.md corrected: `0.x` is early development and not automatically a
  prerelease; the obsolete permanent `release/vX.Y.Z` branch procedure was
  removed; the incorrect instruction to update `go.mod` with the release
  version was replaced with the truth (go.mod declares the toolchain; the
  binary version is stamped from the git tag).

## [Unreleased]

- Declarative `ZenCleanerPolicy` CRD (cleaner.zen-mesh.io/v1alpha1) with TTL modes
  (fixed, field-based, mapped, relative), label/field/annotation/phase conditions,
  dry-run, per-policy rate limiting and batching.
- Controller with leader election, Prometheus metrics, Kubernetes events, and
  validating/mutating admission webhooks.
