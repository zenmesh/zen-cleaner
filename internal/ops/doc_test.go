// Copyright 2026 Zen Mesh. All rights reserved.

package ops

import (
	"os"
	"path/filepath"
	"testing"
)

// The canonical doc is pinned by test (the support precedent): the
// committed docs/architecture/operation-registry.json must be
// byte-identical to the emission from the Go registry. Regenerate with
// ZEN_CLEANER_OPS_REGISTRY_DOC_UPDATE=1 and commit.
func TestCanonicalRegistryDocSync(t *testing.T) {
	want, err := CanonicalDocJSON()
	if err != nil {
		t.Fatalf("emission: %v", err)
	}
	path := filepath.Join("..", "..", "docs", "architecture", "operation-registry.json")
	if os.Getenv("ZEN_CLEANER_OPS_REGISTRY_DOC_UPDATE") == "1" {
		if mkerr := os.MkdirAll(filepath.Dir(path), 0o755); mkerr != nil {
			t.Fatalf("mkdir: %v", mkerr)
		}
		if werr := os.WriteFile(path, want, 0o644); werr != nil {
			t.Fatalf("write: %v", werr)
		}
		t.Logf("regenerated %s", path)
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the canonical doc is missing (%v) — regenerate with ZEN_CLEANER_OPS_REGISTRY_DOC_UPDATE=1", err)
	}
	if string(got) != string(want) {
		t.Fatal("the canonical doc drifted from the Go registry — regenerate with ZEN_CLEANER_OPS_REGISTRY_DOC_UPDATE=1 and commit")
	}
}
