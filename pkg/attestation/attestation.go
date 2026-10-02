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

// Package attestation is zen-cleaner's OPTIONAL_TRUST_ATTESTATION_V1
// applicability declaration (H265+ Program W): a declarative statement of
// which product subjects MAY carry optional Trust attestation evidence,
// with which recommended strategies — and the laws that keep attestation
// an evidence overlay, never an input to deletion safety.
//
// CORE LAW: deletion safety is independent of proof. The safety gate runs
// unconditionally on every destructive decision; turning
// attestation OFF changes nothing about what may be deleted. This package
// imports no decision machinery, and no decision path imports this
// package — the decoupling is structural and test-enforced.
//
// Vocabulary note: this declaration uses PUBLIC product subjects only.
// Private lifecycle vocabulary (opportunities, plans, approved actions,
// realized outcomes) belongs to a different product and must never appear
// here.
package attestation

// Strategy is the closed attestation-strategy vocabulary (OPTIONAL_TRUST_ATTESTATION_V1).
type Strategy string

const (
	StrategyOff                 Strategy = "OFF"
	StrategyAlways              Strategy = "ALWAYS"
	StrategyDeterministicSample Strategy = "DETERMINISTIC_SAMPLE"
	StrategyBudgeted            Strategy = "BUDGETED"
	StrategyConditional         Strategy = "CONDITIONAL"
)

// Enforcement separates hard refusal from best-effort continuation.
type Enforcement string

const (
	// EnforcementRequired: without a valid attestation the operation is
	// refused. ONLY lawful for policy-level guarantees, never for the
	// deletion safety gate.
	EnforcementRequired Enforcement = "REQUIRED"
	// EnforcementBestEffort: the operation continues UNATTESTED with a
	// recorded reason when attestation is unavailable.
	EnforcementBestEffort Enforcement = "BEST_EFFORT"
)

// Subject is one product operation class in the declaration.
type Subject struct {
	// Name is the public subject identity.
	Name string
	// Attestable: can the operation carry optional attestation at all?
	Attestable bool
	// RecommendedStrategy is the product's declarative recommendation
	// (registry C121 default column, product-owned value).
	RecommendedStrategy Strategy
	// RecommendedEnforcement is the enforcement recommendation.
	RecommendedEnforcement Enforcement
	// RequiredCandidate: could this subject ever be policy-REQUIRED?
	// Deletion execution must NEVER be a required candidate: that would
	// make safety dependent on proof availability.
	RequiredCandidate bool
	// DigestSource names what an attestation receipt would bind.
	DigestSource string
	// PrivacyNotes: what must never enter a receipt (policy identities and
	// object names stay out; counts and digests only).
	PrivacyNotes string
}

// Applicability is the product-owned declaration.
var Applicability = []Subject{
	{
		Name: "POLICY_EVALUATION_CYCLE", Attestable: true,
		RecommendedStrategy: StrategyDeterministicSample, RecommendedEnforcement: EnforcementBestEffort,
		RequiredCandidate: false,
		DigestSource:      "cycle receipt digest (matched/refused/deleted counts + policy generation)",
		PrivacyNotes:      "aggregated counts only; no object names, no policy contents",
	},
	{
		Name: "DELETION_EXECUTION", Attestable: true,
		RecommendedStrategy: StrategyOff, RecommendedEnforcement: EnforcementBestEffort,
		RequiredCandidate: false,
		DigestSource:      "execution receipt digest (refusal-reason histogram + deletion counts)",
		PrivacyNotes:      "refusal reason classes only; never object identity, never policy identity",
	},
}

// ByName returns the declaration row for a subject.
func ByName(name string) (Subject, bool) {
	for _, s := range Applicability {
		if s.Name == name {
			return s, true
		}
	}
	return Subject{}, false
}
