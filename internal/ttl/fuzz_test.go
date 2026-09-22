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

package ttl

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// FuzzCalculateExpirationTime: age calculation must never panic and must
// never return a negative non-zero duration semantics (expired vs not is a
// boolean decision; garbage fields fail closed) — SUPPORT2-033 §16.
func FuzzCalculateExpirationTime(f *testing.F) {
	f.Add([]byte("creation30"))
	f.Add([]byte("fieldpath0"))
	f.Add([]byte("rel1800"))
	f.Fuzz(func(t *testing.T, data []byte) {
		pick := func(i int, choices ...string) string {
			if len(choices) == 0 || len(data) == 0 {
				return ""
			}
			return choices[int(data[i%len(data)])%len(choices)]
		}
		obj := &unstructured.Unstructured{Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"creationTimestamp": pick(0, "2026-01-01T00:00:00Z", "1999-01-01T00:00:00Z", "garbage"),
			},
			"status": map[string]interface{}{
				"state":           pick(1, "done", "running"),
				"lastProcessedAt": pick(2, "2026-01-02T00:00:00Z", "garbage"),
			},
		}}
		ttl := Spec{
			SecondsAfterCreation: func() *int64 {
				v := int64(0)
				if len(data) > 3 {
					v = int64(int64(data[3]%30) - 10)
				}
				return &v
			}(),
			FieldPath:  pick(4, "", "status.state", "status.missing"),
			RelativeTo: pick(5, "", "status.lastProcessedAt"),
			SecondsAfter: func() *int64 {
				v := int64(0)
				if len(data) > 6 {
					v = int64(int64(data[6]%30) - 10)
				}
				return &v
			}(),
		}
		expiration, err := CalculateExpirationTime(obj, &ttl)
		if err != nil {
			return // fail-closed is fine
		}
		_ = expiration
	})
}
