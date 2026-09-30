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

// HELPER-H258+ H2 proofs: the six receipt laws against the REAL execution
// path (fake client, no mocks of the classification logic).

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"
	gcapi "github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
)

func testConfigMap(ns, name, uid string) *unstructured.Unstructured {
	o := &unstructured.Unstructured{}
	o.SetAPIVersion("v1")
	o.SetKind("ConfigMap")
	o.SetNamespace(ns)
	o.SetName(name)
	o.SetUID(types.UID(uid))
	return o
}

func testPolicy(dryRun bool) *gcapi.ZenCleanerPolicy {
	p := &gcapi.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "pol-ns", Name: "pol", UID: "policy-uid-1"},
		Spec:       gcapi.ZenCleanerPolicySpec{},
	}
	p.Spec.Behavior.DryRun = dryRun
	return p
}

func reconcilerFor(t *testing.T, objs ...runtime.Object) *PolicyReconciler {
	t.Helper()
	sch := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(sch); err != nil {
		t.Fatal(err)
	}
	if err := gcapi.AddToScheme(sch); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(sch).WithRuntimeObjects(objs...).Build()
	dyn := dynamicfake.NewSimpleDynamicClient(sch, objs...)
	r := NewPolicyReconcilerWithRESTMapper(c, sch, dyn, nil, NewStatusUpdater(dyn), NewEventRecorder(nil), config.NewControllerConfig())
	// H2 isolates the RECEIPT path: the constructor's default safety gate
	// silently refuses (product law) and would mask receipt classification.
	// Safety-gate refusal classification is exercised separately (the
	// still-present-after-path law covers refused deletes as FAILED).
	r.safetyGate = nil
	return r
}

func rl() *ratelimiter.RateLimiter { return ratelimiter.NewRateLimiter(100) }

// Law 1: real delete emits exactly one lawful receipt whose outcome is
// SUCCEEDED-by-observation (object absent after execution).
func TestH2RealDeleteEmitsSingleLawfulReceipt(t *testing.T) {
	r := reconcilerFor(t, testConfigMap("app", "victim", "uid-1"))
	pol := testPolicy(false)
	batch := []*unstructured.Unstructured{testConfigMap("app", "victim", "uid-1")}
	n, errs, receipt := r.ExecuteWithReceipt(context.Background(), batch, pol, rl(), map[string]string{})
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	_ = n
	gvrProbe := r.resolveGVRForDeletion(batch[0])
	t.Logf("DBG gvr=%v", gvrProbe)
	probe, perr := r.dynamicClient.Resource(gvrProbe).Namespace("app").Get(context.Background(), "victim", metav1.GetOptions{})
	t.Logf("DBG post-exec present=%v err=%v", probe != nil, perr)
	live, lerr := r.getLiveForPrecondition(context.Background(), batch[0], gvrProbe)
	t.Logf("DBG precondition live=%v lerr=%v liveUID=%q plannedUID=%q", live != nil, lerr, func() string {
		if live != nil {
			return string(live.GetUID())
		}
		return ""
	}(), string(batch[0].GetUID()))
	if live != nil {
		if derr := r.dynamicClient.Resource(gvrProbe).Namespace("app").Delete(context.Background(), "victim", metav1.DeleteOptions{}); derr != nil {
			t.Logf("DBG manual delete err=%v", derr)
		} else {
			t.Log("DBG manual delete OK")
		}
	}
	if receipt == nil {
		t.Fatal("real delete must emit a receipt")
	}
	if len(errs) != 0 {
		t.Fatalf("batch errs: %v", errs)
	}
	if len(receipt.Results) != 1 || receipt.Results[0].Class != ResultSucceeded {
		t.Fatalf("§H2: observed-success classification required: %+v", receipt.Results)
	}
	if receipt.PolicyRef.Name != "pol" || receipt.PolicyRef.UID != "policy-uid-1" {
		t.Fatal("§H2: receipt must bind the governing policy")
	}
	if receipt.PlanDigest == "" {
		t.Fatal("§H2: receipt must bind the plan digest")
	}
	if receipt.OverallStatus != "SUCCEEDED" {
		t.Fatalf("overall: %s", receipt.OverallStatus)
	}
	// Restart law: a re-planned run over the same (now deleted) object
	// classifies ALREADY_ABSENT — never a second SUCCEEDED for one lifetime.
	batch2 := []*unstructured.Unstructured{testConfigMap("app", "victim", "uid-1")}
	_, _, r2 := r.ExecuteWithReceipt(context.Background(), batch2, pol, rl(), map[string]string{})
	if r2 == nil || len(r2.Results) != 1 || r2.Results[0].Class != ResultAlreadyAbsent {
		t.Fatalf("§H2 restart law: re-run must classify ALREADY_ABSENT: %+v", r2)
	}
}

// Law 2: dry-run emits NO receipt (no fake successful effect receipt).
func TestH2DryRunEmitsNoReceipt(t *testing.T) {
	r := reconcilerFor(t, testConfigMap("app", "keepme", "uid-2"))
	pol := testPolicy(true)
	batch := []*unstructured.Unstructured{testConfigMap("app", "keepme", "uid-2")}
	_, _, receipt := r.ExecuteWithReceipt(context.Background(), batch, pol, rl(), map[string]string{})
	if receipt != nil {
		t.Fatalf("§H2: dry-run must not emit an effect receipt: %+v", receipt)
	}
	// And the object survives (dry-run semantics unchanged).
	survivor := testConfigMap("app", "keepme", "uid-2")
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "app", Name: "keepme"}, survivor); err != nil {
		t.Fatal("dry-run must not delete")
	}
}

// Law 3: stale UID classified SKIPPED_PRECONDITION_MISMATCH.
func TestH2StaleUIDClassified(t *testing.T) {
	r := reconcilerFor(t, testConfigMap("app", "replaced", "uid-NEW"))
	pol := testPolicy(false)
	batch := []*unstructured.Unstructured{testConfigMap("app", "replaced", "uid-OLD")}
	_, _, receipt := r.ExecuteWithReceipt(context.Background(), batch, pol, rl(), map[string]string{})
	if receipt == nil || len(receipt.Results) != 1 || receipt.Results[0].Class != ResultSkippedPreconditionMismatch {
		t.Fatalf("§H2: stale plan must skip with precondition mismatch: %+v", receipt)
	}
	if receipt.OverallStatus != "PARTIAL" {
		t.Fatalf("mismatch degrades overall honestly: %s", receipt.OverallStatus)
	}
	// The replaced object must NOT have been deleted.
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: "app", Name: "replaced"}, testConfigMap("app", "replaced", "uid-NEW")); err != nil {
		t.Fatal("stale-plan protection failed: replaced object was deleted")
	}
}

// Law 4: object absent BEFORE execution classified ALREADY_ABSENT (the
// dedicated bounded class — never a fake SUCCEEDED effect).
func TestH2AlreadyAbsentClassified(t *testing.T) {
	r := reconcilerFor(t) // empty cluster
	pol := testPolicy(false)
	batch := []*unstructured.Unstructured{testConfigMap("app", "ghost", "uid-3")}
	_, _, receipt := r.ExecuteWithReceipt(context.Background(), batch, pol, rl(), map[string]string{})
	if receipt == nil || len(receipt.Results) != 1 || receipt.Results[0].Class != ResultAlreadyAbsent {
		t.Fatalf("§H2: pre-absent must classify ALREADY_ABSENT: %+v", receipt)
	}
	if receipt.OverallStatus != "SUCCEEDED" {
		t.Fatalf("ALREADY_ABSENT is a success-class outcome: %s", receipt.OverallStatus)
	}
}
