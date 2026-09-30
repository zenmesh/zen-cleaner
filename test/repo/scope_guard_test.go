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

// allowedInternalPackages is the declared OSS scope of this repository.
// Extend ONLY when a capability is registered to this component's public
// contract (zen-mgmt component registry: capability cleanup.policy).
var allowedInternalPackages = map[string]bool{
	"internal/backoff":     true,
	"internal/config":      true,
	"internal/election":    true,
	"internal/errors":      true,
	"internal/events":      true,
	"internal/health":      true,
	"internal/logging":     true,
	"internal/ratelimiter": true,
	"internal/ttl":         true,
	"internal/maintenance": false, // reserved: never public (maintenance product is a separate private component)
}

func TestPublicScopePackageAllowlist(t *testing.T) {
	root := repoRoot(t)
	var found []string
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		found = append(found, filepath.ToSlash(filepath.Dir(p)))
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("walk internal: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range found {
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			t.Fatalf("rel %s: %v", p, rerr)
		}
		seen[filepath.ToSlash(rel)] = true
	}
	var outOfScope []string
	for p := range seen {
		if !allowedInternalPackages[p] {
			outOfScope = append(outOfScope, p)
		}
	}
	if len(outOfScope) > 0 {
		sort.Strings(outOfScope)
		t.Fatalf("out-of-scope internal packages present in the PUBLIC tree: %v — extend the allowlist only via the component registry (capability: cleanup.policy)", outOfScope)
	}
	for p, allowed := range allowedInternalPackages {
		if allowed {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err != nil {
				t.Errorf("registered in-scope package missing: %s", p)
			}
		}
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
	// Adjudicated non-self exceptions (each must stay an explicit, documented
	// decision — never a wildcard):
	allowedZenmeshImports := map[string]bool{
		"github.com/zenmesh/zen-sdk": true, // pre-existing logging-facade dependency (flagged to governance for de-coupling assessment)
	}
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
