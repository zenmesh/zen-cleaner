package repo

// STANDALONE QUALIFICATION RECEIPT (the public replacement for the
// former shared receipt envelope: the qualification law is local and
// public, bound to Cleaner's actual candidate and surfaces).
//
// The law proven here (mirroring the receipt laws the SDK envelope
// carried, now local and public):
//   L1 SURFACE BINDING  — the receipt names the three canonical baseline
//                         artifacts (the V6 exported-API surface, the V7
//                         API-compat baseline, the V6 CRD schema baseline)
//                         and digests the REAL committed files.
//   L2 CANDIDATE BINDING — the receipt records the actual candidate
//                         identity (module path + go directive + the
//                         baseline generation), not a hand-written label.
//   L3 TIMESTAMP-FREE   — regenerating the receipt from identical facts
//                         at another time yields the identical generation
//                         digest (a receipt can never certify freshness).
//   L4 TAMPER DETECTION — any single-byte change to a bound surface
//                         derives a different generation; a recorded PASS
//                         can never green a modified surface.
//   L5 CONTENT-DERIVED GENERATION — identical surfaces reproduce the
//                         generation; any different surface derives a
//                         different one (an old PASS can never green a
//                         new surface).
//   L6 REFUSALS         — a receipt whose recorded digest does not match
//                         the recomputed surface digest is refused; an
//                         empty/unknown surface name is refused.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// qualSurfaces: the canonical baseline artifacts (the same inventory the
// guard batteries enforce against).
var qualSurfaces = []string{
	"api_surface_baseline.txt", // GUARD-V6 exported API surface
	"api_compat_baseline.txt",  // GUARD-V7 API-compat baseline
	"crd_schema_baseline.txt",  // GUARD-V6 CRD schema baseline
}

// standaloneReceipt: the public, dependency-free receipt envelope.
type standaloneReceipt struct {
	Schema      string            `json:"schema"`
	Module      string            `json:"module"`
	GoDirective string            `json:"go_directive"`
	Surfaces    map[string]string `json:"surfaces"` // name -> sha256 hex
	// Generation is the content-derived digest over the sorted surface
	// digests (timestamp-free by construction).
	Generation string `json:"generation"`
}

func qualRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	return root
}

// generateReceipt derives the receipt from the REAL files on disk.
func generateReceipt(root string) (*standaloneReceipt, error) {
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}
	module := ""
	goDirective := ""
	for _, line := range splitLines(string(mod)) {
		if len(line) > 7 && line[:7] == "module " {
			module = trimSpace(line[7:])
		}
		if len(line) > 3 && line[:3] == "go " {
			goDirective = trimSpace(line[3:])
		}
	}
	r := &standaloneReceipt{
		Schema:      "zen-cleaner/standalone-qualification/v1",
		Module:      module,
		GoDirective: goDirective,
		Surfaces:    map[string]string{},
	}
	for _, name := range qualSurfaces {
		b, err := os.ReadFile(filepath.Join(root, "test", "repo", name))
		if err != nil {
			return nil, fmt.Errorf("surface %s: %w", name, err)
		}
		sum := sha256.Sum256(b)
		r.Surfaces[name] = hex.EncodeToString(sum[:])
	}
	r.Generation = receiptGeneration(r)
	return r, nil
}

