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

import (
	"context"
	"errors"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
	clienttesting "k8s.io/client-go/testing"

	cleanerlog "github.com/zenmesh/zen-cleaner/internal/logging"
	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"

	gcapi "github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/safety"
)

func testLogger() *cleanerlog.Logger { return cleanerlog.NewLogger("test") }

// cmFixture builds an unstructured ConfigMap fixture. status.completedAt is
// a FIXED past date, so the shared test policy's relative TTL
// (completedAt + 1h) is deterministically expired and delete-time
// eligibility re-evaluation passes for live objects. The anchor is a STRING
// because int64 fixture values do not survive the dynamic fake tracker's
// codec round-trip.
func cmFixture(name, uid string, opts ...func(*unstructured.Unstructured)) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":            name,
				"namespace":       "app",
				"resourceVersion": "rv-1",
			},
			"status": map[string]interface{}{
				"completedAt": "2020-01-01T00:00:00Z",
			},
		},
	}
	if uid != "" {
		obj.SetUID(types.UID(uid))
	}
	for _, opt := range opts {
		opt(obj)
	}
	return obj
}

// clusterFixture builds a cluster-scoped PriorityClass fixture (a
// cluster-scoped kind OUTSIDE the hard-protected set, so the safety gate
// allows it).
func clusterFixture(name, uid string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "scheduling.k8s.io/v1",
			"kind":       "PriorityClass",
			"metadata": map[string]interface{}{
				"name":            name,
				"resourceVersion": "rv-1",
			},
			"status": map[string]interface{}{
				"completedAt": "2020-01-01T00:00:00Z",
			},
		},
	}
	if uid != "" {
		obj.SetUID(types.UID(uid))
	}
	return obj
}

var (
	cmGVR   = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	pcsGVR  = schema.GroupVersionResource{Group: "scheduling.k8s.io", Version: "v1", Resource: "priorityclasses"}
	nsScope = "app"
)

// ttlPolicy is a policy whose relative TTL (status.completedAt + 1h) is
// deterministically expired for the test fixtures (the anchor is the fixed
// date 2020-01-01). targetNS "*" admits cluster-scoped fixtures; "app"
// admits the namespaced ones.
func ttlPolicy(targetNS string, mods ...func(*gcapi.ZenCleanerPolicy)) *gcapi.ZenCleanerPolicy {
	ttlZero := int64(0)
	p := &gcapi.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "pol-ns", Name: "pol", UID: "policy-uid-1"},
		Spec: gcapi.ZenCleanerPolicySpec{
			TargetResource: gcapi.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  targetNS,
			},
			TTL: gcapi.TTLSpec{
				RelativeTo:   "status.completedAt",
				SecondsAfter: int64Ptr(3600),
			},
			Behavior: gcapi.BehaviorSpec{
				GracePeriodSeconds: &ttlZero,
				PropagationPolicy:  "Background",
			},
		},
	}
	for _, mod := range mods {
		mod(p)
	}
	return p
}

func cmPolicy(mods ...func(*gcapi.ZenCleanerPolicy)) *gcapi.ZenCleanerPolicy {
	return ttlPolicy(nsScope, mods...)
}

func clusterPolicy(mods ...func(*gcapi.ZenCleanerPolicy)) *gcapi.ZenCleanerPolicy {
	p := ttlPolicy("*")
	p.Spec.TargetResource.APIVersion = "scheduling.k8s.io/v1"
	p.Spec.TargetResource.Kind = "PriorityClass"
	for _, mod := range mods {
		mod(p)
	}
	return p
}

// apiserverLawFake wraps the dynamic fake with a reactor that models the REAL
// API server DELETE contract: metav1.Preconditions (UID and resourceVersion)
// are validated against the CURRENT object before any deletion is performed.
// The dynamic fake itself ignores preconditions; this reactor is the model —
// it does not prove real API-server behavior (no envtest/kind harness exists
// in this repo), it deterministically reproduces the documented contract.
type apiserverLawFake struct {
	dc *dynamicfake.FakeDynamicClient
	// raceHook runs exactly once at the DELETE boundary of raceTarget (after
	// the live precondition GET inside performResourceDeletion, before the
	// tracker delete), modeling an object landing in the GET→DELETE window.
	raceHook   func(tracker clienttesting.ObjectTracker)
	raceTarget string
	// deleteErr, when non-nil, is returned INSTEAD of performing the delete
	// (models an API-side failure).
	deleteErr error
	// deletes records every delete action that reached the reactor.
	deletes []clienttesting.DeleteAction
}

func newAPIServerLawFake(t *testing.T, objs ...runtime.Object) *apiserverLawFake {
	t.Helper()
	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme.Scheme, map[schema.GroupVersionResource]string{
		cmGVR:  "ConfigMapList",
		pcsGVR: "PriorityClassList",
	}, objs...)
	f := &apiserverLawFake{dc: dc}
	dc.PrependReactor("delete", "*", f.react)
	return f
}

