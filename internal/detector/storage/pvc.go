// Copyright 2026 Zen Mesh. All rights reserved.

// Package storage implements the PVC/PV lifecycle detector (§7/§8).
//
// LAWS:
//   - age is EVIDENCE, never deletion authority: detection above policy
//     thresholds produces opportunities, never executions;
//   - PVC-class actions are advisory-first (REPORT_ONLY /
//     REQUEST_STORAGE_RELEASE); destructive availability requires a
//     protection-free scan AND an explicit policy binding elsewhere;
//   - unknown consumer state biases PROTECTED (an empty use list from an
//     unknown-quality source is UNKNOWN, not empty);
//   - cost phrasing is MEASURED_RESOURCE only — capacity is not dollars.
package storage

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zenmesh/zen-cleaner/internal/opportunity"
)

// PVCFact / PVFact are the detector's input facts (informer snapshots in
// production; fixtures in tests). Consumer lists carry a quality flag: the
// detector must distinguish "no consumers observed" from "consumers not
// enumerable".
type PVCFact struct {
	Name           string
	Namespace      string
	Phase          string // Pending | Bound | Lost
	CreationTime   time.Time
	StorageClass   string
	RequestedBytes int64
	CapacityBytes  int64
	InUseByPods    []string
	ReferencedBy   []string // StatefulSet/other controller claim refs
	Finalizers     []string
	HoldReason     string // non-empty = intentional hold (forensic/restore/reserved)
	ConsumersKnown bool   // true when the consumer enumeration is trustworthy
}

type PVFact struct {
	Name          string
	Phase         string // Available | Bound | Released | Failed
	ReclaimPolicy string
	CreationTime  time.Time
	CapacityBytes int64
	ClaimRef      string // namespace/name when bound/released
}

// Provider abstracts the fact source.
type Provider interface {
	PVCs() []PVCFact
	PVs() []PVFact
	Estate() string
}

// Thresholds bound detection (policy values, injected).
type Thresholds struct {
	MinPendingAge time.Duration // PVC Pending considered abandoned
	MinUnusedAge  time.Duration // Bound-with-no-consumers considered unused
	MinOrphanAge  time.Duration // PV Released/Available considered orphan
}

// DefaultThresholds are conservative product defaults; policies narrow.
func DefaultThresholds() Thresholds {
	return Thresholds{
		MinPendingAge: 7 * 24 * time.Hour,
		MinUnusedAge:  14 * 24 * time.Hour,
		MinOrphanAge:  14 * 24 * time.Hour,
	}
}

// Detector is the pluggable PVC/PV opportunity detector (§36).
type Detector struct {
	Provider   Provider
	Thresholds Thresholds
}

// New constructs the detector with default thresholds.
func New(p Provider) *Detector {
	return &Detector{Provider: p, Thresholds: DefaultThresholds()}
}

// Scan emits opportunities for the current fact snapshot. It never mutates
// anything (§23: dry-run-first law — the detector IS the discovery half).
func (d *Detector) Scan(ctx context.Context) ([]opportunity.Opportunity, error) {
	if d.Provider == nil {
		return nil, fmt.Errorf("storage detector: nil provider")
	}
	now := time.Now().UTC()
	out := make([]opportunity.Opportunity, 0, 8)
	pvByName := map[string]PVFact{}
	for _, pv := range d.Provider.PVs() {
		pvByName[pv.Name] = pv
	}
	for _, pvc := range d.Provider.PVCs() {
		identity := fmt.Sprintf("PVC/%s/%s", pvc.Namespace, pvc.Name)
		age := now.Sub(pvc.CreationTime)
		switch {
		case pvc.Phase == "Pending" && age >= d.Thresholds.MinPendingAge:
			out = append(out, d.pending(identity, pvc, age, now))
		case pvc.Phase == "Bound" && age >= d.Thresholds.MinUnusedAge:
			out = append(out, d.boundUnused(identity, pvc, age, now))
		}
	}
	for _, pv := range d.Provider.PVs() {
		age := now.Sub(pv.CreationTime)
		if age < d.Thresholds.MinOrphanAge {
			continue
		}
		out = append(out, d.pvOrphan(pv, age, now))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DedupKey < out[j].DedupKey })
	return out, nil
}

// protections assembles the lattice for one candidate (§6).
func protections(pvc PVCFact) []opportunity.Protection {
	var p []opportunity.Protection
	if len(pvc.Finalizers) > 0 {
		p = append(p, opportunity.ProtFinalizerActive)
	}
	if pvc.HoldReason != "" {
		p = append(p, opportunity.ProtLegalRetention)
	}
	if len(pvc.ReferencedBy) > 0 {
		p = append(p, opportunity.ProtReferenced)
	}
	if !pvc.ConsumersKnown {
		p = append(p, opportunity.ProtControllerUnresolv)
	}
	return p
}