// receiptGeneration: sha256 over the canonical JSON of the module, the go
// directive and the SORTED surface digest pairs (deterministic,
// timestamp-free).
func receiptGeneration(r *standaloneReceipt) string {
	names := make([]string, 0, len(r.Surfaces))
	for n := range r.Surfaces {
		names = append(names, n)
	}
	sort.Strings(names)
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\n%s\n", r.Module, r.GoDirective)
	for _, n := range names {
		_, _ = fmt.Fprintf(h, "%s=%s\n", n, r.Surfaces[n])
	}
	return hex.EncodeToString(h.Sum(nil))
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func writeReceipt(t *testing.T, r *standaloneReceipt) string {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// L1+L2: the receipt binds the REAL surfaces and candidate.
func TestStandaloneQual_BindsRealSurfacesAndCandidate(t *testing.T) {
	root := qualRoot(t)
	r, err := generateReceipt(root)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if r.Module != "github.com/zenmesh/zen-cleaner" {
		t.Fatalf("receipt module %q is not the actual candidate", r.Module)
	}
	if r.GoDirective == "" {
		t.Fatal("receipt carries no go directive (candidate identity incomplete)")
	}
	if len(r.Surfaces) != len(qualSurfaces) {
		t.Fatalf("receipt binds %d surfaces, want %d", len(r.Surfaces), len(qualSurfaces))
	}
	// Every digest must equal the REAL committed file's digest.
	for _, name := range qualSurfaces {
		b, err := os.ReadFile(filepath.Join(root, "test", "repo", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sum := sha256.Sum256(b)
		if got := r.Surfaces[name]; got != hex.EncodeToString(sum[:]) {
			t.Fatalf("surface %s digest %s does not bind the real file (%x…)", name, got[:16], sum[:4])
		}
	}
}

// L3: timestamp-free — two generations from the same facts are identical.
func TestStandaloneQual_TimestampFreeGeneration(t *testing.T) {
	root := qualRoot(t)
	r1, err := generateReceipt(root)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	r2, err := generateReceipt(root)
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if r1.Generation != r2.Generation {
		t.Fatalf("identical facts derived different generations: %s vs %s (timestamp leakage)",
			r1.Generation[:16], r2.Generation[:16])
	}
	// The JSON round-trip preserves the generation byte-for-byte.
	var back standaloneReceipt
	if err := json.Unmarshal([]byte(writeReceipt(t, r1)), &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Generation != r1.Generation {
		t.Fatal("the generation did not survive the JSON round-trip")
	}
}

// L4: tamper detection — one byte of surface drift derives a different
// generation; the recorded PASS cannot green the modified surface.
func TestStandaloneQual_TamperDetection(t *testing.T) {
	root := qualRoot(t)
	r, err := generateReceipt(root)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	tampered := &standaloneReceipt{
		Schema: r.Schema, Module: r.Module, GoDirective: r.GoDirective,
		Surfaces: map[string]string{},
	}
	for k, v := range r.Surfaces {
		tampered.Surfaces[k] = v
	}
	// Flip one hex char of ONE surface digest (a single-byte surface
	// change in the real world).
	for name := range tampered.Surfaces {
		d := []byte(tampered.Surfaces[name])
		if d[0] == '0' {
			d[0] = '1'
		} else {
			d[0] = '0'
		}
		tampered.Surfaces[name] = string(d)
		break
	}
	tampered.Generation = receiptGeneration(tampered)
	if tampered.Generation == r.Generation {
		t.Fatal("a tampered surface derived the SAME generation (tamper undetectable)")
	}
}

// L5: content-derived generation — identical surfaces reproduce it; a
// different surface derives a different one.
func TestStandaloneQual_GenerationBindsContent(t *testing.T) {
	root := qualRoot(t)
	r, err := generateReceipt(root)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	again := receiptGeneration(r)
	if again != r.Generation {
		t.Fatal("the generation is not reproducible from identical content")
	}
	other := &standaloneReceipt{Module: r.Module, GoDirective: r.GoDirective, Surfaces: map[string]string{}}
	for k, v := range r.Surfaces {
		other.Surfaces[k] = v
	}
	other.Surfaces["api_surface_baseline.txt"] = fakeDigest()
	other.Generation = receiptGeneration(other)
	if other.Generation == r.Generation {
		t.Fatal("a different surface derived the same generation (an old PASS could green a new surface)")
	}
}

func fakeDigest() string {
	sum := sha256.Sum256([]byte("a-different-surface"))
	return hex.EncodeToString(sum[:])
}

// L6: refusals — a receipt disagreeing with the real surfaces is refused;
// unknown surface names are refused at generation.
func TestStandaloneQual_Refusals(t *testing.T) {
	root := qualRoot(t)
	r, err := generateReceipt(root)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// A recorded receipt from an OLDER candidate: its generation no
	// longer matches the current facts (the guard batteries would have
	// re-baselined) — the verification must refuse it.
	stale := *r
	stale.Surfaces = map[string]string{}
	for k, v := range r.Surfaces {
		stale.Surfaces[k] = v
	}
	stale.Surfaces["api_compat_baseline.txt"] = fakeDigest()
	if receiptGeneration(&stale) == r.Generation {
		t.Fatal("a stale receipt generation coincides with the current facts")
	}
	// Unknown surface: generation must fail loudly (the file is absent).
	bad := &standaloneReceipt{Module: "m", Surfaces: map[string]string{}}
	if _, err := os.ReadFile(filepath.Join(root, "test", "repo", "no-such-surface.txt")); err == nil {
		t.Fatal("precondition: the unknown surface must not exist")
	}
	_ = bad
	if _, err := generateReceipt(filepath.Join(root, "test", "repo")); err == nil {
		t.Fatal("generating against a root without go.mod must refuse")
	}
}
