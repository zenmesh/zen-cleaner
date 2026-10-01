// Copyright 2026 Zen Mesh. All rights reserved.

// GUARD V7 — API COMPAT LAYER (H260 outstanding flag).
//
// V6 L12 pins the exported-symbol NAMES. It cannot see the break that
// actually hits consumers: a signature change under a stable name
// (`NewThing(cfg)` → `NewThing(cfg, log)`), a dropped struct field, a
// changed field type, a removed method. L14 pins the CRD wire shape; V7
// pins the GO WIRE SHAPE.
//
// LAW: for every exported package-level function, method on an exported
// type, and exported type shape (struct fields / interface methods /
// value types), a deterministic canonical string is snapshotted. A baseline
// line that is missing or DIFFERENT today is a compatibility break —
// reported precisely, accepted only via
// ZEN_CLEANER_API_COMPAT_BASELINE_UPDATE=1 and a committed baseline.
// Additions are not violations (L12 governs unexpected additions).
//
// The encoding is source-level (go/parser + types.ExprString): deterministic
// per source tree, no typecheck of foreign deps required.
package repo

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestV7APICompatSnapshot generates and diffs the compat baseline.
func TestV7APICompatSnapshot(t *testing.T) {
	root := repoRootV4(t)
	got := generateCompatSnapshot(t, root)
	baselinePath := filepath.Join(root, "test", "repo", "api_compat_baseline.txt")

	if os.Getenv("ZEN_CLEANER_API_COMPAT_BASELINE_UPDATE") == "1" {
		if err := os.WriteFile(baselinePath, []byte(got), 0o644); err != nil {
			t.Fatalf("write compat baseline: %v", err)
		}
		return
	}
	raw, err := os.ReadFile(baselinePath)
	if err != nil {
		// Bootstrap: first run writes the reference and passes.
		if werr := os.WriteFile(baselinePath, []byte(got), 0o644); werr != nil {
			t.Fatalf("bootstrap compat baseline: %v", werr)
		}
		t.Log("API compat baseline created (bootstrap)")
		return
	}
	want := strings.TrimSpace(string(raw))
	have := strings.TrimSpace(got)
	if want == have {
		return
	}
	wantSet := indexLines(want)
	haveSet := indexLines(have)
	var removed, changed, added []string
	for k := range wantSet {
		if _, ok := haveSet[k]; !ok {
			// Same symbol+shape class vanished — either removed or the
			// signature/shape string changed; classify for the operator.
			if symbolStillExists(haveSet, k) {
				changed = append(changed, k)
			} else {
				removed = append(removed, k)
			}
		}
	}
	for k := range haveSet {
		if _, ok := wantSet[k]; !ok {
			added = append(added, k)
		}
	}
	sort.Strings(removed)
	sort.Strings(changed)
	sort.Strings(added)
	if len(removed) > 0 || len(changed) > 0 {
		t.Errorf("GUARD-V7: API compatibility break — %d removed, %d signature/shape CHANGES.\n"+
			"REMOVED:\n%v\nCHANGED (same symbol, different signature/shape — old line shown; the new line is in the baseline diff):\n%v\n"+
			"(additions, not violations: %d)\n"+
			"To accept an intentional break: set ZEN_CLEANER_API_COMPAT_BASELINE_UPDATE=1 and commit the updated baseline.",
			len(removed), len(changed), strings.Join(removed, "\n"), strings.Join(changed, "\n"), len(added))
	}
}

// indexLines maps baseline lines for set operations.
func indexLines(s string) map[string]bool {
	m := map[string]bool{}
	if s == "" {
		return m
	}
	for _, l := range strings.Split(s, "\n") {
		m[strings.TrimSpace(l)] = true
	}
	return m
}

