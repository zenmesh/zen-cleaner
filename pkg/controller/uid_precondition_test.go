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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"

	sdklog "github.com/zenmesh/zen-cleaner/internal/logging"

	gcapi "github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/safety"
)

func testLogger() *sdklog.Logger { return sdklog.NewLogger("test") }

// cmWithUID builds an unstructured ConfigMap fixture with a fixed UID.
func cmWithUID(name, uid string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "app",
				"uid":       uid,
			},
		},
	}
}

var cmGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}

// SUPPORT2-033 §4: a stale-cache observation (same name, new UID) must NEVER
// be deleted. The live UID precondition refuses and the object survives.
func TestPerformResourceDeletion_StaleUIDRefused(t *testing.T) {
	live := cmWithUID("target", "uid-REPLACEMENT")
	dc := dynamicfake.NewSimpleDynamicClient(scheme.Scheme, live)
	r := &PolicyReconciler{
		dynamicClient: dc,
		safetyGate:    safety.NewGate(safety.Config{}),
		logger:        testLogger(),
	}

	observed := cmWithUID("target", "uid-ORIGINAL") // stale snapshot

	err := r.performResourceDeletion(context.Background(), observed, cmGVR,
		&metav1.DeleteOptions{}, gcapi.BehaviorSpec{})
	if err != nil {
		t.Fatalf("stale-UID refusal must be non-fatal, got: %v", err)
	}

	stillThere, err := dc.Resource(cmGVR).Namespace("app").Get(context.Background(), "target", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get after refusal: %v", err)
	}
	if string(stillThere.GetUID()) != "uid-REPLACEMENT" {
		t.Fatalf("replacement object was mutated: uid=%s", stillThere.GetUID())
	}
	actions := dc.Actions()
	for _, a := range actions {
		if a.GetVerb() == "delete" {
			t.Fatal("a DELETE was issued despite the UID precondition")
		}
	}
}

// SUPPORT2-033 §4: when the object vanished since observation, the deletion
// is a no-op success (idempotent), not an error.
func TestPerformResourceDeletion_AlreadyAbsent(t *testing.T) {
	dc := dynamicfake.NewSimpleDynamicClient(scheme.Scheme) // empty cluster
	r := &PolicyReconciler{
		dynamicClient: dc,
		safetyGate:    safety.NewGate(safety.Config{}),
		logger:        testLogger(),
	}
	observed := cmWithUID("gone", "uid-ORIGINAL")
	if err := r.performResourceDeletion(context.Background(), observed, cmGVR,
		&metav1.DeleteOptions{}, gcapi.BehaviorSpec{}); err != nil {
		t.Fatalf("already-absent must be a no-op success, got: %v", err)
	}
	for _, a := range dc.Actions() {
		if a.GetVerb() == "delete" {
			t.Fatal("a DELETE was issued for an absent object")
		}
	}
}

// SUPPORT2-033 §2: the delete-time safety gate refuses protected objects
// even when the policy matched them.
func TestPerformResourceDeletion_SafetyGateRefuses(t *testing.T) {
	pvc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"metadata": map[string]interface{}{
				"name":      "data",
				"namespace": "app",
				"uid":       "uid-PVC",
			},
		},
	}
	dc := dynamicfake.NewSimpleDynamicClient(scheme.Scheme, pvc)
	r := &PolicyReconciler{
		dynamicClient: dc,
		safetyGate:    safety.NewGate(safety.Config{}),
		logger:        testLogger(),
	}
	err := r.performResourceDeletion(context.Background(), pvc,
		schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
		&metav1.DeleteOptions{}, gcapi.BehaviorSpec{})
	if err != nil {
		t.Fatalf("gate refusal must be non-fatal, got: %v", err)
	}
	for _, a := range dc.Actions() {
		if a.GetVerb() == "delete" {
			t.Fatal("a DELETE was issued for a protected kind")
		}
	}
}

// SUPPORT2-033 §4: matching UID deletes normally.
func TestPerformResourceDeletion_MatchingUIDDeletes(t *testing.T) {
	live := cmWithUID("target", "uid-MATCH")
	dc := dynamicfake.NewSimpleDynamicClient(scheme.Scheme, live)
	r := &PolicyReconciler{
		dynamicClient: dc,
		safetyGate:    safety.NewGate(safety.Config{}),
		logger:        testLogger(),
	}
	observed := cmWithUID("target", "uid-MATCH")
	if err := r.performResourceDeletion(context.Background(), observed, cmGVR,
		&metav1.DeleteOptions{}, gcapi.BehaviorSpec{}); err != nil {
		t.Fatalf("matching-UID delete failed: %v", err)
	}
	found := false
	for _, a := range dc.Actions() {
		if a.GetVerb() == "delete" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a DELETE for the matching-UID object")
	}
}
