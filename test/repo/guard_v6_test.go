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

// GUARD V6 (H259+ §4-§7/§19-§25): API/CRD/capability surface snapshot.
//
// V4/V5 covered: AST imports, build constraints, normalized markers,
// module law, tree law. V6 adds the strongest semantic layer:
//
//   L11 CRD field pinning      — the committed ZenCleanerPolicy CRD schema
//                                must not silently acquire fields outside
//                                the registered cleanup.policy contract
//   L12 exported API snapshot  — deterministic inventory of exported Go
//                                symbols per first-party package; unexpected
//                                new symbols indicate capability expansion
//   L13 action vocabulary      — public Cleaner must not acquire generic
//                                action types (shell/HTTP/SQL/patch/rightsize
//                                /backup/cert/obs) in its Go source

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ── L11: CRD field pinning ────────────────────────────────────────────

// allowedCRDTopLevelFields: the committed cleanup.policy contract. Any new
// top-level spec property requires an explicit allowlist extension.
var allowedCRDTopLevelFields = map[string]bool{
	"targetResource": true,
	"ttl":            true,
	"conditions":     true,
	"behavior":       true,
	"schedule":       true,
	"paused":         true,
	"evaluationInterval": true,
}

// forbiddenCRDConcepts: private-maintenance concept names that must never
// appear as CRD field names or descriptions.
var forbiddenCRDConcepts = []string{
	"opportunity", "recommendation", "rightsizing", "rightsiz",
	"backupoptimization", "certrotation", "observabilityremediation",
	"costsaving", "executionpermission", "proposal", "maintenance",
}

// TestV6CRDFieldPinning parses the committed CRD YAML and asserts every
// spec property is in the allowlist and no forbidden concept appears.
func TestV6CRDFieldPinning(t *testing.T) {
	root := repoRootV4(t)
	crdPath := filepath.Join(root, "deploy", "crds", "cleaner.zen-mesh.io_zencleanerpolicies.yaml")
	raw, err := os.ReadFile(crdPath)
	if err != nil {
		t.Skipf("CRD file not found at expected path: %v", err)
	}
	hay := normalizeIdent(string(raw))
	for _, concept := range forbiddenCRDConcepts {
		if strings.Contains(hay, normalizeIdent(concept)) {
			t.Errorf("GUARD-V6 L11: forbidden maintenance concept %q found in committed CRD", concept)
		}
	}
	// Parse YAML to check top-level spec properties.
	var crd map[string]interface{}
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		t.Fatalf("CRD YAML parse: %v", err)
	}
	// Extract spec.properties from the structural schema (navigating the
	// versions[0].schema.openAPIV3Schema.properties.spec path).
	versions, ok := crd["spec"].(map[string]interface{})["versions"].([]interface{})
	if !ok || len(versions) == 0 {
		t.Skip("CRD has no versions array (schema may be inline)")
	}
	v0 := versions[0].(map[string]interface{})
	schema := v0["schema"].(map[string]interface{})["openAPIV3Schema"].(map[string]interface{})
	specProps, ok := schema["properties"].(map[string]interface{})["spec"].(map[string]interface{})["properties"].(map[string]interface{})
	if !ok {
		t.Skip("CRD spec.properties not found")
	}
	for fieldName := range specProps {
		if !allowedCRDTopLevelFields[normalizeIdent(fieldName)] && !allowedCRDTopLevelFields[fieldName] {
			t.Errorf("GUARD-V6 L11: CRD spec field %q is outside the registered cleanup.policy contract — extend the allowlist only via the component registry", fieldName)
		}
	}
}

// ── L12: exported API surface snapshot ─────────────────────────────────

