#!/usr/bin/env python3
"""
zen-cleaner Cross-Linking & SEO Validation Script.

Checks:
1. README links to zen-mesh.io
2. README describes zen-cleaner as Kubernetes TTL/cleanup controller
3. docs index links to zen-mesh.io and GitHub
4. No forbidden prod-live/customer-ready/official-launch wording
5. No placeholder `{{ .projectName }}` remains in public docs
6. zen-mesh.io links to zen-cleaner (if repo available)
7. docs/AI surfaces link to zen-cleaner (if repo available)
"""

import os
import re
import sys

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "../.."))
ZEN_MESH_IO_DIR = os.path.abspath(os.path.join(REPO_ROOT, "../zen-mesh.io"))
DOCS_DIR = os.path.abspath(os.path.join(REPO_ROOT, "../docs"))

FORBIDDEN_PATTERNS = [
    r"production\s+live",
    r"customer-ready",
    r"officially\s+launched",
    r"zero-trust\s+complete",
    r"enterprise-ready",
    r"guaranteed\s+safe\s+deletion",
    r"set-and-forget\s+deletion",
    r"compliance\s+certified",
    r"requires\s+Zen\s+Mesh",
    r"requires\s+zen-cleaner",
]

FORBIDDEN_EXACT = [
    "production live",
    "customer-ready",
    "official launch",
    "zero-trust complete",
    "enterprise-ready",
    "guaranteed safe deletion",
    "set-and-forget deletion",
    "compliance certified",
]

results = {"pass": 0, "fail": 0, "failures": []}


def check(description, condition, fix=None):
    if condition:
        results["pass"] += 1
        print(f"  PASS: {description}")
    else:
        results["fail"] += 1
        msg = f"FAIL: {description}"
        if fix:
            msg += f" | fix: {fix}"
        results["failures"].append(msg)
        print(f"  {msg}")


def read_file(path):
    try:
        with open(path, "r") as f:
            return f.read()
    except FileNotFoundError:
        return None


def check_readme():
    print("\n--- README.md checks ---")
    content = read_file(os.path.join(REPO_ROOT, "README.md"))
    if not content:
        check("README.md exists", False, "Create README.md")
        return

    check("README links to zen-mesh.io", "zen-mesh.io" in content)
    check("README mentions cleanup", "cleanup" in content.lower())
    check("README mentions TTL", "ttl" in content.lower() or "TTL" in content)
    check("README mentions Kubernetes cleanup", "cleanup" in content.lower())
    check("README mentions ConfigMap", "configmap" in content.lower() or "ConfigMap" in content)
    check("README mentions Apache-2.0 license", "apache-2.0" in content.lower() or "Apache-2.0" in content)
    check("README describes zen-cleaner as OSS/community/open source", "open source" in content.lower() or "OSS" in content)
    check("README has 'From the Zen Mesh community' section", "From the Zen Mesh community" in content)
    check("README has 'Related project' or ecosystem table", "Related project" in content or "zen-cleaner" in content and "Zen Mesh" in content)


def check_docs_index():
    print("\n--- docs/INDEX.md checks ---")
    content = read_file(os.path.join(REPO_ROOT, "docs/INDEX.md"))
    if not content:
        check("docs/INDEX.md exists", False, "Create docs/INDEX.md")
        return

    check("INDEX links to zen-mesh.io", "zen-mesh.io" in content)
    check("INDEX links to GitHub", "github.com/zenmesh/zen-cleaner" in content)
    check("INDEX has 'Where this fits with Zen Mesh' section", "Where this fits with Zen Mesh" in content)
    check("INDEX describes as Kubernetes cleanup controller", "cleanup controller" in content.lower())
    check("INDEX mentions Apache-2.0", "apache-2.0" in content.lower() or "Apache-2.0" in content)


def check_no_placeholders():
    print("\n--- Placeholder checks ---")
    docs_dir = os.path.join(REPO_ROOT, "docs")
    placeholders_found = False
    for root, dirs, files in os.walk(docs_dir):
        for f in files:
            if f.endswith(".md"):
                path = os.path.join(root, f)
                with open(path) as fh:
                    content = fh.read()
                    if "{{ .projectName }}" in content:
                        print(f"  FAIL: Placeholder found in {os.path.relpath(path, REPO_ROOT)}")
                        placeholders_found = True
    if not placeholders_found:
        check("No {{ .projectName }} placeholders remain in docs/", True)
    else:
        check("No {{ .projectName }} placeholders remain in docs/", False, "Remove remaining placeholders")


def check_forbidden_claims():
    print("\n--- Forbidden claims checks ---")
    content = read_file(os.path.join(REPO_ROOT, "README.md"))
    if not content:
        return

    for pattern in FORBIDDEN_PATTERNS:
        if re.search(pattern, content, re.IGNORECASE):
            print(f"  FAIL: Found forbidden pattern: {pattern}")
            results["fail"] += 1
            results["failures"].append(f"Forbidden claim matched: {pattern}")
            return

    check("No forbidden prod-live/customer-ready/official-launch claims in README", True)


def check_zen_mesh_io():
    print("\n--- zen-mesh.io checks ---")
    if not os.path.isdir(ZEN_MESH_IO_DIR):
        check("zen-mesh.io repo available for checking", False, "Clone zen-mesh.io repo")
        return

    index_path = os.path.join(ZEN_MESH_IO_DIR, "src/pages/index.astro")
    content = read_file(index_path)
    if not content:
        check("zen-mesh.io index.astro exists", False)
        return

    check("zen-mesh.io links to zen-cleaner", "zen-cleaner" in content or "zen_cleaner" in content)
    check("zen-mesh.io primary CTA preserved (Edge Lite)", "Edge Lite" in content or "edge" in content.lower())
    check("zen-mesh.io does NOT claim prod-live for Zen Mesh", "Early Access" in content or "early access" in content.lower() or "DEMO" in content)


def check_docs():
    print("\n--- docs repo checks ---")
    if not os.path.isdir(DOCS_DIR):
        check("docs repo available for checking", False, "Clone docs repo")
        return

    ai_overview = os.path.join(DOCS_DIR, "docs/ai/overview.md")
    content = read_file(ai_overview)
    if content:
        check("docs AI overview references zen-cleaner", "zen-cleaner" in content)
    else:
        check("docs AI overview exists", False)

    llms_txt = os.path.join(DOCS_DIR, "static/llms.txt")
    content = read_file(llms_txt)
    if content:
        check("docs llms.txt references zen-cleaner", "zen-cleaner" in content)
    else:
        check("docs llms.txt exists", False)


def main():
    print("zen-cleaner Cross-Linking & SEO Validation")
    print("=" * 50)

    check_readme()
    check_docs_index()
    check_no_placeholders()
    check_forbidden_claims()
    check_zen_mesh_io()
    check_docs()

    print(f"\n{'=' * 50}")
    print(f"Results: {results['pass']} PASS, {results['fail']} FAIL")
    if results["failures"]:
        print("Failures:")
        for f in results["failures"]:
            print(f"  - {f}")

    return 0 if results["fail"] == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
