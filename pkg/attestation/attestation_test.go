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

package attestation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeclarationLaws: complete rows; no REQUIRED candidacy anywhere
// (deletion safety must never depend on proof availability); DELETION_EXECUTION
// defaults OFF because the safety gate already refuses unconditionally.
func TestDeclarationLaws(t *testing.T) {
	if len(Applicability) == 0 {
		t.Fatal("declaration must not be empty")
	}
	seen := map[string]bool{}
	for _, s := range Applicability {
		if seen[s.Name] {
			t.Fatalf("duplicate subject %s", s.Name)
		}
		seen[s.Name] = true
		if s.DigestSource == "" || s.PrivacyNotes == "" {
			t.Fatalf("%s: incomplete row", s.Name)
		}
		if s.RequiredCandidate {
			t.Fatalf("%s: deletion-safety product must have no REQUIRED attestation candidate", s.Name)
		}
		if s.RecommendedStrategy == StrategyOff && s.RecommendedEnforcement == EnforcementRequired {
			t.Fatalf("%s: OFF cannot be silently downgraded to REQUIRED", s.Name)
		}
	}
	del, ok := ByName("DELETION_EXECUTION")
	if !ok {
		t.Fatal("DELETION_EXECUTION must be declared")
	}
	if del.RecommendedStrategy != StrategyOff {
		t.Fatal("deletion execution attestation defaults OFF: the safety gate is the authority, not proof")
	}
}

// TestStructuralDecoupling: this package imports no decision machinery and
// the decision package (pkg/safety) imports no attestation — attestation is
// an overlay, never an input to deletion safety, in EITHER direction.
func TestStructuralDecoupling(t *testing.T) {
	for _, dir := range []string{"."} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			// The law test itself must name the decision package path to
			// scan it; only the DECLARATION sources are self-scan banned.
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, banned := range []string{"pkg/safety", "pkg/controller", "EvaluateObject", "SafetyGate"} {
				if strings.Contains(string(raw), banned) {
					t.Fatalf("%s: attestation declaration must not reference decision machinery %q", e.Name(), banned)
				}
			}
		}
	}
	safetyDir := filepath.Join("..", "safety")
	entries, err := os.ReadDir(safetyDir)
	if err != nil {
		t.Fatalf("pkg/safety unreadable: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(safetyDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "pkg/attestation") {
			t.Fatalf("pkg/safety/%s: the safety gate must not import attestation", e.Name())
		}
	}
}

// TestPublicVocabularyOnly: the declaration must not carry private
// lifecycle vocabulary — those subjects belong to a different product.
func TestPublicVocabularyOnly(t *testing.T) {
	banned := []string{"opportunity", "plan", "approvedaction", "realizedoutcome", "maintenance"}
	for _, s := range Applicability {
		hay := strings.ToLower(s.Name + s.DigestSource + s.PrivacyNotes)
		for _, b := range banned {
			if strings.Contains(hay, b) {
				t.Fatalf("%s: private lifecycle vocabulary %q leaked into the public declaration", s.Name, b)
			}
		}
	}
}