func (f *apiserverLawFake) react(action clienttesting.Action) (bool, runtime.Object, error) {
	da, ok := action.(clienttesting.DeleteAction)
	if !ok {
		return false, nil, nil
	}
	f.deletes = append(f.deletes, da)
	if f.deleteErr != nil {
		return true, nil, f.deleteErr
	}
	// The race lands exactly now: after the live GET, before the delete —
	// and only at the delete boundary of the modeled object (batch tests
	// delete several objects).
	if f.raceHook != nil && hookForAction(action, f.raceTarget) {
		hook := f.raceHook
		f.raceHook = nil
		hook(f.dc.Tracker())
	}

	gvr := action.GetResource()
	obj, err := f.dc.Tracker().Get(gvr, da.GetNamespace(), da.GetName())
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Gone between GET and DELETE: delegate; the tracker 404s.
			return false, nil, nil
		}
		return true, nil, err
	}
	acc, err := meta.Accessor(obj)
	if err != nil {
		return true, nil, err
	}
	pre := da.GetDeleteOptions().Preconditions
	if pre != nil {
		if pre.UID != nil && acc.GetUID() != *pre.UID {
			return true, nil, apierrors.NewConflict(
				schema.GroupResource{Group: gvr.Group, Resource: gvr.Resource},
				da.GetName(),
				errors.New("the UID in the precondition does not match the object's current UID. The object might have been deleted and then recreated"),
			)
		}
		if pre.ResourceVersion != nil && acc.GetResourceVersion() != *pre.ResourceVersion {
			return true, nil, apierrors.NewConflict(
				schema.GroupResource{Group: gvr.Group, Resource: gvr.Resource},
				da.GetName(),
				errors.New("the ResourceVersion in the precondition does not match the object's current ResourceVersion. The object might have been modified"),
			)
		}
	}
	return false, nil, nil // preconditions satisfied: perform the real delete
}

func (f *apiserverLawFake) get(t *testing.T, gvr schema.GroupVersionResource, ns, name string) *unstructured.Unstructured {
	t.Helper()
	obj, err := f.dc.Tracker().Get(gvr, ns, name)
	if err != nil {
		t.Fatalf("get %s/%s: %v", ns, name, err)
	}
	return obj.(*unstructured.Unstructured)
}

func (f *apiserverLawFake) hasDelete() bool { return len(f.deletes) > 0 }

// swapUIDHook replaces the object's UID at the delete boundary (the
// GET→DELETE replacement race: same name, new UID).
func swapUIDHook(gvr schema.GroupVersionResource, ns, name, newUID string) func(clienttesting.ObjectTracker) {
	return func(tracker clienttesting.ObjectTracker) {
		obj, err := tracker.Get(gvr, ns, name)
		if err != nil {
			return
		}
		acc, _ := meta.Accessor(obj)
		acc.SetUID(types.UID(newUID))
		_ = tracker.Update(gvr, obj, ns)
	}
}

// deleteActionOn returns a hook-fire predicate: fire only when the delete
// action targets the named object (batch tests delete several objects; the
// race must land at the boundary of the modeled object).
func hookForAction(action clienttesting.Action, name string) bool {
	da, ok := action.(clienttesting.DeleteAction)
	return ok && da.GetName() == name
}

// bumpRVHook updates only the object's resourceVersion at the delete boundary
// (the same-UID update race: eligibility/safety state can change invisibly to
// a UID-only precondition).
func bumpRVHook(gvr schema.GroupVersionResource, ns, name string) func(clienttesting.ObjectTracker) {
	return func(tracker clienttesting.ObjectTracker) {
		obj, err := tracker.Get(gvr, ns, name)
		if err != nil {
			return
		}
		acc, _ := meta.Accessor(obj)
		acc.SetResourceVersion("rv-BUMPED")
		_ = tracker.Update(gvr, obj, ns)
	}
}

func newDeleter(t *testing.T, f *apiserverLawFake) *PolicyReconciler {
	t.Helper()
	return &PolicyReconciler{
		dynamicClient: f.dc,
		safetyGate:    safety.NewGate(safety.Config{}),
		logger:        testLogger(),
	}
}

func mustRefused(t *testing.T, err error, wantReason string) *DeletionRefusedError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a *DeletionRefusedError (%s), got nil", wantReason)
	}
	refused, ok := isDeletionRefused(err)
	if !ok {
		t.Fatalf("expected *DeletionRefusedError (%s), got %T: %v", wantReason, err, err)
	}
	if refused.Reason != wantReason {
		t.Fatalf("refusal reason = %q, want %q", refused.Reason, wantReason)
	}
	return refused
}

func assertSurvives(t *testing.T, f *apiserverLawFake, gvr schema.GroupVersionResource, ns, name, wantUID string) {
	t.Helper()
	live := f.get(t, gvr, ns, name)
	if string(live.GetUID()) != wantUID {
		t.Fatalf("object %s/%s was mutated: uid=%s, want %s", ns, name, live.GetUID(), wantUID)
	}
}

// ── THE CORE RACE: replacement lands between the live GET and the DELETE ──

