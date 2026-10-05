// Copyright 2026 Zen Mesh. All rights reserved.

package ops

import (
	"encoding/json"
	"sort"
	"testing"
)

func TestRegistrySortedAndUnique(t *testing.T) {
	ids := make([]string, 0, len(Registry))
	seen := map[string]bool{}
	for _, op := range Registry {
		if op.ID == "" {
			t.Fatal("an operation has no ID")
		}
		if seen[op.ID] {
			t.Fatalf("duplicate operation ID: %s", op.ID)
		}
		seen[op.ID] = true
		ids = append(ids, op.ID)
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatalf("the registry must be ID-sorted: %v", ids)
	}
}

func TestEveryOperationCarriesTheLaws(t *testing.T) {
	for _, op := range Registry {
		if op.Kind == "" {
			t.Errorf("%s: no Kind", op.ID)
		}
		if op.Consequence == "" {
			t.Errorf("%s: no Consequence (the effect must be typed)", op.ID)
		}
		if op.Idempotency == "" {
			t.Errorf("%s: no Idempotency (the retry law must be typed)", op.ID)
		}
		if op.Auth == "" {
			t.Errorf("%s: no Auth (the authority must be typed)", op.ID)
		}
		if len(op.Bindings) == 0 && len(op.AbsentSurfaces) == 0 {
			t.Errorf("%s: no bindings and no absent typing (the census must be concrete)", op.ID)
		}
	}
}

// TestDeletionNeverBindsHTTP pins the §17 law: the deletion batch
// NEVER binds a REST route — a REST deletion API would be a new
// operation requiring its own front door, never a shortcut.
func TestDeletionNeverBindsHTTP(t *testing.T) {
	for _, op := range Registry {
		if op.ID != "cleaner.executions.delete-batch" {
			continue
		}
		if _, has := op.Bindings[SurfaceAPI]; has {
			t.Fatal("the deletion batch must never bind the REST surface (the §17 law)")
		}
		if note, has := op.AbsentSurfaces[SurfaceAPI]; !has || !strings_Contains(note, "§17") {
			t.Fatalf("the deletion batch's API absence must carry the §17 note: %q", note)
		}
	}
}

func strings_Contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestRegistryJSONRoundTrip(t *testing.T) {
	b, err := json.Marshal(Registry)
	if err != nil {
		t.Fatal(err)
	}
	var back []Operation
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != len(Registry) {
		t.Fatalf("the round trip lost operations: %d vs %d", len(back), len(Registry))
	}
}
