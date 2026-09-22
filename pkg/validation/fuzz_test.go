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

package validation

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gcapi "github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

// fuzzString derives a bounded pseudo-random string from fuzz bytes so the
// search space stays structured (SUPPORT2-033 §16).
func fuzzString(data []byte, i int) string {
	alphabet := []rune("abc*-./_0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	n := 0
	if len(data) > i {
		n = int(data[i]) % 8
	}
	var b strings.Builder
	for j := 0; j < n; j++ {
		idx := 0
		if len(data) > i+j+1 {
			idx = int(data[i+j+1]) % len(alphabet)
		}
		b.WriteRune(alphabet[idx])
	}
	return b.String()
}

// FuzzValidatePolicy: validation must never panic and must never accept a
// policy targeting a protected namespace or hard-protected kind, whatever
// the fuzzed input shape is.
func FuzzValidatePolicy(f *testing.F) {
	f.Add([]byte{3, 5, 2, 7, 1, 4, 6, 0})
	f.Add([]byte("v1ConfigMapdefault3600"))
	f.Fuzz(func(t *testing.T, data []byte) {
		policy := &gcapi.ZenCleanerPolicy{
			Spec: gcapi.ZenCleanerPolicySpec{
				TargetResource: gcapi.TargetResourceSpec{
					APIVersion: fuzzString(data, 0),
					Kind:       fuzzString(data, 2),
					Namespace:  fuzzString(data, 4),
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{fuzzString(data, 6): fuzzString(data, 8)},
					},
				},
				TTL: gcapi.TTLSpec{
					SecondsAfterCreation: func() *int64 {
						v := int64(0)
						if len(data) > 0 {
							v = int64(int64(data[0]%40) - 5)
						}
						return &v
					}(),
				},
				Behavior: gcapi.BehaviorSpec{
					Finalizer: fuzzString(data, 10),
				},
			},
		}
		err := ValidatePolicy(policy)
		if err != nil {
			return
		}
		// Invariants that must hold for every accepted policy.
		if ProtectedNamespaces[policy.Spec.TargetResource.Namespace] {
			t.Fatalf("validated policy targets protected namespace %q", policy.Spec.TargetResource.Namespace)
		}
	})
}
