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

func obj(ns, kind, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
		},
	}
}

// SUPPORT2-033 §2: the safety matrix. Every row is a law.
func TestGate_EvaluateObject(t *testing.T) {
	gate := NewGate(Config{OwnNamespace: "zen-cleaner-system", ProtectedNamespaces: []string{"ops-protected"}})

	cases := []struct {
		name        string
		obj         *unstructured.Unstructured
		behavior    gcapi.BehaviorSpec
		wantAllow   bool
		wantRefusal string
	}{
		{"system namespace denied", obj("kube-system", "ConfigMap", "coredns"), gcapi.BehaviorSpec{}, false, ReasonProtectedNamespace},
		{"node-lease namespace denied", obj("kube-node-lease", "ConfigMap", "x"), gcapi.BehaviorSpec{}, false, ReasonProtectedNamespace},
		{"own namespace denied", obj("zen-cleaner-system", "ConfigMap", "x"), gcapi.BehaviorSpec{}, false, ReasonProtectedNamespace},
		{"configured protected namespace denied", obj("ops-protected", "ConfigMap", "x"), gcapi.BehaviorSpec{}, false, ReasonProtectedNamespace},
		{"PVC denied", obj("app", "PersistentVolumeClaim", "data"), gcapi.BehaviorSpec{}, false, ReasonProtectedKind},
		{"PV denied", obj("", "PersistentVolume", "pv-1"), gcapi.BehaviorSpec{}, false, ReasonProtectedKind},
		{"Namespace kind denied", obj("", "Namespace", "whatever"), gcapi.BehaviorSpec{}, false, ReasonProtectedKind},
		{"CRD denied", obj("", "CustomResourceDefinition", "x"), gcapi.BehaviorSpec{}, false, ReasonProtectedKind},
		{"webhook config denied", obj("app", "ValidatingWebhookConfiguration", "x"), gcapi.BehaviorSpec{}, false, ReasonProtectedKind},
		{"exclusion label denied", func() *unstructured.Unstructured {
			o := obj("app", "ConfigMap", "keepme")
			o.SetLabels(map[string]string{ExclusionLabelKey: "true"})
			return o
		}(), gcapi.BehaviorSpec{}, false, ReasonExcludedLabel},
		{"exclusion annotation denied", func() *unstructured.Unstructured {
			o := obj("app", "ConfigMap", "keepme")
			o.SetAnnotations(map[string]string{ExclusionLabelKey: "true"})
			return o
		}(), gcapi.BehaviorSpec{}, false, ReasonExcludedLabel},
		{"exclusion label non-true ignored", func() *unstructured.Unstructured {
			o := obj("app", "ConfigMap", "maydelete")
			o.SetLabels(map[string]string{ExclusionLabelKey: "false"})
			return o
		}(), gcapi.BehaviorSpec{}, true, ""},
		{"pod denied without opt-in", obj("app", "Pod", "web-1"), gcapi.BehaviorSpec{}, false, ReasonWorkloadManaged},
		{"job denied without opt-in", obj("app", "Job", "build"), gcapi.BehaviorSpec{}, false, ReasonWorkloadManaged},
		{"running pod denied even with opt-in", func() *unstructured.Unstructured {
			o := obj("app", "Pod", "web-1")
			o.Object["status"] = map[string]interface{}{"phase": "Running"}
			return o
		}(), gcapi.BehaviorSpec{AllowWorkloadDeletion: true}, false, ReasonActiveWorkload},
		{"succeeded pod allowed with opt-in", func() *unstructured.Unstructured {
			o := obj("app", "Pod", "done")
			o.Object["status"] = map[string]interface{}{"phase": "Succeeded"}
			return o
		}(), gcapi.BehaviorSpec{AllowWorkloadDeletion: true}, true, ""},
		{"pod without provable phase denied", obj("app", "Pod", "opaque"), gcapi.BehaviorSpec{AllowWorkloadDeletion: true}, false, ReasonActiveWorkload},
		{"regular configmap allowed", obj("app", "ConfigMap", "stale"), gcapi.BehaviorSpec{}, true, ""},
		{"nil object denied", nil, gcapi.BehaviorSpec{}, false, ReasonUnknown},
		{"nameless object denied", obj("app", "ConfigMap", ""), gcapi.BehaviorSpec{}, false, ReasonUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allowed, reason := gate.EvaluateObject(tc.obj, tc.behavior)
			if allowed != tc.wantAllow || reason != tc.wantRefusal {
				t.Errorf("EvaluateObject() = (%v,%q), want (%v,%q)", allowed, reason, tc.wantAllow, tc.wantRefusal)
			}
		})
	}
}

// SUPPORT2-033 §2: the emergency stop denies everything.
func TestGate_EmergencyStop(t *testing.T) {
	gate := NewGate(Config{DeletesDisabled: true})
	allowed, reason := gate.EvaluateObject(obj("app", "ConfigMap", "anything"), gcapi.BehaviorSpec{})
	if allowed || reason != ReasonDeletesDisabled {
		t.Errorf("emergency stop not engaged: (%v,%q)", allowed, reason)
	}
}

// SUPPORT2-033 §3: policy targets are checked at admission/runtime.
func TestGate_EvaluatePolicyTarget(t *testing.T) {
	gate := NewGate(Config{})
	if allowed, reason := gate.EvaluatePolicyTarget(gcapi.TargetResourceSpec{Kind: "ConfigMap", Namespace: "kube-system"}); allowed || reason != ReasonProtectedNamespace {
		t.Errorf("protected namespace target allowed: (%v,%q)", allowed, reason)
	}
	if allowed, reason := gate.EvaluatePolicyTarget(gcapi.TargetResourceSpec{Kind: "PersistentVolumeClaim", Namespace: "app"}); allowed || reason != ReasonProtectedKind {
		t.Errorf("protected kind target allowed: (%v,%q)", allowed, reason)
	}
	if allowed, _ := gate.EvaluatePolicyTarget(gcapi.TargetResourceSpec{Kind: "ConfigMap", Namespace: "app"}); !allowed {
		t.Error("legitimate target refused")
	}
}
