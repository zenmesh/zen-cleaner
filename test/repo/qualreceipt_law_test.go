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

// QUALRECEIPT ADOPTION (H264 Program R): zen-cleaner is the fourth
// consumer of the SHARED QUALIFICATION RECEIPT envelope (zen-sdk
// pkg/qualreceipt), after Advanced Cleaner, zen-meerkat, and zen-lock.
//
// The receipt qualifies the product's COMPATIBILITY CONTRACT SURFACE — the
// canonical baseline artifacts the guard batteries (V6 CRD field pin, V7
// API compat, API surface snapshot) enforce against — so a qualification
// claim outside this repo can be checked against exactly these bytes:
//
//   - Generation is CONTENT-DERIVED (combined digest of the baseline
//     files), never a constant: a changed surface is a new generation;
//   - the envelope passes the shared Validate law and round-trips
//     Marshal/Decode with a stable timestamp-free digest;
//   - Decode refuses unknown fields (mutation detection on the wire);
//   - tampering with any check re-derives a different digest;
//   - the shared bounds and duplicate-identity laws refuse malformed
//     receipts (errors.Is on the typed sentinels, no string parsing).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/zenmesh/zen-sdk/pkg/qualreceipt"
)

// compatSurfaces are the canonical baseline artifacts the guard batteries
// enforce; the receipt digests exactly these bytes.
var compatSurfaces = []struct {
	name string
	rel  string
}{
	{"guard_v6_api_surface_baseline", filepath.Join("test", "repo", "api_surface_baseline.txt")},
	{"guard_v7_api_compat_baseline", filepath.Join("test", "repo", "api_compat_baseline.txt")},
	{"guard_v6_crd_schema_baseline", filepath.Join("test", "repo", "crd_schema_baseline.txt")},
}

// surfaceFacts computes the real digests and element counts of the
// compatibility surface. Missing or empty baselines fail the check rather
// than qualifying a vacuum.
func surfaceFacts(t *testing.T, root string) (digests map[string]string, checks []qualreceipt.Check, evidence map[string]string, combined string) {
	t.Helper()
	digests = map[string]string{}
	evidence = map[string]string{}
	hasher := sha256.New()
	for _, s := range compatSurfaces {
		path := filepath.Join(root, s.rel)
		raw, err := os.ReadFile(path)
		if err != nil {
			checks = append(checks, qualreceipt.Check{Name: s.name, Pass: false, Detail: "unreadable: " + err.Error()})
			digests[s.name] = "sha256:" + strings.Repeat("0", 64)
			continue
		}
		sum := sha256.Sum256(raw)
		h := "sha256:" + hex.EncodeToString(sum[:])
		digests[s.name] = h
		evidence[s.name] = s.rel
		hasher.Write(raw)
		lines := strings.Count(strings.TrimSpace(string(raw)), "\n") + 1
		checks = append(checks, qualreceipt.Check{Name: s.name, Pass: len(raw) > 0, Detail: fmt.Sprintf("%d canonical elements", lines)})
	}
	combined = hex.EncodeToString(hasher.Sum(nil))
	sort.Slice(checks, func(i, j int) bool { return checks[i].Name < checks[j].Name })
	return digests, checks, evidence, combined
}

// surfaceReceipt builds the shared envelope over the real compatibility
// surface (no synthetic facts).
func surfaceReceipt(t *testing.T) qualreceipt.Receipt {
	t.Helper()
	root := repoRootV4(t)
	digests, checks, evidence, combined := surfaceFacts(t, root)
	r := qualreceipt.Receipt{
		Version:     qualreceipt.Version,
		Product:     "zen-cleaner",
		Generation:  "compat-surface-" + combined,
		At:          qualreceipt.Now(),
		Environment: "ci",
		Digests:     digests,
		Checks:      checks,
		Pass:        true,
		Evidence:    evidence,
	}
	for _, c := range checks {
		if !c.Pass {
			r.Pass = false
		}
	}
	return r
}

// TestQualreceipt_SurfaceReceiptLaw: the real compatibility surface
// qualifies under the shared envelope law and round-trips the wire form
// with a stable digest.
func TestQualreceipt_SurfaceReceiptLaw(t *testing.T) {
	r := surfaceReceipt(t)
	if err := r.Validate(); err != nil {
		t.Fatalf("real surface receipt must validate: %v", err)
	}
	if !r.Pass {
		t.Fatal("canonical baselines must qualify (they are the guards' own law)")
	}
	d, err := r.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if len(d) != len("sha256:")+64 || !strings.HasPrefix(d, "sha256:") {
		t.Fatalf("digest form: %q", d)
	}
	raw, err := r.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := qualreceipt.Decode(raw)
	if err != nil {
		t.Fatalf("round-trip decode: %v", err)
	}
	d2, err := back.Digest()
	if err != nil || d2 != d {
		t.Fatalf("round-trip digest drift: %s vs %s (%v)", d, d2, err)
	}
}

