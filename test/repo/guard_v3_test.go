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

// GUARD V3 (H265+ Programs E/F/G): the ATTACK battery + BUILD-GRAPH
// firewall + BINARY semantic firewall.
//
// Support2's bypass classes are each reproduced on a fixture tree and the
// SHARED judgment (v4Evaluate — same implementation TestV4 runs on the real
// repo) must fail every one:
//
//	symlink directory        -> L6
//	build-tag-only source    -> L2 (tags never exclude the AST scan) + L3
//	plain-content smuggling  -> L4 (any tracked text class)
//	vendor/ tree             -> L6
//	private indirect import  -> L2 (not self/pinned/stdlib)
//	nested module / replace  -> L5
//	generated code           -> L2 (no generated-file exemption)
//	go:embed smuggling       -> the V3 embed-content law (below)
//
// The BUILD-GRAPH firewall (F) resolves the module's complete dependency
// graph with GO'S OWN resolution (`go list -deps -test ./...` — not grep)
// and refuses any dependency that is not self, a pinned module, or the
// standard library; additionally no zenmesh path other than zen-cleaner and
// the pinned zen-sdk may appear anywhere in the graph.
//
// The BINARY semantic firewall (G) builds the real server binary and proves
// no private-scope marker and no private module name survives into the
// linked artifact — the compile-level form of "no private capability,
// including through renamed symbols": if the module graph cannot reach the
// private code, no symbol rename can introduce it.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

// writeAttackFixture materializes a module-shaped fixture tree.
func writeAttackFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	gomod := "module github.com/zenmesh/zen-cleaner\n\ngo 1.27.0\n"
	if v, ok := files["go.mod"]; ok {
		gomod = v
		delete(files, "go.mod")
	}
	files["go.mod"] = gomod
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// violationsWith returns the violations carrying any of the given prefixes.
func violationsWith(all []string, prefixes ...string) []string {
	var out []string
	for _, v := range all {
		for _, p := range prefixes {
			if strings.HasPrefix(v, p) {
				out = append(out, v)
				break
			}
		}
	}
	return out
}

func assertFires(t *testing.T, name string, all []string, prefixes ...string) {
	t.Helper()
	if got := violationsWith(all, prefixes...); len(got) == 0 {
		t.Fatalf("%s: guard did NOT fire (wanted prefixes %v); got %v", name, prefixes, all)
	}
}

