# Release Process

This document describes the release process for zen-cleaner.

## Release Types

**A `0.x` version is an EARLY-DEVELOPMENT version, not automatically a
prerelease.** Per SemVer 2.0 §4, anything before `1.0.0` signals that the API
may still change; whether an individual `0.x` release is *published* as a
prerelease is a separate, deliberate choice made at release time.

In this repository that choice is explicit:

- A release tagged `0.1.0` (no suffix) is a normal, installable public
  release — this is what the first public release uses.
- A release tagged `0.2.0-alpha` (a SemVer pre-release suffix) is published as a
  GitHub **prerelease**, which is marked "pre-release" on the release page and
  is excluded from Artifact Hub's stable-version display.

Do not infer "prerelease" from the `0.` alone. Version `0.0.1-alpha` was an
earlier prerelease; `0.1.0` is the first installable public release.

### Stability expectations under `1.0.0`

Stated plainly so users are not surprised:

- The CRD is `v1alpha1` and **may change incompatibly** in any `0.x` release.
- `0.x` releases carry no compatibility guarantee between minor versions.
- Patch releases (`0.1.1`) are expected to be backward compatible within the
  same minor.

No HA, SOC2, or production-qualification claim is made for `0.x`.

## Release Checklist

### Before Release

- [ ] All tests pass (`make test`)
- [ ] Code is linted (`make lint`)
- [ ] Security checks pass (`make security-check`)
- [ ] Documentation is up-to-date
- [ ] CHANGELOG.md is updated with release notes
- [ ] Version numbers are updated in:
  - [ ] `deploy/helm/zen-cleaner/Chart.yaml` (`version` + `appVersion`)
  - [ ] `deploy/helm/zen-cleaner/values.yaml` (`image.tag`)
  - [ ] `README.md` install examples
  - [ ] `CHANGELOG.md`

  **`go.mod` is deliberately NOT a release-version file.** Its `go` directive
  declares the required *language/toolchain* version (currently `go 1.27.2`);
  editing it to match a release number would break builds and mean nothing.
  The binary version is stamped at build time from the git tag via the
  `VERSION` build arg (see `.github/workflows/release-image.yml`).

### Creating a Release

**`main` is the only long-lived branch.** There is no permanent
`release/vX.Y.Z` branch: version-bump work happens on a short-lived branch and
is merged to `main` through a pull request, so every release point is exactly
`main`. Branches are auto-deleted after merge.

1. **Branch from `main`** and update the version files listed above, then open a
   PR. `main` is protected: the five required checks must pass and review
   conversations must be resolved before it merges.

2. **Tag the merged commit** — tagging is what publishes the image:

   ```bash
   git checkout main && git pull
   git tag -a X.Y.Z -m "Release X.Y.Z"
   git push origin X.Y.Z
   ```

   The tag push triggers `.github/workflows/release-image.yml`, which builds
   and pushes the multi-arch image and records the published digest. Tags are
   immutable: **never move or delete a published tag.** To fix a bad release,
   publish the next patch version.

3. **Create the GitHub Release** against that tag, using the CHANGELOG entry.
   Mark it a prerelease only if the tag carries a SemVer pre-release suffix.

4. **Record the digest** published by the workflow in the release notes, and
   keep it pinned in the chart/docs.

### After Release

- [ ] Verify release artifacts are available
- [ ] Update documentation if needed
- [ ] Announce release (if applicable)

## Release Notes

Release notes should include:

- **Added**: New features
- **Changed**: Changes in existing functionality
- **Deprecated**: Soon-to-be removed features
- **Removed**: Removed features
- **Fixed**: Bug fixes
- **Security**: Security fixes

See [CHANGELOG.md](CHANGELOG.md) for the format.

## Hotfixes

For critical bug fixes:

1. Create hotfix branch from the release tag
2. Apply fix
3. Create patch release (e.g., v1.0.1)
4. Merge hotfix back to main

## Semantic Versioning

This project follows [Semantic Versioning](https://semver.org/):

- **MAJOR** version for incompatible API changes
- **MINOR** version for backward-compatible functionality additions
- **PATCH** version for backward-compatible bug fixes


## Supply-chain hardening checklist (D039)

For every release, produce and attach:

1. **Checksums**: `sha256sum zen-cleaner-<ver>.tgz zen-cleaner-<ver>.txt > checksums.txt`
2. **SBOM**: `syft <image>:<tag> -o spdx-json > sbom-spdx.json` (attach to the GitHub release)
3. **Image digest**: record the immutable `sha256:` digest in the release notes; charts must reference the digest, not a mutable tag
4. **Vulnerability scan**: `trivy image --severity CRITICAL,HIGH <image>:<tag>` — attach the report; CRITICALs block the release
5. **Signing**: `cosign sign --key <key-ref> <image>@<digest>` — requires the project signing identity (owner-held); run as the release step once keys are available
6. **Provenance**: record build command + Go version + base image digest in the release notes

`latest` is a development-only tag and must never be the documented production install path.

## Public image publication (ghcr.io — the single path)

Images publish to `ghcr.io/zenmesh/zen-cleaner` from tag pushes by
`.github/workflows/release-image.yml` using the BUILT-IN `GITHUB_TOKEN`
(packages:write) — no Docker Hub or external-registry credential exists or is
needed. Publication failures fail the workflow visibly (no `continue-on-error`
login, `push: true`).

**One-time first-publish step (owner):** a package first created by
`GITHUB_TOKEN` defaults to PRIVATE. After the first tag release, set the
package visibility to public (Package settings → Visibility) so anonymous
readers can pull; anonymous pulls remain denied until then — the soft-launch
install path stays blocked without it.

**Digest law:** record the immutable digest in the release notes and pin it in
the README/chart instructions; tags never move (the old `build-multiarch`
hardcoded `0.0.1-alpha` on main pushes — removed).
