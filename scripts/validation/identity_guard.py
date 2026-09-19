#!/usr/bin/env python3
"""Zen Cleaner product-identity regression guard.

Deterministic checks over the git-tracked tree (and the committed tree when
run with --committed):

1. LEGACY SCAN — product-owned occurrences of the old Zen GC / kube-zen
   identity must not re-enter the repository. Only narrowly classified
   technical/legal exceptions are allowed (none currently).
2. NEW-ID CONSISTENCY — the canonical new identity must be present and
   coherent: module path, binary, chart, CRD group/kind, image refs, env
   prefix, metric prefix, namespace.
3. LLMS/README CONSISTENCY — the machine-readable discovery surface (llms.txt)
   and the human README must agree on product, CRD/API names, and install
   paths, and must not make forbidden maturity/enterprise claims.

Exit code 0 = identity intact; nonzero = regression detected.
"""

import os
import subprocess
import sys

REPO = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))

# ── 1. Legacy product-owned patterns (case-insensitive substrings) ──────────
LEGACY_PATTERNS = [
    "zen-gc",
    "zen_gc",
    "zengc",
    "kube-zen",
    "kube_zen",
    "kubezen",
    "kubzen",
    "garbagecollectionpolicy",
    "garbagecollectionpolicies",
    "gc.ops.zen-mesh.io",
    "gc-controller",
    "gc-system",
    "gc-interval",
    "gc_batch_size",
    "gc_" + "interval",
    "gc_max_concurrent_evaluations",
    "gc_max_deletions_per_second",
    "gc_" + "evaluation_duration",
    "gc_" + "deletion_duration",
    "gc_" + "leader_election",
    "github.com/zenmesh/zen-gc",
    "zenmesh/zen-gc-controller",
]

# Narrowly classified exceptions: (path substring, allowed pattern).
# The guard itself must contain legacy needles to detect them — scanning its
# own source is a narrowly classified self-scan exception.
ALLOWLIST = [
    ("scripts/validation/identity_guard.py", "*"),
]

# ── 2. Canonical new identity assertions ────────────────────────────────────
NEW_ID = [
    ("go.mod", "module github.com/zenmesh/zen-cleaner"),
    ("deploy/crds/cleaner.zen-mesh.io_zencleanerpolicies.yaml", "group: cleaner.zen-mesh.io"),
    ("deploy/crds/cleaner.zen-mesh.io_zencleanerpolicies.yaml", "kind: ZenCleanerPolicy"),
    ("deploy/crds/cleaner.zen-mesh.io_zencleanerpolicies.yaml", "plural: zencleanerpolicies"),
    ("deploy/helm/zen-cleaner/Chart.yaml", "name: zen-cleaner"),
    ("deploy/helm/zen-cleaner/values.yaml", "repository: ghcr.io/zenmesh/zen-cleaner"),
    ("deploy/manifests/namespace.yaml", "name: zen-cleaner-system"),
    ("cmd/zen-cleaner/main.go", "zen-cleaner"),
]

FORBIDDEN_CLAIMS = [
    "enterprise-ready",
    "production-live and",
    "production-grade and certified",
    "zero-risk",
    "guaranteed safe deletion",
    "commercially launched",
    "officially launched",
]

LLMS_REQUIRED = [
    "zen-cleaner",
    "ZenCleanerPolicy",
    "cleaner.zen-mesh.io",
    "github.com/zenmesh/zen-cleaner",
]

README_REQUIRED = [
    "Zen Cleaner",
    "ZenCleanerPolicy",
    "cleaner.zen-mesh.io",
    "deploy/helm/zen-cleaner",
]


def tracked_files():
    out = subprocess.run(
        ["git", "ls-files", "-z"], cwd=REPO, capture_output=True, check=True
    ).stdout
    return [f for f in out.decode("utf-8", "replace").split("\0") if f]


def read(path):
    with open(os.path.join(REPO, path), encoding="utf-8", errors="surrogateescape") as fh:
        return fh.read()


def main() -> int:
    failures = []

    files = tracked_files()
    if os.environ.get("IDENTITY_GUARD_COMMITTED") == "1":
        out = subprocess.run(
            ["git", "ls-tree", "-r", "--name-only", "HEAD", "-z"],
            cwd=REPO, capture_output=True, check=True,
        ).stdout
        files = [f for f in out.decode("utf-8", "replace").split("\0") if f]

    legacy_hits = []
    for path in files:
        try:
            content = read(path).lower()
        except (FileNotFoundError, IsADirectoryError):
            continue
        for pat in LEGACY_PATTERNS:
            if pat in content:
                if any(exc_path in path for exc_path, _ in ALLOWLIST):
                    continue
                legacy_hits.append((path, pat))
    if legacy_hits:
        for path, pat in sorted(set(legacy_hits)):
            failures.append(f"legacy identity '{pat}' found in {path}")

    for path, needle in NEW_ID:
        try:
            if needle not in read(path):
                failures.append(f"canonical identity missing: {needle!r} not in {path}")
        except (FileNotFoundError, IsADirectoryError):
            failures.append(f"canonical identity file missing: {path}")

    try:
        llms = read("llms.txt").lower()
        for needle in LLMS_REQUIRED:
            if needle.lower() not in llms:
                failures.append(f"llms.txt missing canonical token: {needle}")
        for claim in FORBIDDEN_CLAIMS:
            if claim.lower() in llms:
                failures.append(f"llms.txt makes forbidden claim: {claim!r}")
    except FileNotFoundError:
        failures.append("llms.txt missing (it is a maintained discovery surface for this repo)")

    try:
        readme = read("README.md").lower()
        for needle in README_REQUIRED:
            if needle.lower() not in readme:
                failures.append(f"README missing canonical token: {needle}")
        for claim in FORBIDDEN_CLAIMS:
            if claim.lower() in readme:
                failures.append(f"README makes forbidden claim: {claim!r}")
    except FileNotFoundError:
        failures.append("README.md missing")

    if failures:
        print("IDENTITY GUARD: FAIL")
        for f in failures:
            print(f"  - {f}")
        return 1
    print(f"IDENTITY GUARD: PASS ({len(files)} tracked files scanned; "
          f"{len(LEGACY_PATTERNS)} legacy patterns; "
          f"{len(NEW_ID) + len(LLMS_REQUIRED) + len(README_REQUIRED)} identity assertions)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