func TestPerformResourceDeletion_ReplacementBetweenGetAndDelete_Refused(t *testing.T) {
	for _, tc := range []struct {
		name    string
		gvr     schema.GroupVersionResource
		ns      string
		policy  func() *gcapi.ZenCleanerPolicy
		fixture func(name, uid string) *unstructured.Unstructured
	}{
		{"namespaced", cmGVR, nsScope, func() *gcapi.ZenCleanerPolicy { return cmPolicy() },
			func(name, uid string) *unstructured.Unstructured { return cmFixture(name, uid) }},
		{"cluster-scoped", pcsGVR, "", func() *gcapi.ZenCleanerPolicy { return clusterPolicy() },
			func(name, uid string) *unstructured.Unstructured { return clusterFixture(name, uid) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pol := tc.policy()
			live := tc.fixture("target", "uid-ORIGINAL")
			f := newAPIServerLawFake(t, live)
			f.raceHook = swapUIDHook(tc.gvr, tc.ns, "target", "uid-REPLACEMENT")
			f.raceTarget = "target"
			r := newDeleter(t, f)

			observed := tc.fixture("target", "uid-ORIGINAL") // decision made on the original
			outcome, err := r.performResourceDeletion(context.Background(), observed, tc.gvr,
				buildDeleteOptions(pol), pol)
			if err != nil {
				t.Fatalf("precondition conflict must not be an API failure, got: %v", err)
			}
			if outcome.Outcome != OutcomePreconditionConflict || outcome.Reason != safety.ReasonPreconditionConflict {
				t.Fatalf("outcome = %+v, want precondition_conflict", outcome)
			}

			// The replacement survives untouched.
			assertSurvives(t, f, tc.gvr, tc.ns, "target", "uid-REPLACEMENT")

			// The DELETE was issued exactly once and CARRIED the decision-time
			// UID precondition (never a replacement UID).
			if len(f.deletes) != 1 {
				t.Fatalf("expected exactly one delete attempt, got %d", len(f.deletes))
			}
			pre := f.deletes[0].GetDeleteOptions().Preconditions
			if pre == nil || pre.UID == nil || *pre.UID != "uid-ORIGINAL" {
				t.Fatalf("delete must carry the decision-time UID precondition, got %+v", pre)
			}
		})
	}
}

// The same-UID update race: exclusion label/eligibility can change after the
// GET; the resourceVersion precondition refuses atomically at the API server.
func TestPerformResourceDeletion_SameUIDUpdateBetweenEvalAndDelete_Conflict(t *testing.T) {
	live := cmFixture("target", "uid-SAME")
	f := newAPIServerLawFake(t, live)
	f.raceHook = bumpRVHook(cmGVR, nsScope, "target")
	f.raceTarget = "target"
	r := newDeleter(t, f)

	observed := cmFixture("target", "uid-SAME")
	outcome, err := r.performResourceDeletion(context.Background(), observed, cmGVR,
		buildDeleteOptions(cmPolicy()), cmPolicy())
	if err != nil {
		t.Fatalf("precondition conflict must not be an API failure, got: %v", err)
	}
	if outcome.Outcome != OutcomePreconditionConflict {
		t.Fatalf("outcome = %+v, want precondition_conflict", outcome)
	}
	assertSurvives(t, f, cmGVR, nsScope, "target", "uid-SAME")
	if len(f.deletes) != 1 {
		t.Fatalf("expected exactly one delete attempt, got %d", len(f.deletes))
	}
}

// deleteResource surfaces the conflict as a typed refusal (never nil, never
// a mechanism failure), and the backoff wrapper does NOT retry refusals.
func TestDeleteResourceAndBackoff_PreconditionConflict_TypedRefusalNoRetry(t *testing.T) {
	live := cmFixture("target", "uid-ORIGINAL")
	f := newAPIServerLawFake(t, live)
	f.raceHook = swapUIDHook(cmGVR, nsScope, "target", "uid-REPLACEMENT")
	f.raceTarget = "target"
	r := newDeleter(t, f)

	pol := cmPolicy()
	err := r.deleteResource(context.Background(), cmFixture("target", "uid-ORIGINAL"), pol, ratelimiter.NewRateLimiter(100))
	refused := mustRefused(t, err, safety.ReasonPreconditionConflict)
	if refused.Reason != safety.ReasonPreconditionConflict {
		t.Fatalf("refusal reason = %q", refused.Reason)
	}
	if len(f.deletes) != 1 {
		t.Fatalf("refusal must not be retried by the backoff wrapper: %d delete attempts", len(f.deletes))
	}
}

// ── Decision-time UID law ──

