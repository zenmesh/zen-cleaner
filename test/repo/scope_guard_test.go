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
// capability packages to re-enter the public tree: the set of first-party
// internal packages must equal the declared allowlist, and first-party
// packages must not import anything outside the module plus its declared
// public dependencies (dependency direction: public OSS imports nothing
// portfolio-private).
//
// The allowlist is deliberately a PACKAGE-LEVEL allowlist tied to the
// registered component scope — not a keyword blacklist.

import (
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
	// Self is fine. github.com/zenmesh/zen-sdk is a PRE-EXISTING dependency
	// of the OSS logging facade — recorded here as an explicit, adjudicated
	// exception pending the governance decision on de-coupling it; it is NOT
	// a precedent for new private-module dependencies.
	selfModule := "github.com/zenmesh/zen-cleaner"
	// Adjudication (H258 §5): the module requires NO other zenmesh module
	// (go.mod verified) and imports none — the OSS product is standalone.
	// Any new zenmesh module dependency is a guard violation; there are no
	// exceptions.
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
			if !strings.HasPrefix(line, `"github.com/zenmesh/`) && !strings.Contains(line, `"github.com/zenmesh/`) {
				continue
			}
			if !strings.HasPrefix(line, `"`) {
				continue // not an import spec line
			}
			imp := strings.Trim(line, `"`)
			if strings.HasPrefix(imp, selfModule+"/") || imp == selfModule {
				continue // own module packages
			}
			if !allowedZenmeshImports[imp] {
				t.Errorf("%s: imports portfolio-private module %s (public OSS must not depend on private Zen modules)", p, imp)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