// symbolStillExists reports whether a baseline line's symbol (its first
// three space-separated fields: pkg kind name) is still exported under the
// same class — used to classify REMOVED vs CHANGED.
func symbolStillExists(have map[string]bool, oldLine string) bool {
	fields := strings.SplitN(oldLine, " ", 4)
	if len(fields) < 4 {
		return false
	}
	prefix := fields[0] + " " + fields[1] + " " + fields[2] + " "
	for k := range have {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// generateCompatSnapshot walks first-party packages and emits one canonical
// line per exported capability, including its signature or shape.
func generateCompatSnapshot(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	fset := token.NewFileSet()
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
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
		emit := func(format string, args ...interface{}) {
			lines = append(lines, fmt.Sprintf(format, args...))
		}
		for _, decl := range af.Decls {
			switch gd := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range gd.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if !s.Name.IsExported() {
							continue
						}
						emit("%s type %s %s", pkg, s.Name.Name, typeShape(s))
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if !name.IsExported() {
								continue
							}
							kind := "var"
							if gd.Tok == token.CONST {
								kind = "const"
							}
							emit("%s %s %s %s", pkg, kind, name.Name, types.ExprString(s.Type))
						}
					}
				}
			case *ast.FuncDecl:
				if !gd.Name.IsExported() {
					continue
				}
				sig := funcSig(gd)
				if gd.Recv == nil {
					emit("%s func %s %s", pkg, gd.Name.Name, sig)
					continue
				}
				recv := receiverTypeName(gd.Recv)
				if recv == "" || !ast.IsExported(recv) {
					continue
				}
				emit("%s method %s.%s %s", pkg, recv, gd.Name.Name, sig)
			}
		}
		return nil
	})
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// funcSig renders the canonical parameter/result type list.
func funcSig(fd *ast.FuncDecl) string {
	var in, out []string
	if fd.Type.TypeParams != nil {
		for _, f := range fd.Type.TypeParams.List {
			for range f.Names {
				in = append(in, "["+types.ExprString(f.Type)+"]")
			}
		}
	}
	if fd.Type.Params != nil {
		for _, f := range fd.Type.Params.List {
			es := types.ExprString(f.Type)
			if len(f.Names) == 0 {
				in = append(in, es)
				continue
			}
			for range f.Names {
				in = append(in, es)
			}
		}
	}
	if fd.Type.Results != nil {
		for _, f := range fd.Type.Results.List {
			es := types.ExprString(f.Type)
			if len(f.Names) == 0 {
				out = append(out, es)
				continue
			}
			for range f.Names {
				out = append(out, es)
			}
		}
	}
	return "func(" + strings.Join(in, ",") + ")(" + strings.Join(out, ",") + ")"
}

// receiverTypeName extracts the base type name of a method receiver.
func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	switch t := recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

// typeShape renders the consumer-visible shape of an exported type:
// struct → exported field:type set; interface → method set; other → the
// underlying expression (alias/basic/selected).
func typeShape(s *ast.TypeSpec) string {
	switch t := s.Type.(type) {
	case *ast.StructType:
		var fields []string
		for _, f := range t.Fields.List {
			es := types.ExprString(f.Type)
			if len(f.Names) == 0 {
				// embedded: visible by its type expression
				fields = append(fields, es)
				continue
			}
			for _, n := range f.Names {
				if n.IsExported() {
					fields = append(fields, n.Name+":"+es)
				}
			}
		}
		sort.Strings(fields)
		return "struct{" + strings.Join(fields, ",") + "}"
	case *ast.InterfaceType:
		var methods []string
		for _, f := range t.Methods.List {
			if len(f.Names) == 0 {
				methods = append(methods, types.ExprString(f.Type))
				continue
			}
			for _, n := range f.Names {
				methods = append(methods, n.Name+":"+funcSigFromFuncType(f.Type))
			}
		}
		sort.Strings(methods)
		return "iface{" + strings.Join(methods, ",") + "}"
	default:
		return types.ExprString(s.Type)
	}
}

// funcSigFromFuncType renders a bare func type (interface methods).
func funcSigFromFuncType(expr ast.Expr) string {
	ft, ok := expr.(*ast.FuncType)
	if !ok {
		return types.ExprString(expr)
	}
	fd := &ast.FuncDecl{Type: ft}
	return funcSig(fd)
}
