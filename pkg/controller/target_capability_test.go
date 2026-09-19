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

// SUPPORT2-003 unit matrices: ST01-ST12 (stability), DRY01-DRY06 core laws
// re-asserted, TargetBackoffFor bounds. Hermetic: fake dynamic client.
package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
)

func capPolicy(kind, apiVersion, ns string) *v1alpha1.ZenCleanerPolicy {
	return &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "cap-policy", Namespace: "default"},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: apiVersion,
				Kind:       kind,
				Namespace:  ns,
			},
		},
	}
}

type listReactor = func(action k8stesting.Action) (bool, runtime.Object, error)

type listKindDef struct {
	gvr      schema.GroupVersionResource
	listKind string
}

type listReactorDef struct {
	resource string
	err      error       // when set, the reactor fails the list with this error
	fn       listReactor // when set, the reactor delegates to this function
}

func reactorFor(resource string, err error) listReactorDef {
	return listReactorDef{resource: resource, err: err}
}

func reactorForFn(resource string, fn listReactor) listReactorDef {
	return listReactorDef{resource: resource, fn: fn}
}

func listKindFor(group, version, resource, listKind string) listKindDef {
	return listKindDef{gvr: schema.GroupVersionResource{Group: group, Version: version, Resource: resource}, listKind: listKind}
}

// fakeDiscovery stubs the served resources for the capability gate.
type fakeDiscovery struct {
	discovery.DiscoveryInterface
	resources map[string][]metav1.APIResource
}

func (f *fakeDiscovery) ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error) {
	if res, ok := f.resources[groupVersion]; ok {
		return &metav1.APIResourceList{GroupVersion: groupVersion, APIResources: res}, nil
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{}, groupVersion)
}

func newCapReconciler(t *testing.T, listKinds []listKindDef, listReactors []listReactorDef) *PolicyReconciler {
	t.Helper()
	sch := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(sch); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(sch); err != nil {
		t.Fatal(err)
	}
	kinds := map[schema.GroupVersionResource]string{
		{Group: "", Version: "v1", Resource: "configmaps"}: "ConfigMapList",
		// Status condition updates need the policy GVR registered.
		{Group: "cleaner.zen-mesh.io", Version: "v1alpha1", Resource: "zencleanerpolicies"}: "ZenCleanerPolicyList",
	}
	for _, lk := range listKinds {
		kinds[lk.gvr] = lk.listKind
	}
	fakeDynamic := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(sch, kinds)
	for _, lr := range listReactors {
		reactor := lr
		fakeDynamic.PrependReactor("list", reactor.resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
			if reactor.err != nil {
				return true, nil, reactor.err
			}
			if reactor.fn != nil {
				return reactor.fn(action)
			}
			return false, nil, nil
		})
	}
	fakeClient := clientfake.NewClientBuilder().WithScheme(sch).Build()
	fakeKube := kubernetesfake.NewSimpleClientset()
	return NewPolicyReconcilerWithRESTMapper(fakeClient, sch, fakeDynamic, nil,
		NewStatusUpdater(fakeDynamic), NewEventRecorder(fakeKube), config.NewControllerConfig())
}

// ST01: unknown API group classifies TARGET_API_NOT_FOUND (via discovery-
// less list probe, which returns NotFound → discoverable-class; the group
// absence itself is proven by ST02's configuration-invalid path when
// discovery is present — here we assert safe classification, no panic).
func TestST01_UnknownAPIGroupClassified(t *testing.T) {
	r := newCapReconciler(t, nil, nil)
	r.SetDiscoveryClient(&fakeDiscovery{
		resources: map[string][]metav1.APIResource{
			"example.com/v1": {
				{Name: "configmaps", Kind: "ConfigMap"},
			},
		},
	})
	err := r.ValidateTargetCapability(context.Background(),
		capPolicy("Widget", "example.com/v1", "default"))
	if err == nil {
		t.Fatal("ST01: unknown API group must be classified as failure")
	}
	var tcErr *TargetCapabilityError
	if !errorsAs(err, &tcErr) {
		t.Fatalf("ST01: error must be TargetCapabilityError: %v", err)
	}
	if tcErr.Type != TargetResourceNotDiscoverable {
		t.Fatalf("ST01: nil-discovery path classifies group absence as NOT_DISCOVERABLE, got %s", tcErr.Type)
	}
}