// 1. Symlinked DIRECTORY: the walk must report the link itself.
func TestGuardV3_Attack_SymlinkDir(t *testing.T) {
	root := writeAttackFixture(t, map[string]string{
		"pkg/api/real.go": "package api\n",
	})
	if err := os.Symlink(filepath.Join(root, "pkg", "api"), filepath.Join(root, "pkg", "api-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	assertFires(t, "symlink dir", v4Evaluate(t, root), "L6:")
}

// 2. Build-tag-only source: excluded-at-build files are still scanned.
func TestGuardV3_Attack_BuildTagOnlySource(t *testing.T) {
	root := writeAttackFixture(t, map[string]string{
		"pkg/hidden/hidden.go": "//go:build advancedcleaner\n\npackage hidden\n\nimport _ \"github.com/zenmesh/advanced-cleaner/internal/private\"\n\nvar _ = hiddenConst\n\nconst hiddenConst = 1\n",
	})
	all := v4Evaluate(t, root)
	assertFires(t, "tag-only import", all, "L2:")
	assertFires(t, "tag-only atom", all, "L3:")
}

// 3. Plain-content smuggling: private vocabulary pasted into tracked text.
func TestGuardV3_Attack_PlainContentSmuggling(t *testing.T) {
	root := writeAttackFixture(t, map[string]string{
		// The marker is assembled at runtime so the guard's own source
		// does not carry the literal it forbids.
		"deploy/notes.yaml": "description: mimics Storage" + "LongUnbound detector semantics\n",
	})
	assertFires(t, "content smuggling", v4Evaluate(t, root), "L4:")
}

// 4. vendor/ tree: presence alone is the violation.
func TestGuardV3_Attack_Vendor(t *testing.T) {
	root := writeAttackFixture(t, map[string]string{
		"vendor/example.com/mod/mod.go": "package mod\n",
	})
	assertFires(t, "vendor", v4Evaluate(t, root), "L6:")
}

// 5. Private indirect import: any non-pinned, non-stdlib module path.
func TestGuardV3_Attack_PrivateIndirectImport(t *testing.T) {
	root := writeAttackFixture(t, map[string]string{
		"pkg/api/leak.go": "package api\n\nimport _ \"github.com/zenmesh/advanced-cleaner/pkg/protection\"\n",
	})
	assertFires(t, "private import", v4Evaluate(t, root), "L2:")
}

// 6. Nested module with replace: nested go.mod is the L5 violation.
func TestGuardV3_Attack_NestedModuleReplace(t *testing.T) {
	root := writeAttackFixture(t, map[string]string{
		"tools/sub/go.mod": "module sub\n\ngo 1.27.0\n\nreplace github.com/zenmesh/zen-sdk => /private/zen-sdk\n",
		"tools/sub/m.go":   "package sub\n",
	})
	assertFires(t, "nested module", v4Evaluate(t, root), "L5:")
}

// 7. Generated code: no generated-file exemption from the import law.
func TestGuardV3_Attack_GeneratedCode(t *testing.T) {
	root := writeAttackFixture(t, map[string]string{
		"pkg/api/zz_generated_leak.go": "// Code generated by attacker. DO NOT EDIT.\n\npackage api\n\nimport _ \"github.com/zenmesh/advanced-cleaner/internal/registry\"\n",
	})
	assertFires(t, "generated import", v4Evaluate(t, root), "L2:")
}

// ── V3 embed-content law ────────────────────────────────────────────────

var embedDirectiveRe = regexp.MustCompile(`(?m)^//[ \t]*go:embed[ \t]+(.+)$`)

// embedTargetFiles resolves the //go:embed patterns of every .go file under
// root to concrete files (any extension — embedding is not limited to the
// tracked-text classes).
func embedTargetFiles(t *testing.T, root string) []string {
	t.Helper()
	var targets []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range embedDirectiveRe.FindAllStringSubmatch(string(raw), -1) {
			for _, pat := range strings.Fields(m[1]) {
				pat = strings.Trim(pat, "`\"")
				matches, err := filepath.Glob(filepath.Join(filepath.Dir(p), pat))
				if err != nil {
					continue
				}
				for _, hit := range matches {
					info, err := os.Stat(hit)
					if err != nil {
						continue
					}
					if info.IsDir() {
						filepath.WalkDir(hit, func(sub string, sd os.DirEntry, serr error) error {
							if serr == nil && !sd.IsDir() {
								targets = append(targets, sub)
							}
							return nil
						})
						continue
					}
					targets = append(targets, hit)
				}
			}
		}
		return nil
	})
	return targets
}

// 8. go:embed smuggling: a private marker embedded from a non-tracked-text
// extension (.txt) must fail the embed-content law.
func TestGuardV3_Attack_EmbedSmuggling(t *testing.T) {
	root := writeAttackFixture(t, map[string]string{
		"pkg/api/payload.go":  "package api\n\nimport _ \"embed\"\n\n//go:embed payload.txt\nvar payload string\n",
		"pkg/api/payload.txt": "hidden semantic: Storage" + "LongUnbound\n",
	})
	if got := embedTargetFiles(t, root); len(got) != 1 {
		t.Fatalf("embed resolution: %v", got)
	}
	assertFires(t, "embed smuggling", append(v4Evaluate(t, root), embedLawViolations(t, root)...), "V3-EMBED:")
}

// embedLawViolations marker-scans every embed target regardless of file
// extension (normalized marker match, identical to L4).
func embedLawViolations(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	for _, p := range embedTargetFiles(t, root) {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		hay := normalizeIdent(string(raw))
		rel, _ := filepath.Rel(root, p)
		for _, marker := range privateScopeMarkers {
			if strings.Contains(hay, normalizeIdent(marker)) {
				out = append(out, "V3-EMBED: private-scope marker in embedded file "+filepath.ToSlash(rel))
				break
			}
		}
	}
	return out
}

