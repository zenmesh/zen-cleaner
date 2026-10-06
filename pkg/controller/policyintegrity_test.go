// Copyright 2026 Zen Mesh. All rights reserved.

package controller

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testSpec() v1alpha1.ZenCleanerPolicySpec {
	ttl := int64(3600)
	return v1alpha1.ZenCleanerPolicySpec{
		TargetResource: v1alpha1.TargetResourceSpec{
			Kind:      "Pod",
			Namespace: "sandbox",
		},
		TTL: v1alpha1.TTLSpec{SecondsAfterCreation: &ttl},
	}
}

// The digest is deterministic (the same spec always yields the same hex)
// and mutation-sensitive (any spec change yields a different digest).
func TestSpecDigestDeterministicAndMutationSensitive(t *testing.T) {
	a, err := SpecDigest(testSpec())
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	b, err := SpecDigest(testSpec())
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if a != b || len(a) != 64 {
		t.Fatalf("the digest must be deterministic 64-hex: %q vs %q", a, b)
	}
	mutated := testSpec()
	ttl := int64(7200)
	mutated.TTL.SecondsAfterCreation = &ttl
	c, err := SpecDigest(mutated)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if c == a {
		t.Fatal("a mutated spec must yield a different digest")
	}
}

// The sign/verify round trip, and the tamper refusals.
func TestPolicyIntegrityVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	v := NewPolicyIntegrityVerifier(pub)
	if v == nil || !v.Enabled() {
		t.Fatal("a valid key must enable the verifier")
	}
	digest, _ := SpecDigest(testSpec())
	sig := ed25519.Sign(priv, []byte(digest))
	if err := v.Verify(digest, base64.StdEncoding.EncodeToString(sig)); err != nil {
		t.Fatalf("the honest round trip must verify: %v", err)
	}
	if err := v.Verify(digest, base64.StdEncoding.EncodeToString(sig)+"x"); err == nil {
		t.Fatal("a corrupted signature must refuse")
	}
	if err := v.Verify(strings.Repeat("ab", 32), base64.StdEncoding.EncodeToString(sig)); err == nil {
		t.Fatal("a signature over another digest must refuse")
	}
	short := make([]byte, ed25519.SignatureSize-1)
	if err := v.Verify(digest, base64.StdEncoding.EncodeToString(short)); err == nil {
		t.Fatal("a wrong-length signature must refuse")
	}
}

func TestNewPolicyIntegrityVerifierNilOnBadKey(t *testing.T) {
	if NewPolicyIntegrityVerifier(ed25519.PublicKey{}) != nil {
		t.Fatal("an empty key must yield a nil verifier (the overlay OFF)")
	}
	var nilV *PolicyIntegrityVerifier
	if nilV.Enabled() {
		t.Fatal("a nil verifier is the overlay OFF")
	}
	if err := nilV.Verify("ab", "cg=="); err == nil {
		t.Fatal("a nil verifier must refuse verification")
	}
}

// The verdicts: in-sync, visibility-only drift, and the fail-closed law
// with the anchor configured.
func TestCheckIntegrityVerdicts(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	spec := testSpec()
	digest, _ := SpecDigest(spec)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(digest)))
	otherDigest, _ := SpecDigest(func() v1alpha1.ZenCleanerPolicySpec {
		m := testSpec()
		ttl := int64(60)
		m.TTL.SecondsAfterCreation = &ttl
		return m
	}())

	t.Run("unstamped overlay off is in-sync", func(t *testing.T) {
		p := &v1alpha1.ZenCleanerPolicy{Spec: spec}
		v := (*PolicyIntegrityVerifier)(nil).CheckIntegrity(p)
		if v.Drifted || v.Refuses {
			t.Fatalf("an unstamped policy under no anchor must sail: %+v", v)
		}
	})

	t.Run("stampede drift is visibility-only without the anchor", func(t *testing.T) {
		p := &v1alpha1.ZenCleanerPolicy{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{AnnDeclaredDigest: otherDigest}},
			Spec:       spec,
		}
		v := (*PolicyIntegrityVerifier)(nil).CheckIntegrity(p)
		if !v.Drifted || v.Refuses {
			t.Fatalf("drift without the anchor must be visibility-only: %+v", v)
		}
		if !strings.Contains(v.Detail, "visibility-only") {
			t.Fatalf("the detail must type the posture: %s", v.Detail)
		}
	})

	t.Run("anchor configured: unsigned policy refuses", func(t *testing.T) {
		vrf := NewPolicyIntegrityVerifier(pub)
		p := &v1alpha1.ZenCleanerPolicy{Spec: spec}
		v := vrf.CheckIntegrity(p)
		if !v.Refuses {
			t.Fatalf("the anchor makes the signature mandatory: %+v", v)
		}
		if !strings.Contains(v.Detail, "no signature") {
			t.Fatalf("the detail must name the law: %s", v.Detail)
		}
	})

	t.Run("anchor configured: honest signed in-sync sails", func(t *testing.T) {
		vrf := NewPolicyIntegrityVerifier(pub)
		p := &v1alpha1.ZenCleanerPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					AnnDeclaredDigest:  digest,
					AnnPolicySignature: sig,
					AnnPolicySigner:    "gitops-pipeline",
				},
			},
			Spec: spec,
		}
		v := vrf.CheckIntegrity(p)
		if v.Refuses || v.Drifted {
			t.Fatalf("the honest signed artifact must sail: %+v", v)
		}
	})

	t.Run("anchor configured: drift on a signed policy refuses the mutated intent", func(t *testing.T) {
		vrf := NewPolicyIntegrityVerifier(pub)
		// signed over the OLD digest, but the live spec mutated
		p := &v1alpha1.ZenCleanerPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					AnnDeclaredDigest:  digest,
					AnnPolicySignature: sig,
				},
			},
			Spec: func() v1alpha1.ZenCleanerPolicySpec {
				m := testSpec()
				ttl := int64(60)
				m.TTL.SecondsAfterCreation = &ttl
				return m
			}(),
		}
		v := vrf.CheckIntegrity(p)
		if !v.Refuses || !v.Drifted {
			t.Fatalf("a drifted signed policy must refuse: %+v", v)
		}
		if !strings.Contains(v.Detail, "DRIFT") {
			t.Fatalf("the detail must type the drift: %s", v.Detail)
		}
		_ = otherDigest
	})

	t.Run("anchor configured: garbage signature refuses", func(t *testing.T) {
		vrf := NewPolicyIntegrityVerifier(pub)
		p := &v1alpha1.ZenCleanerPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					AnnDeclaredDigest:  digest,
					AnnPolicySignature: "not-base64!!",
				},
			},
			Spec: spec,
		}
		v := vrf.CheckIntegrity(p)
		if !v.Refuses {
			t.Fatalf("garbage must refuse: %+v", v)
		}
	})
}