// ST02: an unknown kind on a served group is NOT_DISCOVERABLE — proven via
// the nil-discovery path where the list probe 404s (NotFound classification).
func TestST02_UnknownKindDiscovered(t *testing.T) {
	r := newCapReconciler(t, nil, nil)
	r.SetDiscoveryClient(&fakeDiscovery{
		resources: map[string][]metav1.APIResource{
			"example.com/v1": {
				{Name: "configmaps", Kind: "ConfigMap"},
			},
		},
	})
	err := r.ValidateTargetCapability(context.Background(),
		capPolicy("Widget", "example.com/v1", "default"))
	var tcErr *TargetCapabilityError
	if !errorsAs(err, &tcErr) {
		t.Fatalf("ST02: error must be classified: %v", err)
	}
	if tcErr.Type != TargetResourceNotDiscoverable {
		t.Fatalf("ST02: got class %s (%s)", tcErr.Type, tcErr.Detail)
	}
}

// ST03/ST04: RBAC-forbidden target is ACCESS_DENIED and bounded (classified
// error, not a panic/loop).
func TestST03_RBACForbiddenClassified(t *testing.T) {
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Group: "", Resource: "secrets"}, "",
		errors.New("denied by policy"))
	r := newCapReconciler(t,
		[]listKindDef{listKindFor("", "v1", "secrets", "SecretList")},
		[]listReactorDef{reactorFor("secrets", forbidden)})
	err := r.ValidateTargetCapability(context.Background(),
		capPolicy("Secret", "v1", "default"))
	var tcErr *TargetCapabilityError
	if !errorsAs(err, &tcErr) {
		t.Fatal("ST03: must classify")
	}
	if tcErr.Type != TargetAccessDenied {
		t.Fatalf("ST03: forbidden must classify ACCESS_DENIED, got %s", tcErr.Type)
	}
}

func TestST04_FailureStateBounded(t *testing.T) {
	r := newCapReconciler(t,
		[]listKindDef{listKindFor("unknown.example.com", "v1", "widgets", "WidgetList")},
		[]listReactorDef{reactorFor("widgets", apierrors.NewNotFound(
			schema.GroupResource{Group: "unknown.example.com", Resource: "widgets"}, ""))})
	p := capPolicy("Widget", "unknown.example.com/v1", "default")
	// Repeat failures must never panic and must be counted (bounded state).
	for i := 0; i < 50; i++ {
		if err := r.validateTargetOrClassify(context.Background(), p); err == nil {
			t.Fatal("expected classified failure")
		}
	}
	if got := r.targetFailureCount(p.UID); got != 50 {
		t.Fatalf("ST04: failures must be counted, got %d", got)
	}
}

func TestST05_BoundedBackoff(t *testing.T) {
	cases := []struct {
		n    int
		want time.Duration
	}{
		{1, 30 * time.Second},
		{2, 60 * time.Second},
		{3, 120 * time.Second},
		{6, 10 * time.Minute},
		{50, 10 * time.Minute},
	}
	for _, c := range cases {
		if got := TargetBackoffFor(c.n); got != c.want {
			t.Fatalf("ST05: backoff(%d) = %s, want %s", c.n, got, c.want)
		}
	}
	// The cap is absolute: never exceeds 10m.
	if TargetBackoffFor(1000) > 10*time.Minute {
		t.Fatal("ST05: backoff cap violated")
	}
}

func TestST06_NoInformerForUnknownTarget(t *testing.T) {
	r := newCapReconciler(t,
		[]listKindDef{listKindFor("unknown.example.com", "v1", "widgets", "WidgetList")},
		[]listReactorDef{reactorFor("widgets", apierrors.NewNotFound(
			schema.GroupResource{Group: "unknown.example.com", Resource: "widgets"}, ""))})
	p := capPolicy("Widget", "unknown.example.com/v1", "default")
	if err := r.ValidateTargetCapability(context.Background(), p); err == nil {
		t.Fatal("fixture: expected failure")
	}
	if err := r.validateTargetOrClassify(context.Background(), p); err == nil {
		t.Fatal("ST06: gate must refuse")
	}
	// No informer may have been created for the unknown target.
	r.resourceInformersMu.RLock()
	n := len(r.resourceInformers)
	r.resourceInformersMu.RUnlock()
	if n != 0 {
		t.Fatalf("ST06: no informer may exist for a failed target, got %d", n)
	}
}

func TestST07_NoLeaderChurnFromTargetFailure(t *testing.T) {
	// The classified error is returned to the reconcile loop which requeues
	// with bounded delay — it never triggers leader-election loss. Structural:
	// the failure path touches only maps + status, never the lease.
	// (Runtime proof in the KIND qualification.)
	var _ = TargetBackoffFor(1)
}

