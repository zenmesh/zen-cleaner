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

package controller

// SUPPORT2-033 §4 (policy-freshness): the batch decision was made on a
// policy snapshot; an irreversible DELETE must not execute under a snapshot
// whose governing policy has been deleted, recreated, or spec-edited. The
// check re-reads through an UNCACHED reader immediately before the DELETE.
// HONEST LIMIT (asserted by design, not by test): the uncached read narrows
// the GET→DELETE window; it cannot make updates to two Kubernetes objects
// atomic.

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	gcapi "github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
	"github.com/zenmesh/zen-cleaner/pkg/safety"
)

func freshnessScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	sch := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(sch); err != nil {
		t.Fatal(err)
	}
	if err := gcapi.AddToScheme(sch); err != nil {
		t.Fatal(err)
	}
	return sch
}

func livePolicyFor(pol *gcapi.ZenCleanerPolicy) *gcapi.ZenCleanerPolicy {
	live := pol.DeepCopy()
	live.ResourceVersion = "rv-1"
	return live
}

func freshnessReconciler(t *testing.T, readerPolicy *gcapi.ZenCleanerPolicy) (*PolicyReconciler, *apiserverLawFake) {
	t.Helper()
	sch := freshnessScheme(t)
	f := newAPIServerLawFake(t, cmFixture("target", "uid-MATCH"))
	crClient := fake.NewClientBuilder().WithScheme(sch).Build()
	r := NewPolicyReconcilerWithRESTMapper(crClient, sch, f.dc, nil, NewStatusUpdater(f.dc), NewEventRecorder(nil), func() *config.ControllerConfig {
		c := config.NewControllerConfig()
		c.DeleteEnabled = true
		return c
	}())
	if readerPolicy != nil {
		reader := fake.NewClientBuilder().WithScheme(sch).WithObjects(readerPolicy).Build()
		r.SetAPIReader(reader)
	}
	return r, f
}

func basePolicy() *gcapi.ZenCleanerPolicy {
	p := cmPolicy()
	p.Generation = 7
	return p
}

// Deleted policy: the snapshot is from a dead policy — refuse, no DELETE.
func TestPerformResourceDeletion_PolicyDeleted_Refused(t *testing.T) {
	r, f := freshnessReconciler(t, nil) // reader without the policy = NotFound
	deleteReader := fake.NewClientBuilder().WithScheme(freshnessScheme(t)).Build()
	r.SetAPIReader(deleteReader)

	outcome, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-MATCH"), cmGVR,
		buildDeleteOptions(basePolicy()), basePolicy())
	if err != nil {
		t.Fatalf("freshness refusal must not be an API failure, got: %v", err)
	}
	if outcome.Outcome != OutcomeRefused || outcome.Reason != safety.ReasonPolicyChanged {
		t.Fatalf("outcome = %+v, want refused/policy_changed", outcome)
	}
	if f.hasDelete() {
		t.Fatal("a DELETE was issued under a deleted policy")
	}
	assertSurvives(t, f, cmGVR, nsScope, "target", "uid-MATCH")
}

// Recreated policy (same name, new UID): refuse.
func TestPerformResourceDeletion_PolicyRecreatedUIDChanged_Refused(t *testing.T) {
	live := basePolicy()
	live.UID = "policy-uid-RECREATED"
	r, f := freshnessReconciler(t, livePolicyFor(live))

	outcome, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-MATCH"), cmGVR,
		buildDeleteOptions(basePolicy()), basePolicy())
	if err != nil {
		t.Fatalf("got: %v", err)
	}
	if outcome.Reason != safety.ReasonPolicyChanged {
		t.Fatalf("outcome = %+v, want policy_changed", outcome)
	}
	if f.hasDelete() {
		t.Fatal("a DELETE was issued under a recreated policy")
	}
}

