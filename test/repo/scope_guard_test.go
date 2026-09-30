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
	var found []string
	err := filepath.WalkDir("internal", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		dir := filepath.ToSlash(filepath.Dir(p))
		found = append(found, dir)
		return filepath.SkipAll
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("walk internal: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range found {
		seen[p] = true
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
			if _, err := os.Stat(filepath.FromSlash(p)); err != nil {
				t.Errorf("registered in-scope package missing: %s", p)
			}
		}
	}
}

func TestDependencyDirectionNoPrivateImports(t *testing.T) {
	// First-party packages must not import any portfolio-private module.
	// The OSS module builds standalone; any zenmesh module dependency other
	// than this one is a dependency-direction violation.
	allowedZenmeshImports := map[string]bool{
		"github.com/zenmesh/zen-cleaner": true, // self
	}
	err := filepath.WalkDir(".", func(p string, d os.DirEntry, err error) error {
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