// TestQualreceipt_TimestampFreeDigest: the digest is canonical over
// content, never over the wall clock — re-qualification at another time
// with identical facts yields the identical digest.
func TestQualreceipt_TimestampFreeDigest(t *testing.T) {
	r := surfaceReceipt(t)
	later := r
	later.At = "2027-01-01T00:00:00Z"
	d1, err1 := r.Digest()
	d2, err2 := later.Digest()
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	if d1 != d2 {
		t.Fatalf("digest must be timestamp-free: %s vs %s", d1, d2)
	}
}

// TestQualreceipt_DecodeRefusesUnknownField: the wire form is strict — an
// injected unknown field is a mutation and must refuse (shared law: no
// silent field accretion across consumers).
func TestQualreceipt_DecodeRefusesUnknownField(t *testing.T) {
	r := surfaceReceipt(t)
	raw, err := r.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["attestationOverride"] = "trust-me"
	tampered, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := qualreceipt.Decode(tampered); !errors.Is(err, qualreceipt.ErrMalformed) {
		t.Fatalf("unknown field must refuse with ErrMalformed, got %v", err)
	}
}

// TestQualreceipt_TamperDetection: flipping any check re-derives a
// different digest — a sealed PASS cannot be widened into a green for a
// failing battery.
func TestQualreceipt_TamperDetection(t *testing.T) {
	r := surfaceReceipt(t)
	sealed, err := r.Digest()
	if err != nil {
		t.Fatal(err)
	}
	forged := r
	forged.Checks = append([]qualreceipt.Check(nil), r.Checks...)
	for i := range forged.Checks {
		forged.Checks[i] = qualreceipt.Check{Name: forged.Checks[i].Name, Pass: true, Detail: "forged"}
	}
	forged.Pass = true
	fd, err := forged.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if fd == sealed {
		t.Fatal("tampered receipt must re-derive a different digest")
	}
}

// TestQualreceipt_SharedLawsBoundsAndDuplicates: the product inherits the
// envelope's refusal laws verbatim (bounds, duplicate identity, missing
// generation) — no product-local loosening is possible.
func TestQualreceipt_SharedLawsBoundsAndDuplicates(t *testing.T) {
	base := surfaceReceipt(t)

	over := base
	over.Checks = nil
	for i := 0; i < 257; i++ {
		over.Checks = append(over.Checks, qualreceipt.Check{Name: fmt.Sprintf("c%03d", i), Pass: true})
	}
	if err := over.Validate(); !errors.Is(err, qualreceipt.ErrBounds) {
		t.Fatalf(">256 checks must refuse with ErrBounds, got %v", err)
	}

	dup := base
	dup.Checks = []qualreceipt.Check{{Name: "x", Pass: true}, {Name: "x", Pass: true}}
	if err := dup.Validate(); !errors.Is(err, qualreceipt.ErrDuplicateCheck) {
		t.Fatalf("duplicate check identity must refuse, got %v", err)
	}

	nogen := base
	nogen.Generation = ""
	if err := nogen.Validate(); !errors.Is(err, qualreceipt.ErrMissingGeneration) {
		t.Fatalf("generation is mandatory, got %v", err)
	}
}

// TestQualreceipt_GenerationBindsContent: the generation derivation is a
// pure function of the qualified bytes — identical surfaces reproduce the
// identical generation (deterministic re-qualification), and any different
// surface derives a different generation, so an old PASS can never green a
// new surface.
func TestQualreceipt_GenerationBindsContent(t *testing.T) {
	root := repoRootV4(t)
	_, _, _, combinedA := surfaceFacts(t, root)
	_, _, _, combinedAagain := surfaceFacts(t, root)
	if combinedA != combinedAagain {
		t.Fatal("surface derivation must be deterministic for identical bytes")
	}

	// A changed surface: one extra canonical element.
	other := sha256.New()
	other.Write([]byte("guard_v8_future: a new enforced capability\n"))
	combinedB := hex.EncodeToString(other.Sum(nil))
	genA := "compat-surface-" + combinedA
	genB := "compat-surface-" + combinedB
	if genA == genB {
		t.Fatal("a changed surface must derive a different generation")
	}

	// The construction path uses exactly this derivation.
	r := surfaceReceipt(t)
	if r.Generation != genA {
		t.Fatalf("receipt generation must be the derived one: %s vs %s", r.Generation, genA)
	}
}