// Spec-edited policy (generation moved — e.g. dry-run flipped or exclusions
// changed): refuse; the queued reconcile re-evaluates under the new spec.
func TestPerformResourceDeletion_PolicyGenerationMoved_Refused(t *testing.T) {
	live := basePolicy()
	live.Generation = 8
	r, f := freshnessReconciler(t, livePolicyFor(live))

	outcome, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-MATCH"), cmGVR,
		buildDeleteOptions(basePolicy()), basePolicy())
	if err != nil {
		t.Fatalf("got: %v", err)
	}
	if outcome.Reason != safety.ReasonPolicyChanged {
		t.Fatalf("outcome = %+v, want policy_changed", outcome)
	}
	if f.hasDelete() {
		t.Fatal("a DELETE was issued under an edited policy")
	}
}

// Unchanged policy: the freshness check passes and the delete proceeds.
func TestPerformResourceDeletion_PolicyUnchanged_Deletes(t *testing.T) {
	live := basePolicy()
	r, f := freshnessReconciler(t, livePolicyFor(live))

	outcome, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-MATCH"), cmGVR,
		buildDeleteOptions(basePolicy()), basePolicy())
	if err != nil {
		t.Fatalf("got: %v", err)
	}
	if outcome.Outcome != OutcomeDeleted {
		t.Fatalf("outcome = %+v, want deleted", outcome)
	}
	if _, gerr := f.dc.Tracker().Get(cmGVR, nsScope, "target"); !isNotFoundErr(gerr) {
		t.Fatalf("object should be gone, get err: %v", gerr)
	}
}

// No uncached reader wired (unit tests only; production always wires one):
// the check is skipped, never fabricated.
func TestPerformResourceDeletion_NoAPIReader_CheckSkipped(t *testing.T) {
	r, f := freshnessReconciler(t, nil)
	r.apiReader = nil

	outcome, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-MATCH"), cmGVR,
		buildDeleteOptions(basePolicy()), basePolicy())
	if err != nil {
		t.Fatalf("got: %v", err)
	}
	if outcome.Outcome != OutcomeDeleted {
		t.Fatalf("outcome = %+v, want deleted (check skipped without a reader)", outcome)
	}
	if _, gerr := f.dc.Tracker().Get(cmGVR, nsScope, "target"); !isNotFoundErr(gerr) {
		t.Fatalf("object should be gone, get err: %v", gerr)
	}
}

// A policy re-read FAILURE refuses (fail closed), never allows.
func TestPerformResourceDeletion_PolicyRereadFailure_Refused(t *testing.T) {
	r, f := freshnessReconciler(t, basePolicy())
	// A reader that fails every Get (scheme without the policy type).
	broken := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()
	r.SetAPIReader(broken)
	_ = f

	outcome, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-MATCH"), cmGVR,
		buildDeleteOptions(basePolicy()), basePolicy())
	if err != nil {
		t.Fatalf("freshness refusal must not be an API failure, got: %v", err)
	}
	if outcome.Reason != safety.ReasonPolicyChanged {
		t.Fatalf("outcome = %+v, want policy_changed on read failure", outcome)
	}
	if f.hasDelete() {
		t.Fatal("a DELETE was issued despite a failed policy re-read")
	}
}

// nil deleteOptions: the preconditions are built on a fresh options object.
func TestPerformResourceDeletion_NilDeleteOptions_StillPreconditioned(t *testing.T) {
	live := cmFixture("target", "uid-MATCH")
	f := newAPIServerLawFake(t, live)
	r := newDeleter(t, f)

	outcome, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-MATCH"), cmGVR, nil, cmPolicy())
	if err != nil {
		t.Fatalf("got: %v", err)
	}
	if outcome.Outcome != OutcomeDeleted {
		t.Fatalf("outcome = %+v, want deleted", outcome)
	}
	pre := f.deletes[0].GetDeleteOptions().Preconditions
	if pre == nil || pre.UID == nil || *pre.UID != "uid-MATCH" || pre.ResourceVersion == nil || *pre.ResourceVersion != "rv-1" {
		t.Fatalf("nil deleteOptions must still carry both preconditions: %+v", pre)
	}
}

func isNotFoundErr(err error) bool {
	return apierrors.IsNotFound(err)
}
