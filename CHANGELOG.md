# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.9] — 2026-10-30

### Fixed
- Release-workflow chart job: the AH metadata artifact push now runs after the metadata file lands and uses the correct artifact CONFIG media type. (Tag 0.1.8 was created with mismatched chart metadata and produced no artifacts — version burned per the immutable-tag policy; 0.1.7's OCI chart artifact exists but has no GitHub release.)

## [0.1.7] — 2026-10-29

### Added
- Artifact Hub repository-metadata artifact (tag `artifacthub.io`, config media type `application/vnd.cncf.artifacthub.config.v1+yaml`) published alongside every chart, keeping the registered repository's metadata (ID `b1179c76…`, verified-publisher claim) aligned automatically.

## [0.1.6] — 2026-10-29

### Fixed
- Startup Lease-NotFound no longer emits a false `APIUnreachable` event pair: the first lease GET on a fresh cluster returns NotFound (the object simply does not exist yet) and the reachability observer classified any error as unreachable. API-response errors (NotFound/401/403/conflict/...) now classify as reachable — only transport failures mean unreachable — with a regression test at the lock observer.

## [0.1.5] — 2026-10-29

### Added
- The structured-logging event sink is retained on the broadcaster: every Kubernetes event (including `APIUnreachable`) is mirrored to the controller's structured log, so failure evidence remains visible in `kubectl logs` while the API event sink cannot deliver — the exact condition NetworkPolicy-enforced egress rejection produces.
- `networkPolicy.egressCidr` (default `0.0.0.0/0`, unchanged behavior): narrow the 443/6443 API allowance to the cluster node/pod network for a tighter posture on clusters with known address ranges.

## [0.1.4] — 2026-10-29

### Fixed
- **Default egress NetworkPolicy now permits the Kubernetes API path under enforcement on k3s.** The embedded kube-router policy engine matches egress rules against the POST-DNAT endpoint tuple, so an API dial (ClusterIP:443) arrives at the policy chain as endpoint:6443 and a 443-only rule denied it — the controller stayed leaderless while the pod reported Ready (0.1.2/0.1.3 under enforcement). The rule now allows TCP/443 and TCP/6443 to any destination; enforcement of non-listed ports was re-verified unchanged by probe matrix on k3s v1.37.0 and v1.35.5.
- `APIUnreachable` pod events fire without requiring `POD_NAME` to be projected (HOSTNAME fallback).

## [0.1.3] — 2026-10-29

### Added
- API-reachability observation on the leader-election lock: unreachable states surface as `APIUnreachable` Warning events on the pod, a `zen_cleaner_api_reachable` metric, `/leaderz` detail, and a NOT-Ready `/readyz`. Follower (standby) status is never conflated with failure — a reachable standby stays Ready per the §11-§12 health law.

### Fixed
- The readiness probe now targets `/readyz` (the leadership-aware readiness law). 0.1.2 probed `/healthz` — the always-OK process-level liveness endpoint — which left pods reporting Ready while unable to reach the Kubernetes API (observed under egress NetworkPolicy enforcement on k3s; see the supported-environments note in README).

## [0.1.2] — 2026-10-20

### Fixed
- Stable plain-version tags trigger the release workflow; release versions must match chart metadata.
- Chart publication uses the GitHub Container Registry and the repository's built-in Actions token.
- Builder toolchain bumped to Go 1.27.3 (`golang:1.27.3-alpine`, digest-pinned after re-verifying the Alpine base), carrying the GO-2026-6809 `crypto/x509` fix; the `go.mod` language version is raised to match, so CI and the shipped image build on the fixed toolchain.

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
