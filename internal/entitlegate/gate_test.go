// Package entitlegate test: the H2 custody-block adoption proof for
// zen-cleaner — a SYNTHETIC policy (synthetic issuer/org/grants)
// exercised end-to-end through the shared engine at the deletion
// enforcement point. No real org, key, or figure anywhere.
package entitlegate_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/zenmesh/zen-cleaner/internal/entitlegate"
	"github.com/zenmesh/zen-sdk/pkg/directoryrefs"
	"github.com/zenmesh/zen-sdk/pkg/entitlement"
	"github.com/zenmesh/zen-sdk/pkg/entitlement/engine"
)

func issuer(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func result(t *testing.T, key *ecdsa.PrivateKey, ents []entitlement.Entitlement) *entitlement.Result {
	t.Helper()
	now := time.Now()
	snap := entitlement.Snapshot{
		Schema:       entitlement.Schema,
		SnapshotID:   "synthetic-cleaner-snap",
		Organization: directoryrefs.OrganizationRef{ID: "synthetic-org"},
		Revision:     9,
		IssuedAt:     now.Add(-time.Hour).UTC().Format(time.RFC3339),
		ExpiresAt:    now.Add(24 * time.Hour).UTC().Format(time.RFC3339),
		Issuer:       "platform-synthetic-issuer",
		Entitlements: ents,
	}
	signed, err := entitlement.Sign(snap, key, "synthetic-key-1")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	res, err := entitlement.Verify(&key.PublicKey, &signed, now, entitlement.VerifyOpts{
		TrustedIssuer: "platform-synthetic-issuer",
		TrustedKeyID:  "synthetic-key-1",
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	return &res
}

func TestDeletionBatchesUnderSyntheticPolicy(t *testing.T) {
	key := issuer(t)
	// Synthetic grant: 2 deletion batches this window.
	res := result(t, key, []entitlement.Entitlement{{Key: entitlegate.EntitlementKey, Limit: 2}})
	meter, err := engine.NewMeter("org://synthetic-org", func() time.Time { return time.Now().UTC() })
	if err != nil {
		t.Fatal(err)
	}
	g := entitlegate.New(res, meter)
	ran := 0
	exec := func() func(context.Context) error {
		return func(context.Context) error { ran++; return nil }
	}
	if err := g.Execute(context.Background(), "plan-digest-1", exec()); err != nil {
		t.Fatalf("batch 1: %v", err)
	}
	if ran != 1 || g.Executed() != 1 {
		t.Fatalf("after batch 1: ran=%d executed=%d", ran, g.Executed())
	}
	// A failed deletion batch releases its reservation (failed deletions
	// never bill).
	if err := g.Execute(context.Background(), "plan-digest-2", func(context.Context) error {
		return errors.New("API server unreachable")
	}); err == nil {
		t.Fatal("failed batch admitted silently")
	}
	if g.Executed() != 1 {
		t.Fatalf("failed batch billed (%d)", g.Executed())
	}
	if err := g.Execute(context.Background(), "plan-digest-2", exec()); err != nil {
		t.Fatalf("batch 2: %v", err)
	}
	// Window spent: the third batch refuses typed and never runs.
	if err := g.Execute(context.Background(), "plan-digest-3", exec()); !errors.Is(err, engine.ErrQuotaExhausted) {
		t.Fatalf("batch 3: %v", err)
	}
	if ran != 2 {
		t.Fatal("exhausted batch ran anyway")
	}
	// The plan-digest approval binding is enforced (a missing digest
	// refuses inside the admitted operation — the quota is released).
	if err := g.Execute(context.Background(), "", exec()); err == nil {
		t.Fatal("digest-less execution admitted")
	}
	if g.Executed() != 2 {
		t.Fatalf("digest-less batch billed (%d)", g.Executed())
	}
	// Metered exactly the successes; the chain holds; billing aggregates.
	events := meter.Events()
	if len(events) != 2 || meter.Misses() != 0 {
		t.Fatalf("meter: %d events / %d misses", len(events), meter.Misses())
	}
	if corrupt := engine.VerifyChain(events); corrupt != 0 {
		t.Fatalf("chain corrupt at %d", corrupt)
	}
	handle, err := meter.ExportBilling(time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil || handle.EventCount != 2 || handle.Lines[0].Quantity != 2 {
		t.Fatalf("billing: (%+v,%v)", handle, err)
	}
}

func TestNoGrantRefusesTyped(t *testing.T) {
	key := issuer(t)
	g := entitlegate.New(result(t, key, nil), nil)
	ran := false
	var denied *engine.ErrDenied
	if err := g.Execute(context.Background(), "plan-digest-x", func(context.Context) error {
		ran = true
		return nil
	}); !errors.As(err, &denied) || denied.Decision.Reason != engine.ReasonUnknownEntitlement {
		t.Fatalf("no-grant batch: %v", err)
	}
	if ran {
		t.Fatal("no-grant batch ran anyway")
	}
}
