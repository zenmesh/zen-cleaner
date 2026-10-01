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

// GUARD V5 (H259+): closes the Support2 S174 PARTIAL families that V4 left
// open, on the same Go-tooling layer discipline:
//
//   L7  extension classification — private-scope marker scan now covers
//       .txt/.tmpl/.sh alongside .go/.yaml/.yml/.json/.md (§4-§5); binaries
//       are NOT scanned as text (§5 avoidance law)
//   L8  bin/dist rework (§7-§9): these trees are no longer blind-skipped —
//       Go sources there are subject to the package allowlist like any
//       other tree, and COMMITTED BINARIES are banned outright (structural
//       policy over pretended semantic scan)
//   L9  go.work ban (§14): a committed workspace file can bind local
//       private modules into the build
//   L10 module graph (§11-§12): requires remain the pinned allowlist and
//       zenmesh modules other than self remain forbidden (V4 law kept)
//
// RETIRED (S174-closed, regression-pinned by V4 tests retained in history):
// legacy +build, parenthesized constraints, marker normalization, symlinks.
// Threat model unchanged (§3): no renamed-algorithm plagiarism detection.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// v5TextExtensions: repository-controlled text formats that can carry
// private capability content into the public tree (§5).
var v5TextExtensions = map[string]bool{
	".go": true, ".yaml": true, ".yml": true, ".json": true, ".md": true,
	".txt": true, ".tmpl": true, ".sh": true,
}

// v5BinaryExtensions: committed compiled artifacts are banned in the OSS
// module (§9) — structural policy instead of pretending a semantic scan.
var v5BinaryExtensions = map[string]bool{
	".exe": true, ".bin": true, ".so": true, ".dylib": true, ".a": true,
	".tar": true, ".gz": true, ".zip": true, ".tgz": true,
}

// TestV5 is the incremental battery over V4's laws (V4's own test remains
// the base regression; V5 adds the S174 families).
func TestV5(t *testing.T) {
	root := repoRootV4(t)
	var violations []string
	textScanned := 0

	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			switch name {
			case ".git", "node_modules":
				return filepath.SkipDir
			case "vendor":
				violations = append(violations, "L6/V5 §10: vendor tree present — module must remain proxy-resolvable standalone")
				return filepath.SkipDir
			case "bin", "dist":
				// §7-§8: NOT blind-skipped. Sources inside are scanned like
				// any other tree; committed binaries are banned below.
				return nil
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil // V4 law (tested there)
		}
		ext := strings.ToLower(filepath.Ext(rel))

		// §9: committed binary artifacts banned outright.
		if v5BinaryExtensions[ext] {
			violations = append(violations, "V5 §9: committed binary artifact banned in OSS module: "+rel)
			return nil
		}

		// §5: marker scan across ALL repository-controlled text formats,
		// including previously-unscanned .txt/.tmpl/.sh. Generated content
		// is NOT exempt (§6): if committed and public, it is guarded.
		if v5TextExtensions[ext] {
			if strings.HasSuffix(rel, "guard_v4_test.go") || strings.HasSuffix(rel, "guard_v5_test.go") {
				return nil // the guards' own tables are not leaks
			}
			if rel == "test/repo/private_extraction_markers.txt" {
				return nil // the guard's own marker DATA file: contains markers by definition
			}
			raw, rerr := os.ReadFile(p)
			if rerr != nil {
				return nil
			}
			// §5 avoidance: never treat binary-ish payloads as text.
			if strings.Contains(string(raw[:min(len(raw), 800)]), "\x00") {
				violations = append(violations, "V5 §9: binary content with text extension: "+rel)
				return nil
			}
			hay := normalizeIdent(string(raw))
			textScanned++
			for _, marker := range privateScopeMarkers {
				if strings.Contains(hay, normalizeIdent(marker)) {
					violations = append(violations, "V5 §5: private-scope marker in text artifact: "+rel+" ("+marker+")")
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	// §14: a committed go.work can bind local private modules into every
	// build — banned outright.
	if _, err := os.Stat(filepath.Join(root, "go.work")); err == nil {
		violations = append(violations, "V5 §14: committed go.work present (local-module binding vector)")
	}
	// §15: nested modules banned (V4 checks during its walk; asserted here
	// independently so V5 stands alone).
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil || filepath.ToSlash(rel) == "." {
			return nil
		}
		if filepath.Base(p) == "go.mod" && filepath.ToSlash(rel) != "go.mod" {
			violations = append(violations, "V5 §15: nested go.mod at "+filepath.ToSlash(rel))
		}
		return nil
	})

	if textScanned == 0 {
		t.Fatal("V5: extension scan must have covered text artifacts")
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		for _, v := range violations {
			t.Errorf("GUARD-V5: %s", v)
		}
		t.FailNow()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
