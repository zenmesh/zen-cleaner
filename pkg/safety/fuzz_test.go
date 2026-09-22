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

package safety

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	gcapi "github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

// fuzzObject builds an object from fuzz input; it must never panic and the
// hard invariants must hold for every input (SUPPORT2-033 §16).
func fuzzObject(data []byte) *unstructured.Unstructured {
	pick := func(i int, choices ...string) string {
		if len(choices) == 0 {
			return ""
		}
		if len(data) == 0 {
			return choices[0]
		}
		return choices[int(data[i%len(data)])%len(choices)]
	}
	o := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": pick(0, "v1", "batch/v1", "apps/v1"),
			"kind":       pick(1, "ConfigMap", "Pod", "PersistentVolumeClaim", "Namespace", "Job", "Secret"),
			"metadata": map[string]interface{}{
				"name":      pick(2, "x", "", "coredns"),
				"namespace": pick(3, "app", "kube-system", ""),
			},
		},
	}
	if len(data) > 4 && data[4]%3 == 0 {
		o.SetLabels(map[string]string{ExclusionLabelKey: pick(5, "true", "false")})
	}
	if len(data) > 6 && data[6]%4 == 0 {
		o.Object["status"] = map[string]interface{}{"phase": pick(7, "Running", "Succeeded", "Failed")}
	}
	return o
}

// FuzzGateEvaluateObject: the gate never panics, never allows a
// hard-protected kind, and never allows anything inside kube-system — for
// every input.
func FuzzGateEvaluateObject(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 0, 0, 0, 0})
	f.Add([]byte{0, 0, 0, 0, 1, 1, 1, 1})
	f.Add([]byte("kube-system pod running"))
	gate := NewGate(Config{OwnNamespace: "zen-cleaner-system"})
	f.Fuzz(func(t *testing.T, data []byte) {
		obj := fuzzObject(data)
		allowed, reason := gate.EvaluateObject(obj, gcapi.BehaviorSpec{AllowWorkloadDeletion: len(data) > 0 && data[0]%2 == 0})
		if obj.GetKind() == "PersistentVolumeClaim" && allowed {
			t.Fatalf("PVC allowed: %v (reason=%q)", obj.Object, reason)
		}
		if obj.GetKind() == "Namespace" && allowed {
			t.Fatalf("Namespace allowed: %v", obj.Object)
		}
		if obj.GetNamespace() == "kube-system" && allowed {
			t.Fatalf("kube-system object allowed: %v", obj.Object)
		}
	})
}

// FuzzGateNeverPanicsOnGarbage: nil-ish and malformed maps must not panic.
func FuzzGateNeverPanicsOnGarbage(f *testing.F) {
	f.Add([]byte{9, 9, 9})
	f.Fuzz(func(t *testing.T, data []byte) {
		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"kind": string(data),
			"metadata": map[string]interface{}{
				"name": string(data),
			},
		}}
		if len(data) == 0 {
			obj = nil
		}
		_, _ = NewGate(Config{}).EvaluateObject(obj, gcapi.BehaviorSpec{})
	})
}