// The DELETE carries the UID of the object ON WHICH THE DECISION WAS MADE,
// and the options the caller passed in are never mutated.
func TestPerformResourceDeletion_MatchingUIDDeletes_CarriesPreconditionsWithoutMutatingOptions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		gvr     schema.GroupVersionResource
		ns      string
		policy  func() *gcapi.ZenCleanerPolicy
		fixture func(name, uid string) *unstructured.Unstructured
	}{
		{"namespaced", cmGVR, nsScope, func() *gcapi.ZenCleanerPolicy { return cmPolicy() },
			func(name, uid string) *unstructured.Unstructured { return cmFixture(name, uid) }},
		{"cluster-scoped", pcsGVR, "", func() *gcapi.ZenCleanerPolicy { return clusterPolicy() },
			func(name, uid string) *unstructured.Unstructured { return clusterFixture(name, uid) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			live := tc.fixture("target", "uid-MATCH")
			f := newAPIServerLawFake(t, live)
			r := newDeleter(t, f)

			observed := tc.fixture("target", "uid-MATCH")
			opts := &metav1.DeleteOptions{GracePeriodSeconds: int64Ptr(42), PropagationPolicy: deletionPropagationPtr(metav1.DeletePropagationForeground)}
			outcome, err := r.performResourceDeletion(context.Background(), observed, tc.gvr, opts, tc.policy())
			if err != nil {
				t.Fatalf("matching-UID delete failed: %v", err)
			}
			if outcome.Outcome != OutcomeDeleted {
				t.Fatalf("outcome = %+v, want deleted", outcome)
			}

			// The delete action preserved the caller's grace period and
			// propagation policy AND carried both preconditions.
			if len(f.deletes) != 1 {
				t.Fatalf("expected exactly one delete, got %d", len(f.deletes))
			}
			got := f.deletes[0].GetDeleteOptions()
			if got.GracePeriodSeconds == nil || *got.GracePeriodSeconds != 42 {
				t.Fatalf("grace period not preserved: %+v", got)
			}
			if got.PropagationPolicy == nil || *got.PropagationPolicy != metav1.DeletePropagationForeground {
				t.Fatalf("propagation policy not preserved: %+v", got)
			}
			if got.Preconditions == nil || got.Preconditions.UID == nil || *got.Preconditions.UID != "uid-MATCH" {
				t.Fatalf("UID precondition missing: %+v", got)
			}
			if got.Preconditions.ResourceVersion == nil || *got.Preconditions.ResourceVersion != live.GetResourceVersion() {
				t.Fatalf("resourceVersion precondition missing or wrong: %+v", got)
			}

			// The caller's options pointer was NOT mutated.
			if opts.Preconditions != nil {
				t.Fatal("shared delete options were mutated with preconditions")
			}

			// The object is gone.
			if _, err := f.dc.Tracker().Get(tc.gvr, tc.ns, "target"); !apierrors.IsNotFound(err) {
				t.Fatalf("object should be deleted, get err: %v", err)
			}
		})
	}
}

// An empty observed UID refuses: no delete-by-name fallback, ever.
func TestPerformResourceDeletion_EmptyObservedUIDRefused(t *testing.T) {
	live := cmFixture("target", "uid-LIVE")
	f := newAPIServerLawFake(t, live)
	r := newDeleter(t, f)

	observed := cmFixture("target", "") // no UID on the decision observation
	outcome, err := r.performResourceDeletion(context.Background(), observed, cmGVR,
		buildDeleteOptions(cmPolicy()), cmPolicy())
	if err != nil {
		t.Fatalf("empty-UID refusal must not be an API failure, got: %v", err)
	}
	if outcome.Outcome != OutcomeRefused || outcome.Reason != safety.ReasonMissingUID {
		t.Fatalf("outcome = %+v, want refused/missing_uid", outcome)
	}
	if f.hasDelete() {
		t.Fatal("a DELETE was issued for an object with no observable UID")
	}
	assertSurvives(t, f, cmGVR, nsScope, "target", "uid-LIVE")
}

// Replacement BEFORE the GET (stale informer cache): refused with the
// ErrStaleUID sentinel cause, no delete issued.
func TestPerformResourceDeletion_StaleBeforeGET_RefusedWithSentinel(t *testing.T) {
	live := cmFixture("target", "uid-REPLACEMENT")
	f := newAPIServerLawFake(t, live)
	r := newDeleter(t, f)

	observed := cmFixture("target", "uid-ORIGINAL") // stale snapshot
	err := r.deleteResource(context.Background(), observed, cmPolicy(), ratelimiter.NewRateLimiter(100))
	refused := mustRefused(t, err, safety.ReasonStaleUID)
	if !errors.Is(err, ErrStaleUID) {
		t.Fatalf("stale refusal must carry the ErrStaleUID sentinel, got: %v", err)
	}
	_ = refused
	if f.hasDelete() {
		t.Fatal("a DELETE was issued despite the stale-observation refusal")
	}
	assertSurvives(t, f, cmGVR, nsScope, "target", "uid-REPLACEMENT")
}

// Object vanished before the call: idempotent no-op, no DELETE issued.
func TestPerformResourceDeletion_AlreadyAbsent(t *testing.T) {
	f := newAPIServerLawFake(t) // empty cluster
	r := newDeleter(t, f)
	observed := cmFixture("gone", "uid-ORIGINAL")
	outcome, err := r.performResourceDeletion(context.Background(), observed, cmGVR,
		buildDeleteOptions(cmPolicy()), cmPolicy())
	if err != nil {
		t.Fatalf("already-absent must be a no-op success, got: %v", err)
	}
	if outcome.Outcome != OutcomeAlreadyAbsent {
		t.Fatalf("outcome = %+v, want already_absent", outcome)
	}
	if f.hasDelete() {
		t.Fatal("a DELETE was issued for an absent object")
	}
}

// Object removed between the precondition GET and the DELETE: idempotent
// ALREADY_ABSENT, never a fake deletion effect.
func TestPerformResourceDeletion_AbsentBetweenGetAndDelete_Idempotent(t *testing.T) {
	live := cmFixture("target", "uid-ORIGINAL")
	f := newAPIServerLawFake(t, live)
	f.raceTarget = "target"
	f.raceHook = func(tracker clienttesting.ObjectTracker) {
		_ = tracker.Delete(cmGVR, nsScope, "target")
	}
	r := newDeleter(t, f)
	outcome, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-ORIGINAL"), cmGVR,
		buildDeleteOptions(cmPolicy()), cmPolicy())
	if err != nil {
		t.Fatalf("got err: %v", err)
	}
	if outcome.Outcome != OutcomeAlreadyAbsent {
		t.Fatalf("outcome = %+v, want already_absent", outcome)
	}
}

