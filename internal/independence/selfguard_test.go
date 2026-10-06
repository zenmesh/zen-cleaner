// Copyright 2026 Zen Mesh. All rights reserved.

// THE CLEANER SELF-INDEPENDENCE GUARD (PI-SC-001 §2A applied to this
// product): zen-cleaner is a PUBLIC repo with nothing around finops —
// its RUNTIME imports may reference the shared SDK only. Any other
// portfolio module imported at runtime would make Cleaner a consumer
// of another product's semantics, which the independence law forbids
// without a declaration.
//
// The law is enforced over THIS module's tree at test time: every
// non-test .go file is parsed, each import path is checked against
// the lawful closure, and any undeclared portfolio module is a typed
// violation.
package independence

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lawfulPrefixes: the portfolio modules zen-cleaner may import at
// runtime — itself, plus the shared SDK per the SDKWildcard law
// (every ->sdk edge is the contract surface; the entitlement-engine
// adoption runs through it).
var lawfulPrefixes = []string{
	"github.com/zenmesh/zen-cleaner",
	"github.com/zenmesh/zen-sdk",
}

// portfolioPrefixes: every module domain the estate census knows.
var portfolioPrefixes = []string{
	"github.com/zenmesh/zen-",
	"zen-mesh.io/zen-",
	"github.com/kube-zen/",
}

func TestCleanerSelfIndependence(t *testing.T) {
	var violations []string
	files := 0
	absRoot, aerr := filepath.Abs("../..")
	if aerr != nil {
		t.Fatal(aerr)
	}
	err := filepath.Walk(absRoot, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			name := fi.Name()
			if name == "vendor" || name == "node_modules" || name == "testdata" ||
				name == "test" || name == "fixtures" || name == "generated" ||
				strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return nil
		}
		files++
		for _, imp := range f.Imports {
			impPath := strings.Trim(imp.Path.Value, `"`)
			cross := false
			for _, pp := range portfolioPrefixes {
				if strings.HasPrefix(impPath, pp) {
					cross = true
					break
				}
			}
			if !cross {
				continue
			}
			lawful := false
			for _, lp := range lawfulPrefixes {
				if strings.HasPrefix(impPath, lp) {
					lawful = true
					break
				}
			}
			if !lawful {
				violations = append(violations, path+": "+impPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("no files walked — the guard is broken")
	}
	if len(violations) > 0 {
		t.Fatalf("CLEANER SELF-INDEPENDENCE violated: %v", violations)
	}
}
