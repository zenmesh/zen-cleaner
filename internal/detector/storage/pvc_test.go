// Copyright 2026 Zen Mesh. All rights reserved.

// §78/§79 adversarial battery: false positives MUST NOT surface and true
// orphans MUST surface, with the protection lattice intact. Age is never
// deletion authority: no destructive action survives protection.
package storage

import (
	"context"
	"testing"
	"time"

	"github.com/zenmesh/zen-cleaner/internal/opportunity"
)

type fixture struct {
	pvcs []PVCFact
	pvs  []PVFact
}

func (f *fixture) PVCs() []PVCFact { return f.pvcs }
func (f *fixture) PVs() []PVFact   { return f.pvs }
func (f *fixture) Estate() string  { return "test-estate" }

var old = time.Now().UTC().Add(-45 * 24 * time.Hour)

func find(ops []opportunity.Opportunity, t opportunity.Type, id string) *opportunity.Opportunity {
	for i := range ops {
		if ops[i].Type == t && ops[i].Identity == id {
			return &ops[i]
		}
	}
	return nil
}

// FALSE POSITIVE: a PVC temporarily unbound during rollout (young) must not
// surface — below policy threshold.
func TestFPYoungPendingPVCNotSurfaced(t *testing.T) {
	f := &fixture{pvcs: []PVCFact{{Name: "rollout", Namespace: "app", Phase: "Pending", CreationTime: time.Now().UTC().Add(-2 * time.Hour), ConsumersKnown: true}}}
	ops, err := New(f).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if find(ops, opportunity.TypeStorageLongUnbound, "PVC/app/rollout") != nil {
		t.Fatal("§78: young pending PVC must not surface (age is evidence, not authority)")
	}
}

// FALSE POSITIVE: StatefulSet scale-to-zero retains its claim — referenced
// PVCs are PROTECTED, never released.
func TestFPScaleToZeroSTSReferencedIsProtected(t *testing.T) {
	f := &fixture{pvcs: []PVCFact{{Name: "db-data", Namespace: "app", Phase: "Bound",
		CreationTime: old, ReferencedBy: []string{"STS/app/db"}, ConsumersKnown: true}}}
	ops, _ := New(f).Scan(context.Background())
	o := find(ops, opportunity.TypeStorageUnused, "PVC/app/db-data")
	if o == nil {
		t.Fatal("the fact may surface as evidence")
	}
	if !o.Protected() {
		t.Fatal("§78: STS-referenced PVC must be PROTECTED (scale-to-zero retention)")
	}
	for _, a := range o.ExecutableActions() {
		switch a {
		case opportunity.ActionRequestStorageRel:
			t.Fatal("release must not survive STS protection")
		}
	}
}

// FALSE POSITIVE: restore-source / forensic hold (owner annotation) is
// legal-retention protected.
func TestFPRestoreSourceHoldProtected(t *testing.T) {
	f := &fixture{pvcs: []PVCFact{{Name: "restore-src", Namespace: "ops", Phase: "Pending",
		CreationTime: old, HoldReason: "restore-source-per-owner", ConsumersKnown: true}}}
	ops, _ := New(f).Scan(context.Background())
	o := find(ops, opportunity.TypeStorageLongUnbound, "PVC/ops/restore-src")
	if o == nil {
		t.Fatal("held PVC may still surface as an opportunity fact")
	}
	protected := false
	for _, p := range o.Protections {
		if p == opportunity.ProtLegalRetention {
			protected = true
		}
	}
	if !protected {
		t.Fatal("§78: forensic/restore hold must map to LEGAL_SECURITY_RETENTION protection")
	}
}

// FALSE NEGATIVE: a genuinely abandoned pending PVC with known-empty
// consumers must surface with the release request available.
func TestFNAbandonedPendingSurfaces(t *testing.T) {
	f := &fixture{pvcs: []PVCFact{{Name: "dead", Namespace: "qual", Phase: "Pending",
		CreationTime: old, InUseByPods: []string{}, ReferencedBy: []string{}, ConsumersKnown: true}}}
	ops, _ := New(f).Scan(context.Background())
	o := find(ops, opportunity.TypeStorageLongUnbound, "PVC/qual/dead")
	if o == nil {
		t.Fatal("§79: abandoned PVC must surface")
	}
	if !o.Protected() {
		if o.AvailableActions == nil {
			t.Fatal("unprotected abandoned PVC should carry advisory+request actions")
		}
	}
	if o.CostClass != opportunity.CostMeasuredResource {
		t.Fatal("§10: capacity impact is MEASURED_RESOURCE, never dollars")
	}
}

// UNKNOWN consumer enumeration biases PROTECTED (§6/§62).
func TestUnknownConsumersProtect(t *testing.T) {
	f := &fixture{pvcs: []PVCFact{{Name: "mystery", Namespace: "x", Phase: "Bound",
		CreationTime: old, InUseByPods: nil, ConsumersKnown: false}}}
	ops, _ := New(f).Scan(context.Background())
	o := find(ops, opportunity.TypeStorageUnused, "PVC/x/mystery")
	if o == nil {
		t.Fatal("unknown-consumer PVC may surface as evidence")
	}
	found := false
	for _, p := range o.Protections {
		if p == opportunity.ProtControllerUnresolv {
			found = true
		}
	}
	if !found {
		t.Fatal("§6: unknown consumer state must bias PROTECTED")
	}
}

// Released PV under Retain is the flagship "why are we paying for this"
// opportunity — and it is REPORT_ONLY (advisory, never destructive).
func TestReleasedRetainPVSurfacesReportOnly(t *testing.T) {
	f := &fixture{pvs: []PVFact{{Name: "pv-old", Phase: "Released", ReclaimPolicy: "Retain",
		CreationTime: old, CapacityBytes: 50 << 30, ClaimRef: "old-ns/old-claim"}}}
	ops, _ := New(f).Scan(context.Background())
	o := find(ops, opportunity.TypeStorageOrphan, "PV/pv-old")
	if o == nil {
		t.Fatal("released retain PV must surface")
	}
	if !o.Protected() {
		t.Fatal("claim-referenced PV must be protected")
	}
	for _, a := range o.ExecutableActions() {
		if a != opportunity.ActionReportOnly {
			t.Fatalf("§8: PV-class action must stay REPORT_ONLY under protection; got %s", a)
		}
	}
}

// PROPERTY (§80): protection dominates action across a randomized lattice —
// any protection present removes destructive action availability.
func TestPropertyProtectionDominatesAction(t *testing.T) {
	allProts := []opportunity.Protection{
		opportunity.ProtFinalizerActive, opportunity.ProtReferenced,
		opportunity.ProtLegalRetention, opportunity.ProtControllerUnresolv,
	}
	for _, prot := range allProts {
		o := opportunity.Opportunity{
			AvailableActions: []opportunity.Action{
				opportunity.ActionReportOnly, opportunity.ActionRequestStorageRel,
				opportunity.ActionDeleteConfirmedOrph,
			},
			Protections: []opportunity.Protection{prot},
		}
		for _, a := range o.ExecutableActions() {
			if a == opportunity.ActionDeleteConfirmedOrph || a == opportunity.ActionRequestStorageRel {
				t.Fatalf("protection %s must suppress action %s", prot, a)
			}
		}
	}
}