// ── Delete-time re-evaluation of safety and eligibility on the LIVE object ──

// A same-UID update that adds the exclusion label before the delete gate is
// refused: the gate must evaluate the CURRENT object, not the observation.
func TestPerformResourceDeletion_ExclusionLabelAddedBeforeDelete_RefusedOnLive(t *testing.T) {
	live := cmFixture("target", "uid-SAME", func(o *unstructured.Unstructured) {
		o.SetLabels(map[string]string{safety.ExclusionLabelKey: "true"})
	})
	f := newAPIServerLawFake(t, live)
	r := newDeleter(t, f)

	observed := cmFixture("target", "uid-SAME") // observation WITHOUT the label
	outcome, err := r.performResourceDeletion(context.Background(), observed, cmGVR,
		buildDeleteOptions(cmPolicy()), cmPolicy())
	if err != nil {
		t.Fatalf("gate refusal must not be an API failure, got: %v", err)
	}
	if outcome.Outcome != OutcomeRefused || outcome.Reason != safety.ReasonExcludedLabel {
		t.Fatalf("outcome = %+v, want refused/excluded_label", outcome)
	}
	if f.hasDelete() {
		t.Fatal("a DELETE was issued for an object that carries the exclusion label NOW")
	}
	assertSurvives(t, f, cmGVR, nsScope, "target", "uid-SAME")
}

// A same-UID update that breaks the deletion decision (TTL field removed) is
// refused and requeued under current policy.
func TestPerformResourceDeletion_EligibilityChangedOnLive_Refused(t *testing.T) {
	live := cmFixture("target", "uid-SAME", func(o *unstructured.Unstructured) {
		// TTL anchor field gone: the live object no longer satisfies the
		// policy TTL (the earlier decision no longer applies).
		unstructured.RemoveNestedField(o.Object, "status", "completedAt")
	})
	f := newAPIServerLawFake(t, live)
	r := newDeleter(t, f)

	observed := cmFixture("target", "uid-SAME") // TTL-expired at decision time
	outcome, err := r.performResourceDeletion(context.Background(), observed, cmGVR,
		buildDeleteOptions(cmPolicy()), cmPolicy())
	if err != nil {
		t.Fatalf("eligibility refusal must not be an API failure, got: %v", err)
	}
	if outcome.Outcome != OutcomeRefused || outcome.Reason != safety.ReasonNoLongerEligible {
		t.Fatalf("outcome = %+v, want refused/no_longer_eligible", outcome)
	}
	if f.hasDelete() {
		t.Fatal("a DELETE was issued for an object whose eligibility changed")
	}
	assertSurvives(t, f, cmGVR, nsScope, "target", "uid-SAME")
}

// A same-UID label change that moves the object out of the policy's selector
// scope is refused before the delete.
func TestPerformResourceDeletion_SelectorNoLongerMatchesOnLive_Refused(t *testing.T) {
	live := cmFixture("target", "uid-SAME", func(o *unstructured.Unstructured) {
		o.SetLabels(map[string]string{"app": "keepme"})
	})
	f := newAPIServerLawFake(t, live)
	r := newDeleter(t, f)

	pol := cmPolicy(func(p *gcapi.ZenCleanerPolicy) {
		p.Spec.TargetResource.LabelSelector = &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "cleanme"},
		}
	})
	observed := cmFixture("target", "uid-SAME", func(o *unstructured.Unstructured) {
		o.SetLabels(map[string]string{"app": "cleanme"})
	})
	outcome, err := r.performResourceDeletion(context.Background(), observed, cmGVR,
		buildDeleteOptions(pol), pol)
	if err != nil {
		t.Fatalf("selector refusal must not be an API failure, got: %v", err)
	}
	if outcome.Outcome != OutcomeRefused || outcome.Reason != safety.ReasonNoLongerEligible {
		t.Fatalf("outcome = %+v, want refused/no_longer_eligible", outcome)
	}
	if f.hasDelete() {
		t.Fatal("a DELETE was issued for an object that left the policy scope")
	}
}

// Server-side failures other than conflicts propagate as errors — no fake
// success, no refusal classification.
func TestPerformResourceDeletion_ServerErrorPropagates(t *testing.T) {
	live := cmFixture("target", "uid-MATCH")
	f := newAPIServerLawFake(t, live)
	f.deleteErr = apierrors.NewInternalError(errors.New("boom"))
	r := newDeleter(t, f)

	_, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-MATCH"), cmGVR,
		buildDeleteOptions(cmPolicy()), cmPolicy())
	if err == nil {
		t.Fatal("expected the API error to propagate")
	}
	if _, isRefusal := isDeletionRefused(err); isRefusal {
		t.Fatal("an API failure must not be classified as a refusal")
	}
	var refused *DeletionRefusedError
	if errors.As(err, &refused) {
		t.Fatal("an API failure must not be a refusal")
	}
}