func (d *Detector) pending(identity string, pvc PVCFact, age time.Duration, now time.Time) opportunity.Opportunity {
	o := opportunity.Opportunity{
		DedupKey:         opportunity.ComposeDedupKey(d.Provider.Estate(), opportunity.TypeStorageLongUnbound, identity, "PVC_PENDING_BEYOND_POLICY_AGE"),
		Type:             opportunity.TypeStorageLongUnbound,
		ResourceClass:    "k8s-pvc",
		Identity:         identity,
		Estate:           d.Provider.Estate(),
		Namespace:        pvc.Namespace,
		Detector:         "storage/pvc-lifecycle",
		DiscoveredAt:     now,
		LastSeenAt:       now,
		FirstObservedAge: age,
		CurrentState:     "Pending",
		ExpectedState:    "Bound or released by owner",
		Reason:           "PVC_PENDING_BEYOND_POLICY_AGE",
		EvidenceQuality:  opportunity.QualityMeasured,
		Evidence: []opportunity.EvidenceRef{
			{Kind: "kube-object", Ref: fmt.Sprintf("%s phase=Pending age>=%s class=%s", identity, d.Thresholds.MinPendingAge, pvc.StorageClass), At: now},
		},
		Recommendation:   "confirm intent with owner; request storage release if abandoned",
		AvailableActions: []opportunity.Action{opportunity.ActionReportOnly, opportunity.ActionRequestStorageRel},
		RiskClass:        opportunity.RiskR3Consequential,
		CostClass:        opportunity.CostMeasuredResource,
		Impact:           fmt.Sprintf("requested %d bytes provisioned but unbound", pvc.RequestedBytes),
		State:            opportunity.StateOpen,
	}
	o.Protections = protections(pvc)
	if pvc.ConsumersKnown && len(pvc.InUseByPods) == 0 && len(pvc.ReferencedBy) == 0 && pvc.HoldReason == "" && len(pvc.Finalizers) == 0 {
		o.AvailableActions = append(o.AvailableActions, opportunity.ActionRecheck)
	}
	return o
}

func (d *Detector) boundUnused(identity string, pvc PVCFact, age time.Duration, now time.Time) opportunity.Opportunity {
	o := opportunity.Opportunity{
		DedupKey:         opportunity.ComposeDedupKey(d.Provider.Estate(), opportunity.TypeStorageUnused, identity, "BOUND_NO_OBSERVED_CONSUMER"),
		Type:             opportunity.TypeStorageUnused,
		ResourceClass:    "k8s-pvc",
		Identity:         identity,
		Estate:           d.Provider.Estate(),
		Namespace:        pvc.Namespace,
		Detector:         "storage/pvc-lifecycle",
		DiscoveredAt:     now,
		LastSeenAt:       now,
		FirstObservedAge: age,
		CurrentState:     "Bound",
		ExpectedState:    "bound with consumers, or released by owner",
		Reason:           "BOUND_NO_OBSERVED_CONSUMER",
		EvidenceQuality:  opportunity.QualityMeasured,
		Recommendation:   "verify consumer absence with owner before any release",
		AvailableActions: []opportunity.Action{opportunity.ActionReportOnly, opportunity.ActionRequestStorageRel},
		RiskClass:        opportunity.RiskR3Consequential,
		CostClass:        opportunity.CostMeasuredResource,
		Impact:           fmt.Sprintf("requested %d bytes bound without observed consumer", pvc.RequestedBytes),
		State:            opportunity.StateOpen,
	}
	o.Protections = protections(pvc)
	return o
}

func (d *Detector) pvOrphan(pv PVFact, age time.Duration, now time.Time) opportunity.Opportunity {
	t := opportunity.TypeStorageOrphan
	reason := "PV_AVAILABLE_OUTSIDE_POOL"
	switch {
	case pv.Phase == "Released":
		reason = "PV_RELEASED_RETAIN_POLICY"
	case pv.Phase == "Bound" || pv.Phase == "Available" && false:
		reason = "PV_AVAILABLE_OUTSIDE_POOL"
	}
	var prots []opportunity.Protection
	if pv.ClaimRef != "" {
		prots = append(prots, opportunity.ProtReferenced)
	}
	o := opportunity.Opportunity{
		DedupKey:         opportunity.ComposeDedupKey(d.Provider.Estate(), t, "PV/"+pv.Name, reason),
		Type:             t,
		ResourceClass:    "k8s-pv",
		Identity:         "PV/" + pv.Name,
		Estate:           d.Provider.Estate(),
		Detector:         "storage/pvc-lifecycle",
		DiscoveredAt:     now,
		LastSeenAt:       now,
		FirstObservedAge: age,
		CurrentState:     pv.Phase,
		ExpectedState:    "Deleted, re-bound, or declared pool member",
		Reason:           reason,
		EvidenceQuality:  opportunity.QualityMeasured,
		Evidence: []opportunity.EvidenceRef{
			{Kind: "kube-object", Ref: fmt.Sprintf("PV/%s phase=%s reclaim=%s", pv.Name, pv.Phase, pv.ReclaimPolicy), At: now},
		},
		Recommendation:   "review reclaim policy with storage owner; retain is intentional until proven otherwise",
		AvailableActions: []opportunity.Action{opportunity.ActionReportOnly},
		RiskClass:        opportunity.RiskR3Consequential,
		CostClass:        opportunity.CostMeasuredResource,
		Impact:           fmt.Sprintf("capacity %d bytes held in %s", pv.CapacityBytes, pv.Phase),
		PolicyRef:        "reclaimPolicy=" + pv.ReclaimPolicy,
		Protections:      prots,
		State:            opportunity.StateOpen,
	}
	return o
}

// SanitizeIdentity keeps namespace/name grammar (no path tricks).
func SanitizeIdentity(ns, name string) string {
	return fmt.Sprintf("%s/%s", strings.ReplaceAll(ns, "/", "_"), strings.ReplaceAll(name, "/", "_"))
}
