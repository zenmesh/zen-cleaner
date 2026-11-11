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

// SUPPORT2-033 §5 (selector fail-closed): invalid selectors must never
// broaden the observed or deleted scope. These tests exercise the REAL
// informer-construction seam, the real list/watch options, and the final
// in-memory match — not just the helper.

import (
	"context"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clienttesting "k8s.io/client-go/testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	ctrl "sigs.k8s.io/controller-runtime/pkg/reconcile"

	gcapi "github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
)

func reconcileRequest(pol *gcapi.ZenCleanerPolicy) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Namespace: pol.Namespace, Name: pol.Name}}
}

func selectorReconciler(t *testing.T) (*PolicyReconciler, *dynamicfake.FakeDynamicClient) {
	t.Helper()
	sch := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(sch); err != nil {
		t.Fatal(err)
	}
	if err := gcapi.AddToScheme(sch); err != nil {
		t.Fatal(err)
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(sch, map[schema.GroupVersionResource]string{
		cmGVR: "ConfigMapList",
	})
	c := fake.NewClientBuilder().WithScheme(sch).Build()
	r := NewPolicyReconcilerWithRESTMapper(c, sch, dyn, nil, NewStatusUpdater(dyn), NewEventRecorder(nil), config.NewControllerConfig())
	return r, dyn
}

func invalidSelectorPolicy() *gcapi.ZenCleanerPolicy {
	return &gcapi.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "pol-ns", Name: "bad-selector"},
		Spec: gcapi.ZenCleanerPolicySpec{
			TargetResource: gcapi.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  "app",
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "app", Operator: "Between", Values: []string{"a", "b"}},
					},
				},
			},
			TTL: gcapi.TTLSpec{
				RelativeTo:   "status.completedAt",
				SecondsAfter: int64Ptr(3600),
			},
		},
	}
}

