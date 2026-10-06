// Copyright 2026 Zen Mesh. All rights reserved.

// The canonical registry doc: docs/architecture/operation-registry.json
// (the four-surface parity law's canonical path) emitted DETERMINISTICALLY
// from the Go registry — the support precedent ("deterministic ... pinned
// by test"). The census reads THIS file; internal/ops stays the source.
package ops

import (
	"encoding/json"
	"sort"
	"strings"
)

// lawClass maps the registry Kind to the parity law's closed vocabulary.
func lawClass(k Kind) string {
	switch k {
	case KindMutation:
		return "IRREVERSIBLE_MUTATION" // the deletion batch deletes; the receipt records, never restores
	case KindDryRun:
		return "READ_ONLY" // the plan without the effect
	default:
		return "READ_ONLY"
	}
}

// DocOperation is one operation in the canonical doc shape (the census's
// consumption contract: id + surfaces read, never inferred).
type DocOperation struct {
	ID                  string             `json:"operation_id"`
	Product             string             `json:"product"`
	Domain              string             `json:"domain,omitempty"`
	Summary             string             `json:"summary"`
	Kind                Kind               `json:"kind"`
	ConsequenceClass    string             `json:"consequence_class"`
	Consequence         string             `json:"consequence"`
	AuthRequirement     string             `json:"auth_requirement"`
	TenantScope         string             `json:"tenant_environment_scope"`
	Idempotency         string             `json:"idempotency"`
	OCC                 string             `json:"occ"`
	DestructiveCeremony string             `json:"destructive_ceremony"`
	Surfaces            map[Surface]bool   `json:"surfaces"`
	Bindings            map[Surface]string `json:"surface_bindings"`
	AbsentSurfaces      map[Surface]string `json:"absent_surfaces,omitempty"`
}

// Doc is the canonical document root.
type Doc struct {
	Schema        string         `json:"schema"`
	Product       string         `json:"product"`
	Note          string         `json:"note"`
	OperationsAbs int            `json:"operations_total"`
	Operations    []DocOperation `json:"operations"`
}

// CanonicalDoc builds the deterministic doc from the registry.
func CanonicalDoc() Doc {
	ops := append([]Operation{}, Registry...)
	sort.Slice(ops, func(i, j int) bool { return ops[i].ID < ops[j].ID })
	doc := Doc{
		Schema:        "zen-cleaner/operation-registry-doc/v1",
		Product:       "zen-cleaner",
		Note:          "the canonical operation registry (the four-surface parity law): emitted from internal/ops (the source) — never hand-edited; the sync test pins it",
		OperationsAbs: len(ops),
	}
	for _, op := range ops {
		doc.Operations = append(doc.Operations, DocOperation{
			ID:                  op.ID,
			Product:             "zen-cleaner",
			Domain:              op.Domain,
			Summary:             op.Summary,
			Kind:                op.Kind,
			ConsequenceClass:    op.ConsequenceClass,
			Consequence:         op.Consequence,
			AuthRequirement:     op.Auth,
			TenantScope:         op.TenantScope,
			Idempotency:         op.Idempotency,
			OCC:                 op.OCC,
			DestructiveCeremony: op.DestructiveCeremony,
			Surfaces:            op.SurfaceBools(),
			Bindings:            op.Bindings,
			AbsentSurfaces:      op.AbsentSurfaces,
		})
	}
	return doc
}

// CanonicalDocJSON is the deterministic JSON body (sorted keys, stable
// indent) — byte-identical across runs at the same generation.
func CanonicalDocJSON() ([]byte, error) {
	b, err := json.MarshalIndent(CanonicalDoc(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

var _ = strings.TrimSpace