// ── Truthful batch accounting: refusals never count as deletions ──

func TestDeleteBatchShared_RefusalsNotCounted(t *testing.T) {
	protected := cmFixture("protected", "uid-PROT", func(o *unstructured.Unstructured) {
		o.SetLabels(map[string]string{safety.ExclusionLabelKey: "true"})
	})
	// The race fixture lives in the tracker as the ORIGINAL; the hook swaps
	// its UID at the delete boundary (a replacement landed meanwhile).
	fRace := cmFixture("race", "uid-ORIGINAL")
	f := newAPIServerLawFake(t,
		cmFixture("victim", "uid-VICTIM"),
		protected,
		fRace,
	)
	f.raceHook = swapUIDHook(cmGVR, nsScope, "race", "uid-REPLACED")
	f.raceTarget = "race"
	r := newDeleter(t, f)

	pol := cmPolicy()
	batch := []*unstructured.Unstructured{
		cmFixture("victim", "uid-VICTIM"),
		protected,
		fRace,
	}
	outcomes := make(map[string]DeletionOutcomeRecord, len(batch))
	n, errs := deleteBatchShared(context.Background(), batch, pol, ratelimiter.NewRateLimiter(100), map[string]string{}, r, outcomes)

	if n != 1 {
		t.Fatalf("deletedCount = %d, want 1 (only the real deletion)", n)
	}
	if len(errs) != 0 {
		t.Fatalf("refusals must not surface as deletion failures: %v", errs)
	}
	if len(outcomes) != 2 {
		t.Fatalf("expected 2 recorded non-deletion outcomes, got %d: %+v", len(outcomes), outcomes)
	}
	if rec, ok := outcomes[objectKey(protected)]; !ok || rec.Reason != safety.ReasonExcludedLabel {
		t.Fatalf("protected outcome missing/wrong: %+v", outcomes[objectKey(protected)])
	}
	if rec, ok := outcomes[objectKey(fRace)]; !ok || rec.Outcome != OutcomePreconditionConflict {
		t.Fatalf("conflict outcome missing/wrong: %+v", outcomes[objectKey(fRace)])
	}
}

// ── Receipt classification of refusals: SKIPPED, never FAILED/SUCCEEDED ──

func TestExecuteWithReceipt_RefusalsClassifiedAsSkipped(t *testing.T) {
	protected := cmFixture("protected", "uid-PROT", func(o *unstructured.Unstructured) {
		o.SetLabels(map[string]string{safety.ExclusionLabelKey: "true"})
	})
	f := newAPIServerLawFake(t, cmFixture("victim", "uid-VICTIM"), protected)
	r := newDeleter(t, f)

	pol := cmPolicy()
	batch := []*unstructured.Unstructured{
		cmFixture("victim", "uid-VICTIM"),
		protected,
	}
	n, errs, receipt := r.ExecuteWithReceipt(context.Background(), batch, pol, ratelimiter.NewRateLimiter(100), map[string]string{})
	if n != 1 {
		t.Fatalf("deletedCount = %d, want 1", n)
	}
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if receipt == nil || len(receipt.Results) != 2 {
		t.Fatalf("receipt = %+v", receipt)
	}
	classes := map[string]ObjectResultClass{}
	for _, res := range receipt.Results {
		classes[res.Name] = res.Class
	}
	if classes["victim"] != ResultSucceeded {
		t.Fatalf("victim = %v, want SUCCEEDED", classes["victim"])
	}
	if classes["protected"] != ResultSkippedPreconditionMismatch {
		t.Fatalf("protected = %v, want SKIPPED_PRECONDITION_MISMATCH (a refusal is a non-deletion, never FAILED and never a success)", classes["protected"])
	}
	if receipt.OverallStatus != "PARTIAL" {
		t.Fatalf("overall = %s, want PARTIAL", receipt.OverallStatus)
	}
	// The protected object survives.
	assertSurvives(t, f, cmGVR, nsScope, "protected", "uid-PROT")
}

func TestExecuteWithReceipt_PreconditionConflictClassifiedAsSkipped(t *testing.T) {
	f := newAPIServerLawFake(t, cmFixture("race", "uid-ORIGINAL"))
	f.raceHook = swapUIDHook(cmGVR, nsScope, "race", "uid-REPLACED")
	f.raceTarget = "race"
	r := newDeleter(t, f)

	pol := cmPolicy()
	batch := []*unstructured.Unstructured{cmFixture("race", "uid-ORIGINAL")}
	_, errs, receipt := r.ExecuteWithReceipt(context.Background(), batch, pol, ratelimiter.NewRateLimiter(100), map[string]string{})
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if receipt == nil || len(receipt.Results) != 1 || receipt.Results[0].Class != ResultSkippedPreconditionMismatch {
		t.Fatalf("receipt = %+v, want SKIPPED_PRECONDITION_MISMATCH", receipt)
	}
	if receipt.OverallStatus != "PARTIAL" {
		t.Fatalf("overall = %s, want PARTIAL", receipt.OverallStatus)
	}
	assertSurvives(t, f, cmGVR, nsScope, "race", "uid-REPLACED")
}

// ── helpers ──

func deletionPropagationPtr(p metav1.DeletionPropagation) *metav1.DeletionPropagation { return &p }