// An invalid selector must fail closed at the informer seam: NO informer is
// created, no list/watch reflector is started (the only List is the
// pre-existing bounded capability probe).
func TestGetOrCreateResourceInformer_InvalidSelectorFailsClosed(t *testing.T) {
	r, dyn := selectorReconciler(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := r.getOrCreateResourceInformer(ctx, invalidSelectorPolicy())
	if err == nil {
		t.Fatal("invalid selector must fail informer creation (fail closed), got nil")
	}
	if !strings.Contains(err.Error(), "label selector") {
		t.Fatalf("error must name the selector problem, got: %v", err)
	}

	// No unintended watch: the reflector was never started.
	for _, action := range dyn.Actions() {
		if action.GetVerb() == "watch" {
			t.Fatalf("a WATCH was started for a policy with an invalid selector: %+v", action)
		}
	}
	// No informer registered.
	r.resourceInformersMu.RLock()
	n := len(r.resourceInformers)
	r.resourceInformersMu.RUnlock()
	if n != 0 {
		t.Fatalf("informer stored despite invalid selector: %d", n)
	}
}

// A VALID selector still produces the deliberately UNFILTERED shared
// informer (documented behavior: one informer per GVR+namespace, policy
// selectors applied per-object at evaluation) — and no selector is pushed
// down to the list/watch.
func TestGetOrCreateResourceInformer_ValidSelector_StaysUnfiltered(t *testing.T) {
	r, dyn := selectorReconciler(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pol := invalidSelectorPolicy()
	pol.Name = "good-selector"
	pol.Spec.TargetResource.LabelSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "cleanme"},
	}

	informer, err := r.getOrCreateResourceInformer(ctx, pol)
	if err != nil {
		t.Fatalf("valid selector must create the informer: %v", err)
	}
	if informer == nil {
		t.Fatal("informer is nil")
	}

	deadline := time.Now().Add(10 * time.Second)
	var listRestrictions *clienttesting.ListRestrictions
	var watchRestrictions *clienttesting.WatchRestrictions
	for time.Now().Before(deadline) {
		for _, action := range dyn.Actions() {
			if action.GetResource().Resource != "configmaps" {
				continue
			}
			switch action.GetVerb() {
			case "list":
				if la, ok := action.(clienttesting.ListAction); ok {
					o := la.GetListRestrictions()
					listRestrictions = &o
				}
			case "watch":
				if wa, ok := action.(clienttesting.WatchAction); ok {
					o := wa.GetWatchRestrictions()
					watchRestrictions = &o
				}
			}
		}
		if listRestrictions != nil && watchRestrictions != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if listRestrictions == nil {
		t.Fatal("expected a list from the informer reflector")
	}
	if listRestrictions.Labels.String() != "" {
		t.Fatalf("shared informer must stay deliberately unfiltered, list LabelSelector=%q", listRestrictions.Labels.String())
	}
	if watchRestrictions == nil {
		t.Fatal("expected a watch from the informer reflector")
	}
	if watchRestrictions.Labels.String() != "" {
		t.Fatalf("shared informer must stay deliberately unfiltered, watch LabelSelector=%q", watchRestrictions.Labels.String())
	}

	// Policy selection is applied per-object in memory (and fails closed on
	// invalid selectors independently of any pushdown).
	matching := cmFixture("m", "uid-m")
	matching.SetLabels(map[string]string{"app": "cleanme"})
	other := cmFixture("o", "uid-o")
	other.SetLabels(map[string]string{"app": "keepme"})
	if !r.matchesSelectors(matching, &pol.Spec.TargetResource) {
		t.Fatal("matching object must match in memory")
	}
	if r.matchesSelectors(other, &pol.Spec.TargetResource) {
		t.Fatal("non-matching object must not match in memory")
	}
}

// The in-memory final match independently fails closed on an invalid
// selector: NOTHING matches, so nothing is deleted.
func TestMatchesSelectorsShared_InvalidSelectorMatchesNothing(t *testing.T) {
	target := &gcapi.TargetResourceSpec{
		Namespace: "app",
		LabelSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "app", Operator: "Between", Values: []string{"a", "b"}},
			},
		},
	}
	anyObject := cmFixture("x", "uid-x")
	if matchesSelectorsShared(anyObject, target) {
		t.Fatal("invalid selector must match NOTHING (fail closed), got a match")
	}
	// Same for an In-operator selector without values (invalid per
	// LabelSelectorAsSelector).
	target2 := &gcapi.TargetResourceSpec{
		Namespace: "app",
		LabelSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "app", Operator: "In"},
			},
		},
	}
	if matchesSelectorsShared(anyObject, target2) {
		t.Fatal("In-without-values selector must match NOTHING (fail closed), got a match")
	}
}

// Reconcile refuses a malformed policy from the API path BEFORE any
// evaluation: no informer, no watch, no deletion (policy_invalid).
func TestReconcile_MalformedSelectorPolicy_NoWatchNoDeletion(t *testing.T) {
	r, dyn := selectorReconciler(t)
	// A perfectly deletable object sits in the cluster.
	obj := cmFixture("target", "uid-1")
	if err := dyn.Tracker().Add(obj); err != nil {
		t.Fatal(err)
	}
	// Keep the safety gate but make sure deletes are enabled (config default).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pol := invalidSelectorPolicy()
	pol.Namespace = "pol-ns"
	if err := r.Create(ctx, pol); err != nil {
		t.Fatalf("policy create failed: %v", err)
	}

	result, err := r.Reconcile(ctx, reconcileRequest(pol))
	if err != nil {
		t.Fatalf("invalid policy must be a refusal with requeue, not an error: %v", err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("invalid policy must requeue, got %+v", result)
	}

	for _, action := range dyn.Actions() {
		if action.GetVerb() == "watch" {
			t.Fatalf("a WATCH was started for a malformed policy: %+v", action)
		}
		if action.GetVerb() == "delete" {
			t.Fatalf("a DELETE was issued for a malformed policy: %+v", action)
		}
	}
	// The object survives.
	live, gerr := dyn.Resource(cmGVR).Namespace("app").Get(ctx, "target", metav1.GetOptions{})
	if gerr != nil {
		t.Fatalf("object must survive: %v", gerr)
	}
	if string(live.GetUID()) != "uid-1" {
		t.Fatalf("object was mutated: %v", live.GetUID())
	}
}