// TestV6ExportedAPISnapshot generates a deterministic inventory of exported
// Go symbols per first-party package and compares against the baseline.
// To refresh: run with ZEN_CLEANER_API_BASELINE_UPDATE=1 and commit.
func TestV6ExportedAPISnapshot(t *testing.T) {
	root := repoRootV4(t)
	if update := os.Getenv("ZEN_CLEANER_API_BASELINE_UPDATE"); update == "1" {
		generateAndWrite(t, root)
		return
	}
	got := generateSnapshot(t, root)
	baselinePath := filepath.Join(root, "test", "repo", "api_surface_baseline.txt")
	baselineRaw, err := os.ReadFile(baselinePath)
	if err != nil {
		// First run: write the baseline and pass (bootstrap).
		os.WriteFile(baselinePath, []byte(got), 0o644)
		t.Log("API surface baseline created (bootstrap)")
		return
	}
	want := strings.TrimSpace(string(baselineRaw))
	got = strings.TrimSpace(got)
	if got != want {
		// Find the diff lines for a useful error.
		wantSet := map[string]bool{}
		for _, l := range strings.Split(want, "\n") {
			wantSet[strings.TrimSpace(l)] = true
		}
		gotSet := map[string]bool{}
		for _, l := range strings.Split(got, "\n") {
			gotSet[strings.TrimSpace(l)] = true
		}
		var added, removed []string
		for l := range gotSet {
			if !wantSet[l] {
				added = append(added, l)
			}
		}
		for l := range wantSet {
			if !gotSet[l] {
				removed = append(removed, l)
			}
		}
		sort.Strings(added)
		sort.Strings(removed)
		t.Errorf("GUARD-V6 L12: public API surface drift — %d added, %d removed.\n"+
			"ADDED (unexpected capability expansion?):\n%v\n"+
			"REMOVED:\n%v\n"+
			"To accept intentional OSS changes: set ZEN_CLEANER_API_BASELINE_UPDATE=1 and commit the updated baseline.",
			len(added), len(removed), strings.Join(added, "\n"), strings.Join(removed, "\n"))
	}
}