// ── Independent-review follow-ups (2026-11-11) ────────────────────────────

// A live object without a resourceVersion cannot have its evaluated state
// preconditioned: refuse instead of sending an unenforceable precondition
// (real API-server objects always carry one).
func TestPerformResourceDeletion_EmptyLiveResourceVersionRefused(t *testing.T) {
	live := cmFixture("target", "uid-SAME")
	unstructured.RemoveNestedField(live.Object, "metadata", "resourceVersion")
	f := newAPIServerLawFake(t, live)
	r := newDeleter(t, f)

	outcome, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-SAME"), cmGVR,
		buildDeleteOptions(cmPolicy()), cmPolicy())
	if err != nil {
		t.Fatalf("empty-RV refusal must not be an API failure, got: %v", err)
	}
	if outcome.Outcome != OutcomeRefused || outcome.Reason != safety.ReasonMissingResourceVersion {
		t.Fatalf("outcome = %+v, want refused/missing_resource_version", outcome)
	}
	if f.hasDelete() {
		t.Fatal("a DELETE was issued without an enforceable resourceVersion precondition")
	}
	assertSurvives(t, f, cmGVR, nsScope, "target", "uid-SAME")
}

// A nil governing policy cannot be re-evaluated: fail closed (the safety
// gate still ran with the default behavior and would still deny protected
// objects).
func TestPerformResourceDeletion_NilPolicyRefused(t *testing.T) {
	live := cmFixture("target", "uid-SAME")
	f := newAPIServerLawFake(t, live)
	r := newDeleter(t, f)

	outcome, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-SAME"), cmGVR,
		buildDeleteOptions(cmPolicy()), nil)
	if err != nil {
		t.Fatalf("nil-policy refusal must not be an API failure, got: %v", err)
	}
	if outcome.Outcome != OutcomeRefused || outcome.Reason != safety.ReasonNoLongerEligible {
		t.Fatalf("outcome = %+v, want refused/no_longer_eligible", outcome)
	}
	if f.hasDelete() {
		t.Fatal("a DELETE was issued without a governing policy context")
	}
	assertSurvives(t, f, cmGVR, nsScope, "target", "uid-SAME")
}

// PRESERVATION LAW: caller-supplied preconditions that AGREE with the
// decision-time UID / evaluated RV are carried and the delete proceeds.
func TestPerformResourceDeletion_CallerPreconditionsMatching_CarriedAndDeleted(t *testing.T) {
	live := cmFixture("target", "uid-MATCH")
	f := newAPIServerLawFake(t, live)
	r := newDeleter(t, f)

	opts := &metav1.DeleteOptions{Preconditions: &metav1.Preconditions{
		UID:             uidPtr(types.UID("uid-MATCH")),
		ResourceVersion: strPtr("rv-1"),
	}}
	outcome, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-MATCH"), cmGVR, opts, cmPolicy())
	if err != nil {
		t.Fatalf("matching caller preconditions must not block the delete: %v", err)
	}
	if outcome.Outcome != OutcomeDeleted {
		t.Fatalf("outcome = %+v, want deleted", outcome)
	}
	got := f.deletes[0].GetDeleteOptions()
	if got.Preconditions == nil || got.Preconditions.UID == nil || *got.Preconditions.UID != "uid-MATCH" ||
		got.Preconditions.ResourceVersion == nil || *got.Preconditions.ResourceVersion != "rv-1" {
		t.Fatalf("preconditions not carried correctly: %+v", got.Preconditions)
	}
	// The caller's options pointer was not mutated.
	if opts.Preconditions.UID == nil || *opts.Preconditions.UID != "uid-MATCH" {
		t.Fatal("caller options were mutated")
	}
}

// A caller-supplied UID precondition that CONFLICTS with the decision-time
// UID fails closed with an error — never overwritten, never discarded.
func TestPerformResourceDeletion_CallerPreconditionsConflictingUID_Refused(t *testing.T) {
	live := cmFixture("target", "uid-MATCH")
	f := newAPIServerLawFake(t, live)
	r := newDeleter(t, f)

	opts := &metav1.DeleteOptions{Preconditions: &metav1.Preconditions{
		UID: uidPtr(types.UID("uid-DIFFERENT")),
	}}
	_, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-MATCH"), cmGVR, opts, cmPolicy())
	if err == nil {
		t.Fatal("conflicting caller UID precondition must fail closed, got nil")
	}
	if _, isRefusal := isDeletionRefused(err); isRefusal {
		t.Fatal("a caller-contract conflict is an error, not a safety-state refusal")
	}
	if !strings.Contains(err.Error(), "conflicts with the decision-time UID") {
		t.Fatalf("error must name the conflict: %v", err)
	}
	if f.hasDelete() {
		t.Fatal("a DELETE was issued with a conflicting caller precondition")
	}
	// The caller's options were not rewritten.
	if *opts.Preconditions.UID != "uid-DIFFERENT" {
		t.Fatal("caller precondition was mutated")
	}
	assertSurvives(t, f, cmGVR, nsScope, "target", "uid-MATCH")
}