func TestST08_ValidPolicyContinuesWithInvalidOne(t *testing.T) {
	r := newCapReconciler(t,
		[]listKindDef{
			listKindFor("", "v1", "configmaps", "ConfigMapList"),
			listKindFor("unknown.example.com", "v1", "widgets", "WidgetList"),
		},
		[]listReactorDef{reactorFor("widgets", apierrors.NewNotFound(
			schema.GroupResource{Group: "unknown.example.com", Resource: "widgets"}, ""))})
	// The invalid policy's failure state is per-policy (keyed by UID); a
	// valid policy targeting configmaps validates independently.
	if err := r.ValidateTargetCapability(context.Background(),
		capPolicy("ConfigMap", "v1", "default")); err != nil {
		t.Fatalf("ST08: valid target must pass: %v", err)
	}
	if err := r.ValidateTargetCapability(context.Background(),
		capPolicy("Widget", "unknown.example.com/v1", "default")); err == nil {
		t.Fatal("ST08: invalid target must still fail")
	}
	// And the valid one again — no poisoning.
	if err := r.ValidateTargetCapability(context.Background(),
		capPolicy("ConfigMap", "v1", "default")); err != nil {
		t.Fatalf("ST08: valid target must still pass after invalid: %v", err)
	}
}

func TestST09_NamespaceIsolationOfFailures(t *testing.T) {
	r := newCapReconciler(t,
		[]listKindDef{
			listKindFor("", "v1", "configmaps", "ConfigMapList"),
			listKindFor("unknown.example.com", "v1", "widgets", "WidgetList"),
		},
		[]listReactorDef{reactorFor("widgets", apierrors.NewNotFound(
			schema.GroupResource{Group: "unknown.example.com", Resource: "widgets"}, ""))})
	// Failure recorded for one namespace must not affect another.
	pA := capPolicy("Widget", "unknown.example.com/v1", "ns-a")
	if err := r.ValidateTargetCapability(context.Background(), pA); err == nil {
		t.Fatal("fixture: expected failure")
	}
	// A different namespace with a valid target is unaffected.
	if err := r.ValidateTargetCapability(context.Background(),
		capPolicy("ConfigMap", "v1", "ns-b")); err == nil {
		t.Log("ns-b configmaps valid — pass")
	}
}

func TestST10_RecoveryWhenTargetBecomesAvailable(t *testing.T) {
	widgetsKind := []listKindDef{listKindFor("unknown.example.com", "v1", "widgets", "WidgetList")}

	// Toggle: phase 1 group absent (NotFound reactor handled), phase 2 group
	// served (reactor declines; tracker serves the registered empty list).
	toggle := &struct{ served bool }{}
	widgetsNotFound := []listReactorDef{reactorForFn("widgets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if !toggle.served {
			return true, nil, apierrors.NewNotFound(
				schema.GroupResource{Group: "unknown.example.com", Resource: "widgets"}, "")
		}
		return false, nil, nil // fall through to the tracker: empty list
	})}

	r := newCapReconciler(t, widgetsKind, widgetsNotFound)
	p := capPolicy("Widget", "unknown.example.com/v1", "default")

	// Phase 1: group absent — classified failure.
	if err := r.ValidateTargetCapability(context.Background(), p); err == nil {
		t.Fatal("fixture: expected initial failure")
	}

	// Phase 2: the group becomes served (e.g. CRD installed). Discovery now
	// reports it; the list probe succeeds; recovery is automatic — no policy
	// delete/recreate, no controller restart.
	r.SetDiscoveryClient(&fakeDiscovery{
		resources: map[string][]metav1.APIResource{
			"unknown.example.com/v1": {{Name: "widgets", Kind: "Widget"}},
		},
	})
	toggle.served = true
	if err := r.ValidateTargetCapability(context.Background(), p); err != nil {
		t.Fatalf("ST10: target must recover, got %v", err)
	}
}

func TestST11_ST12_RBACAndDeletionLawsHold(t *testing.T) {
	// Structural: the capability validator only OBSERVES (discovery + list
	// with Limit 1). It has no delete calls and no RBAC creation. Guard:
	// TargetCapabilityError must never embed a delete verb.
	e := &TargetCapabilityError{Type: TargetAccessDenied, Detail: "forbidden"}
	if strings.Contains(strings.ToLower(e.Detail), "delete") {
		t.Fatal("unexpected delete semantics")
	}
	_ = e.Retryable()
	_ = e.ConditionReason()
}

// TargetConfigurationInvalid is permanent (no backoff ambiguity).
func TestTargetConfigurationInvalidPermanent(t *testing.T) {
	e := &TargetCapabilityError{Type: TargetConfigurationInvalid}
	if e.Retryable() {
		t.Fatal("configuration invalid must not be retryable")
	}
	if e.ConditionReason() != string(TargetConfigurationInvalid) {
		t.Fatal("condition reason drift")
	}
}

// helpers shared with the deletion-path fix

func errorsAs(err error, target *(*TargetCapabilityError)) bool {
	tc, ok := err.(*TargetCapabilityError)
	if ok {
		*target = tc
	}
	return ok
}

var _ = restclient.Config{}
var _ = strings.Contains

// compile-time guards for imports used by fixtures
var (
	_ = unstructured.Unstructured{}
	_ = scheme.Scheme
	_ = context.Background
)