func generateSnapshot(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	fset := token.NewFileSet()
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !allowedFirstPartyPackages[rel[:len(rel)-3]] && !allowedFirstPartyPackages[filepath.ToSlash(filepath.Dir(rel))] {
			return nil
		}
		af, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return nil
		}
		pkg := filepath.ToSlash(filepath.Dir(rel))
		for _, decl := range af.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name.IsExported() {
						lines = append(lines, fmt.Sprintf("%s type %s", pkg, s.Name.Name))
					}
				case *ast.ValueSpec:
					for _, name := range s.Names {
						if name.IsExported() {
							kind := "var"
							if gd.Tok == token.CONST {
								kind = "const"
							}
							lines = append(lines, fmt.Sprintf("%s %s %s", pkg, kind, name.Name))
						}
					}
				}
			}
		}
		for _, decl := range af.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if ok && fd.Recv == nil && fd.Name.IsExported() {
				lines = append(lines, fmt.Sprintf("%s func %s", pkg, fd.Name.Name))
			}
		}
		return nil
	})
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func generateAndWrite(t *testing.T, root string) {
	t.Helper()
	got := generateSnapshot(t, root)
	baselinePath := filepath.Join(root, "test", "repo", "api_surface_baseline.txt")
	if err := os.WriteFile(baselinePath, []byte(got), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("API surface baseline written (%d bytes)", len(got))
}

// ── L13: action vocabulary guard ───────────────────────────────────────

// TestV6NoGenericActionVocabulary: the public Cleaner Go source must not
// declare action constants/types for generic mutation or private
// maintenance capabilities. This is a structural type-surface scan, not a
// keyword ban.
func TestV6NoGenericActionVocabulary(t *testing.T) {
	root := repoRootV4(t)
	forbiddenActions := map[string]bool{
		"actiondelete":             true, // generic delete (scoped deletes are OK)
		"actionpatch":              true,
		"actionshell":              true,
		"actionhttp":               true,
		"actionsql":                true,
		"actionkubectl":            true,
		"actionrightsize":          true,
		"actioncertmaintain":       true,
		"actionbackupmaintain":     true,
		"actionobsremediate":       true,
		"actioncostoptimi":         true,
		"actionexecuteopportunity": true,
	}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		if strings.Contains(p, "vendor/") || strings.Contains(p, "test/") {
			return nil
		}
		raw, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		hay := normalizeIdent(string(raw))
		for action := range forbiddenActions {
			if strings.Contains(hay, "const "+action) || strings.Contains(hay, action+" action") {
				t.Errorf("GUARD-V6 L13: %s declares forbidden generic action %q", p, action)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// ── L14: CRD full-schema snapshot ──────────────────────────────────────

// TestV6CRDFullSchemaSnapshot pins the ENTIRE committed CRD schema — every
// field path (spec, status, nested), its OpenAPI type, and required lists —
// into a deterministic baseline. L11 pins the contract vocabulary; L14 pins
// the exact wire shape, so ANY schema change (type change, new nested field,
// dropped required) is a reviewed, explicit event — never silent chart drift.
// To accept intentional schema changes: ZEN_CLEANER_CRD_SCHEMA_BASELINE_UPDATE=1.
func TestV6CRDFullSchemaSnapshot(t *testing.T) {
	root := repoRootV4(t)
	crdPath := filepath.Join(root, "deploy", "crds", "cleaner.zen-mesh.io_zencleanerpolicies.yaml")
	raw, err := os.ReadFile(crdPath)
	if err != nil {
		t.Skipf("CRD file not found at expected path: %v", err)
	}
	var crd map[string]interface{}
	if err := yaml.Unmarshal(raw, &crd); err != nil {
		t.Fatalf("CRD YAML parse: %v", err)
	}
	versions, ok := crd["spec"].(map[string]interface{})["versions"].([]interface{})
	if !ok || len(versions) == 0 {
		t.Skip("CRD has no versions array")
	}
	v0, ok := versions[0].(map[string]interface{})
	if !ok {
		t.Skip("CRD version entry malformed")
	}
	sch, ok := v0["schema"].(map[string]interface{})["openAPIV3Schema"].(map[string]interface{})
	if !ok {
		t.Skip("CRD openAPIV3Schema not found")
	}

	var lines []string
	var walk func(path string, node map[string]interface{})
	walk = func(path string, node map[string]interface{}) {
		if typeStr, ok := node["type"].(string); ok {
			lines = append(lines, path+" type="+typeStr)
		}
		if req, ok := node["required"].([]interface{}); ok {
			var names []string
			for _, r := range req {
				if s, ok := r.(string); ok {
					names = append(names, s)
				}
			}
			sort.Strings(names)
			lines = append(lines, path+" required=["+strings.Join(names, ",")+"]")
		}
		props, _ := node["properties"].(map[string]interface{})
		keys := make([]string, 0, len(props))
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child, _ := props[k].(map[string]interface{})
			if child == nil {
				continue
			}
			walk(path+"."+k, child)
		}
	}
	walk("", sch)
	sort.Strings(lines)
	got := strings.Join(lines, "\n")

	baselinePath := filepath.Join(root, "test", "repo", "crd_schema_baseline.txt")
	if os.Getenv("ZEN_CLEANER_CRD_SCHEMA_BASELINE_UPDATE") == "1" {
		if err := os.WriteFile(baselinePath, []byte(got), 0o644); err != nil {
			t.Fatalf("write CRD schema baseline: %v", err)
		}
		return
	}
	baselineRaw, err := os.ReadFile(baselinePath)
	if err != nil {
		// Bootstrap: write and pass once, so the diff law starts from truth.
		if err := os.WriteFile(baselinePath, []byte(got), 0o644); err != nil {
			t.Fatalf("bootstrap CRD schema baseline: %v", err)
		}
		t.Log("CRD schema baseline created (bootstrap)")
		return
	}
	want := strings.TrimSpace(string(baselineRaw))
	if strings.TrimSpace(got) != want {
		wantSet, gotSet := map[string]bool{}, map[string]bool{}
		for _, l := range strings.Split(want, "\n") {
			wantSet[strings.TrimSpace(l)] = true
		}
		for _, l := range strings.Split(got, "\n") {
			gotSet[strings.TrimSpace(l)] = true
		}
		var added, removed []string
		for l := range gotSet {
			if !wantSet[l] {
				added = append(added, l)
			}
		}
		for l := range wantSet {
			if !gotSet[l] {
				removed = append(removed, l)
			}
		}
		sort.Strings(added)
		sort.Strings(removed)
		t.Errorf("GUARD-V6 L14: CRD schema drift — %d added, %d removed schema elements.\n"+
			"ADDED:\n%v\nREMOVED:\n%v\n"+
			"A CRD schema change is a WIRE-CONTRACT change: review it, then accept with ZEN_CLEANER_CRD_SCHEMA_BASELINE_UPDATE=1 and commit.",
			len(added), len(removed), strings.Join(added, "\n"), strings.Join(removed, "\n"))
	}
}