// A caller-supplied resourceVersion precondition that CONFLICTS with the
// evaluated live RV fails closed with an error.
func TestPerformResourceDeletion_CallerPreconditionsConflictingRV_Refused(t *testing.T) {
	live := cmFixture("target", "uid-MATCH")
	f := newAPIServerLawFake(t, live)
	r := newDeleter(t, f)

	opts := &metav1.DeleteOptions{Preconditions: &metav1.Preconditions{
		ResourceVersion: strPtr("rv-OLD"),
	}}
	_, err := r.performResourceDeletion(context.Background(), cmFixture("target", "uid-MATCH"), cmGVR, opts, cmPolicy())
	if err == nil {
		t.Fatal("conflicting caller RV precondition must fail closed, got nil")
	}
	if !strings.Contains(err.Error(), "conflicts with the evaluated live resourceVersion") {
		t.Fatalf("error must name the conflict: %v", err)
	}
	if f.hasDelete() {
		t.Fatal("a DELETE was issued with a conflicting caller precondition")
	}
	assertSurvives(t, f, cmGVR, nsScope, "target", "uid-MATCH")
}

// TRUTHFUL ABSENCE ACCOUNTING: an already-absent object is NOT counted as a
// deletion, produces no error, and does not degrade the batch.
func TestDeleteBatchShared_AbsentNotCountedAsDeletion(t *testing.T) {
	f := newAPIServerLawFake(t, cmFixture("victim", "uid-VICTIM"))
	r := newDeleter(t, f)

	pol := cmPolicy()
	batch := []*unstructured.Unstructured{
		cmFixture("victim", "uid-VICTIM"), // real deletion
		cmFixture("ghost", "uid-GHOST"),   // already absent
	}
	n, errs := deleteBatchShared(context.Background(), batch, pol, ratelimiter.NewRateLimiter(100), map[string]string{}, r, nil)
	if n != 1 {
		t.Fatalf("deletedCount = %d, want 1 (absence is not a deletion)", n)
	}
	if len(errs) != 0 {
		t.Fatalf("absence must not surface as a failure: %v", errs)
	}
}

func uidPtr(u types.UID) *types.UID { return &u }
func strPtr(s string) *string       { return &s }

// F1 (independent review): an object that vanishes BETWEEN the receipt
// pre-read and the delete path must classify ALREADY_ABSENT ("nothing
// deleted by this run") — never SUCCEEDED/"effect confirmed" for a deletion
// this run did not perform.
func TestExecuteWithReceipt_AbsenceBetweenPreReadAndDelete_ClassifiedAlreadyAbsent(t *testing.T) {
	f := newAPIServerLawFake(t, cmFixture("vanish", "uid-V"))
	gets := 0
	f.dc.PrependReactor("get", "configmaps", func(action clienttesting.Action) (bool, runtime.Object, error) {
		ga, ok := action.(clienttesting.GetAction)
		if !ok || ga.GetName() != "vanish" {
			return false, nil, nil
		}
		gets++
		if gets == 2 { // the live GET inside performResourceDeletion
			_ = f.dc.Tracker().Delete(cmGVR, nsScope, "vanish")
		}
		return false, nil, nil // delegate to the tracker
	})
	r := newDeleter(t, f)

	pol := cmPolicy()
	batch := []*unstructured.Unstructured{cmFixture("vanish", "uid-V")}
	outcomes := make(map[string]DeletionOutcomeRecord, 1)
	n, errs, receipt := r.ExecuteWithReceipt(context.Background(), batch, pol, ratelimiter.NewRateLimiter(100), map[string]string{})
	_ = outcomes
	if n != 0 {
		t.Fatalf("deletedCount = %d, want 0 (this run deleted nothing)", n)
	}
	if len(errs) != 0 {
		t.Fatalf("absence must not surface as a failure: %v", errs)
	}
	if receipt == nil || len(receipt.Results) != 1 || receipt.Results[0].Class != ResultAlreadyAbsent {
		t.Fatalf("receipt = %+v, want ALREADY_ABSENT", receipt)
	}
	if !containsFold(receipt.Results[0].Detail, "nothing deleted by this run") {
		t.Fatalf("detail must be truthful about this run: %q", receipt.Results[0].Detail)
	}
}

// F2 (independent review): a k8s NotFound escaping a deleter implementation
// into the backoff wrapper surfaces as the TYPED absence signal — terminal,
// non-retried, and never a bare nil that batch accounting would count as a
// performed deletion.
type notFoundStatusDeleter struct{ attempts int }

func (d *notFoundStatusDeleter) DeleteResourceWithContext(ctx context.Context, resource *unstructured.Unstructured, policy *gcapi.ZenCleanerPolicy, rateLimiter *ratelimiter.RateLimiter) error {
	d.attempts++
	return apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, "x")
}

func TestDeleteResourceWithBackoffShared_NotFoundReturnsTypedAbsence(t *testing.T) {
	d := &notFoundStatusDeleter{}
	err := deleteResourceWithBackoffShared(context.Background(), cmFixture("x", "uid-X"), cmPolicy(), ratelimiter.NewRateLimiter(100), d, nil)
	if !errors.Is(err, ErrAlreadyAbsent) {
		t.Fatalf("NotFound through the wrapper must surface the typed absence signal, got: %v", err)
	}
	if d.attempts != 1 {
		t.Fatalf("absence must not be retried, attempts = %d", d.attempts)
	}
}
