/*
Copyright 2026 Zen Mesh

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package repo

// Public-scope governance guard: zen-cleaner is the portfolio's single
// intentional OSS exception, registered as a declarative Kubernetes cleanup
// controller. This test makes it mechanically hard for out-of-scope
// capability packages to re-enter the public tree:
//
//	V2 — the set of first-party Go packages must equal the declared
//	allowlist (package-level, tied to the registered component scope).
//	V3 — first-party packages must not import (or go.mod-require) any
//	portfolio-private module, and no symbol from the preserved
//	out-of-scope extraction may appear in ANY file of the tree (any
//	file type, including generated, build-tagged, test-only, docs,
//	embedded YAML and CRD field names).
//
// The allowlist is deliberately a PACKAGE-LEVEL allowlist tied to the
// registered component scope — not a keyword blacklist. The V3 content
// layer covers what a package allowlist cannot see: capability smuggled
// INSIDE an already-allowed package or inside non-Go artifacts.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// repoRoot resolves the module root (tests run in the package directory).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above test package")
		}
		dir = parent
	}
}

// allowedFirstPartyPackages is the FULL first-party allowlist (H258 §3 V2:
// covers every Go package directory in the module — internal/, pkg/, cmd/,
// observability/, root — so private capability cannot smuggle in via a
// non-internal path, a new sibling package, or a build-tagged file in an
// otherwise-allowed directory).
var allowedFirstPartyPackages = map[string]bool{
	"cmd/validate-examples":  true,
	"cmd/zen-cleaner":        true,
	"internal/backoff":       true,
	"internal/config":        true,
	"internal/election":      true,
	"internal/errors":        true,
	"internal/events":        true,
	"internal/health":        true,
	"internal/logging":       true,
	"internal/ratelimiter":   true,
	"internal/ttl":           true,
	"observability":          true,
	"pkg/api/v1alpha1":       true,
	"pkg/config":             true,
	"pkg/controller":         true,
	"pkg/controller/testing": true,
	"pkg/errors":             true,
	"pkg/safety":             true,
	"pkg/validation":         true,
	"pkg/webhook":            true,
	"test/integration":       true,
	"test/repo":              true,
	"test/e2e":               true,
	"deploy":                 true, // deploy-time Go helpers (smoke/parity checks)
	"deploy/manifests":       true,
}

func TestPublicScopePackageAllowlist(t *testing.T) {
	root := repoRoot(t)
	var found []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == ".git" || name == "bin" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		found = append(found, filepath.ToSlash(filepath.Dir(rel)))
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("walk internal: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range found {
		seen[p] = true // V2 walk already yields module-relative dirs
	}
	var outOfScope []string
	for p := range seen {
		if !allowedFirstPartyPackages[p] {
			outOfScope = append(outOfScope, p)
		}
	}
	if len(outOfScope) > 0 {
		sort.Strings(outOfScope)
		t.Fatalf("out-of-scope first-party packages present in the PUBLIC tree: %v — extend the allowlist only via the component registry (capability: cleanup.policy)", outOfScope)
	}

}

func TestDependencyDirectionNoPrivateImports(t *testing.T) {
	root := repoRoot(t)
	// First-party packages must not import any portfolio-private module.
	// The OSS module builds standalone; any zenmesh module dependency other
	// than this one is a dependency-direction violation.
	// Self is fine. The former zen-sdk logging-facade exception was
	// adjudicated (H258 §5) and REMOVED: go.mod carries zero zenmesh
	// requires and no first-party file imports one — the OSS product is
	// fully standalone. There are no exceptions.
	selfModule := "github.com/zenmesh/zen-cleaner"
	allowedZenmeshImports := map[string]bool{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", ".git", "testbin":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		if strings.HasSuffix(filepath.ToSlash(p), "test/repo/scope_guard_test.go") {
			return nil // this guard's own exception table is not an import
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			// Attack-hardened (H258 §3 battery): extract every quoted
			// zenmesh path wherever it appears on the line — bare import
			// specs, `import _ "..."` / `import x "..."` single-line forms,
			// and dot-imports all smuggled past a prefix-only check.
			if !strings.Contains(line, `"github.com/zenmesh/`) {
				continue
			}
			for _, seg := range strings.Split(line, `"`) {
				if !strings.HasPrefix(seg, "github.com/zenmesh/") {
					continue
				}
				if seg == selfModule || strings.HasPrefix(seg, selfModule+"/") {
					continue // own module packages (exact-boundary: zen-cleaner-evil is NOT self)
				}
				if !allowedZenmeshImports[seg] {
					t.Errorf("%s: imports portfolio-private module %s (public OSS must not depend on private Zen modules)", p, seg)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	// V3: the import-line scan above sees only compiled references; a
	// dormant `require github.com/zenmesh/<private-module>` in go.mod is
	// the same dependency-direction violation one refactor away from
	// activation. go.mod must carry zero non-self zenmesh requires.
	modBytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, line := range strings.Split(string(modBytes), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "github.com/zenmesh/") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == selfModule {
			continue
		}
		t.Errorf("go.mod requires portfolio-private module %s (public OSS must be standalone)", fields[0])
	}
}

// TestNoPrivateExtractionMarkers is the V3 content layer: the package
// allowlist cannot see capability smuggled INSIDE an already-allowed
// package, inside generated/build-tagged/test-only files, or inside
// non-Go artifacts (docs examples, embedded YAML, CRD field names). Every
// marker in private_extraction_markers.txt is a distinctive symbol of the
// preserved out-of-scope extraction, mechanically derived and pruned to
// zero expected hits, so any hit is material scope drift.
func TestNoPrivateExtractionMarkers(t *testing.T) {
	root := repoRoot(t)
	markerPath := filepath.Join(root, "test", "repo", "private_extraction_markers.txt")
	raw, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read marker list: %v", err)
	}
	var markers []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		markers = append(markers, line)
	}
	if len(markers) < 50 {
		t.Fatalf("marker list implausibly small (%d): refuse to run a toothless guard", len(markers))
	}
	var hits []string
	walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "bin", "dist", "testbin":
				return filepath.SkipDir
			}
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		if filepath.ToSlash(rel) == "test/repo/private_extraction_markers.txt" {
			return nil // the denylist itself is the one lawful occurrence
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		content, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil // unreadable (perm/socket) — not a content smuggle
		}
		s := string(content)
		for _, m := range markers {
			if strings.Contains(s, m) {
				hits = append(hits, fmt.Sprintf("%s contains %q", filepath.ToSlash(rel), m))
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	if len(hits) > 0 {
		sort.Strings(hits)
		t.Fatalf("out-of-scope extraction symbols present in the PUBLIC tree (material scope drift — extend scope only via the component registry):\n%s", strings.Join(hits, "\n"))
	}
}