// TestGuardV3_EmbedContentLaw runs the embed law over the real repo.
func TestGuardV3_EmbedContentLaw(t *testing.T) {
	if v := embedLawViolations(t, repoRootV4(t)); len(v) > 0 {
		for _, x := range v {
			t.Errorf("%s", x)
		}
		t.FailNow()
	}
}

// ── Build-graph firewall (F) ────────────────────────────────────────────

// TestGuardV3_BuildGraphFirewall resolves the complete import graph with
// Go's own module/build resolution and refuses anything outside
// self / pinned modules / stdlib — plus the zenmesh-path allowlist.
func TestGuardV3_BuildGraphFirewall(t *testing.T) {
	root := repoRootV4(t)
	gomodRaw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	mf, err := modfile.Parse("go.mod", gomodRaw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(mf.Replace) > 0 {
		t.Fatal("L5: go.mod contains replace directives")
	}
	pinned := map[string]bool{}
	for _, r := range mf.Require {
		pinned[r.Mod.Path] = true
	}

	cmd := exec.Command("go", "list", "-deps", "-test", "./...")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps -test: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	const (
		self   = "github.com/zenmesh/zen-cleaner"
		zenSDK = "github.com/zenmesh/zen-sdk"
	)
	allowedZenmesh := map[string]bool{self: true, zenSDK: true}
	var violations []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		dep := strings.TrimSpace(line)
		if dep == "" {
			continue
		}
		if isStdlibV4(dep) {
			continue
		}
		if dep == self || strings.HasPrefix(dep, self+"/") {
			continue
		}
		if pinned[dep] || pinnedPrefix(pinned, dep) != "" {
			// zenmesh-path allowlist beyond pinning: every zenmesh module
			// in the graph must be exactly the allowlisted set.
			if strings.HasPrefix(dep, "github.com/zenmesh/") {
				mod := pinnedPrefix(pinned, dep)
				if mod == "" || !allowedZenmesh[mod] {
					violations = append(violations, "V3-GRAPH: zenmesh dependency outside the allowlist: "+dep)
				}
			}
			continue
		}
		violations = append(violations, "V3-GRAPH: dependency not self/pinned/stdlib: "+dep)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		for _, v := range violations {
			t.Errorf("%s", v)
		}
		t.FailNow()
	}
}

// ── Binary semantic firewall (G) ────────────────────────────────────────

// TestGuardV3_BinarySemanticFirewall builds the real controller binary and
// proves no private-scope marker and no private module name survives
// linking. Compile-level form of "no renamed-symbol escape": the module
// graph cannot reach the private code, so no symbol of it exists.
func TestGuardV3_BinarySemanticFirewall(t *testing.T) {
	root := repoRootV4(t)
	probe := filepath.Join(t.TempDir(), "zen-cleaner-firewall-probe")
	build := exec.Command("go", "build", "-o", probe, "./cmd/zen-cleaner")
	build.Dir = root
	var bs bytes.Buffer
	build.Stderr = &bs
	if err := build.Run(); err != nil {
		t.Fatalf("build probe binary: %v: %s", err, strings.TrimSpace(bs.String()))
	}
	raw, err := os.ReadFile(probe)
	if err != nil {
		t.Fatal(err)
	}
	// Normalize the whole image to [a-z0-9] (same normalizer as L4) and
	// require the private vocabulary to be absent.
	var norm strings.Builder
	norm.Grow(len(raw))
	for _, b := range raw {
		if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') {
			norm.WriteByte(b)
			continue
		}
		if b >= 'A' && b <= 'Z' {
			norm.WriteByte(b + 32)
		}
	}
	hay := norm.String()
	for _, marker := range privateScopeMarkers {
		nm := normalizeIdent(marker)
		if nm == "" {
			continue
		}
		if strings.Contains(hay, nm) {
			t.Errorf("GUARD-V3-BINARY: private-scope marker %q survives into the linked binary", marker)
		}
	}
	for _, banned := range []string{"advancedcleaner", "zen" + "maintenance", "advanced-cleaner"} {
		if strings.Contains(hay, normalizeIdent(banned)) {
			t.Errorf("GUARD-V3-BINARY: private module name %q survives into the linked binary", banned)
		}
	}
}
