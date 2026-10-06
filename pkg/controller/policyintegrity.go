// Copyright 2026 Zen Mesh. All rights reserved.

// THE POLICY-INTEGRITY LAWS (the git-gitops LEAD row: the declarative
// cleanup-policy surface, SIGNED, DRIFT-DETECTED).
//
// A declarative artifact carries its own integrity with it: the applying
// pipeline stamps the spec digest it declared (an annotation) and MAY sign
// it (ed25519 over the digest, base64, with the signer identity). At
// reconcile the controller compares the LIVE spec digest against the
// declared one and verifies the signature against the configured trust
// anchor.
//
//   - SIGNED: when the controller runs with a verification key, a policy
//     without a valid signature is policy_invalid — fail-closed. This is a
//     POLICY-LEVEL guarantee (who declared this), the only enforcement the
//     OPTIONAL_TRUST_ATTESTATION_V1 law permits required enforcement for;
//     deletion safety itself stays independent of proof (pkg/attestation).
//   - DRIFT-DETECTED: a live spec digest that diverges from the declared
//     digest is always receipted as the PolicyDrifted condition — visible,
//     typed. With the trust anchor configured, drift additionally refuses
//     execution: the live spec is not the artifact that was declared and
//     signed. Without the anchor the drift is visibility-only and execution
//     proceeds (the overlay OFF, per the attestation law).
package controller

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

// The integrity annotations the applying pipeline stamps.
const (
	// AnnDeclaredDigest carries the sha256 hex digest of the policy spec
	// exactly as the declarative source (git) declared it.
	AnnDeclaredDigest = "zen-cleaner.io/declared-digest"
	// AnnPolicySignature carries the base64 ed25519 signature over the
	// digest hex string.
	AnnPolicySignature = "zen-cleaner.io/policy-signature"
	// AnnPolicySigner records the signer identity (display only; the
	// trust decision comes from the key, never the label).
	AnnPolicySigner = "zen-cleaner.io/policy-signer"
)

// SpecDigest is the canonical digest of a policy spec: deterministic JSON
// (the struct's field order), sha256 hex. The same spec always yields the
// same digest; any mutation yields a different one.
func SpecDigest(spec v1alpha1.ZenCleanerPolicySpec) (string, error) {
	b, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("marshal spec for digest: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// PolicyIntegrityVerifier holds the configured trust anchor. A nil
// receiver (no key configured) means the signing overlay is OFF.
type PolicyIntegrityVerifier struct {
	key ed25519.PublicKey
}

// NewPolicyIntegrityVerifier builds the verifier from an ed25519 public
// key (the trust anchor). A nil key returns a nil verifier (the overlay
// OFF) — the caller checks nil.
func NewPolicyIntegrityVerifier(key ed25519.PublicKey) *PolicyIntegrityVerifier {
	if len(key) != ed25519.PublicKeySize {
		return nil
	}
	return &PolicyIntegrityVerifier{key: key}
}

// Enabled reports whether the signing overlay is enforced.
func (v *PolicyIntegrityVerifier) Enabled() bool { return v != nil }

// Verify checks the base64 ed25519 signature over the digest hex string.
func (v *PolicyIntegrityVerifier) Verify(digestHex, sigB64 string) error {
	if v == nil {
		return fmt.Errorf("no trust anchor configured")
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("signature is not valid base64: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("signature is %d bytes, want %d", len(sig), ed25519.SignatureSize)
	}
	if !ed25519.Verify(v.key, []byte(digestHex), sig) {
		return fmt.Errorf("signature does not verify against the trust anchor")
	}
	return nil
}

// IntegrityVerdict is the typed outcome of the reconcile-time check.
type IntegrityVerdict struct {
	// Digest is the LIVE spec's digest (what the controller computed).
	Digest string
	// Declared is the digest the artifact declared ("" when unstamped).
	Declared string
	// Drifted: the live spec diverges from the declared digest.
	Drifted bool
	// Refuses: the check refuses execution (policy_invalid).
	Refuses bool
	// Detail is the bounded operator-facing message.
	Detail string
}

// CheckIntegrity runs the declared-vs-live comparison and, when the
// trust anchor is configured, the signature verification.
func (v *PolicyIntegrityVerifier) CheckIntegrity(policy *v1alpha1.ZenCleanerPolicy) IntegrityVerdict {
	verdict := IntegrityVerdict{}
	digest, err := SpecDigest(policy.Spec)
	if err != nil {
		verdict.Refuses = true
		verdict.Detail = "policy integrity: spec digest failed: " + err.Error()
		return verdict
	}
	verdict.Digest = digest
	verdict.Declared = policy.Annotations[AnnDeclaredDigest]

	if verdict.Declared != "" && verdict.Declared != digest {
		verdict.Drifted = true
	}

	if v.Enabled() {
		sig := policy.Annotations[AnnPolicySignature]
		if sig == "" {
			verdict.Refuses = true
			verdict.Detail = "policy integrity: the trust anchor is configured but the policy carries no signature (" + AnnPolicySignature + ")"
			return verdict
		}
		if verr := v.Verify(verdict.Declared, sig); verr != nil {
			verdict.Refuses = true
			verdict.Detail = "policy integrity: signature verification failed: " + verr.Error()
			return verdict
		}
		// Signed and verified: a live spec that no longer matches the
		// signed digest is exactly the mutation the anchor exists to catch.
		if verdict.Drifted {
			verdict.Refuses = true
			verdict.Detail = fmt.Sprintf("policy integrity: DRIFT — the live spec digest %s diverges from the signed declaration %s; refusing the mutated intent",
				shortDigest(verdict.Digest), shortDigest(verdict.Declared))
			return verdict
		}
		verdict.Detail = "policy integrity: signature verified against the trust anchor; the live spec matches the signed declaration"
		return verdict
	}

	// The overlay OFF: drift is visibility-only.
	if verdict.Drifted {
		verdict.Detail = fmt.Sprintf("policy integrity: DRIFT (visibility-only; no trust anchor configured) — the live spec digest %s diverges from the declared digest %s",
			shortDigest(verdict.Digest), shortDigest(verdict.Declared))
		return verdict
	}
	verdict.Detail = "policy integrity: the live spec matches the declared digest (no trust anchor configured)"
	return verdict
}

func shortDigest(d string) string {
	if len(d) > 12 {
		return d[:12]
	}
	return d
}
