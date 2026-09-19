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

package events

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestNewRecorder(t *testing.T) {
	recorder := NewRecorder(nil, "test-component")
	if recorder == nil {
		t.Fatal("NewRecorder returned nil")
	}
	if recorder.recorder == nil {
		t.Error("recorder.recorder is nil")
	}
}

// mockRuntimeObject is a minimal runtime.Object implementation without GetName
type mockRuntimeObject struct{}

func (m *mockRuntimeObject) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

func (m *mockRuntimeObject) DeepCopyObject() runtime.Object {
	return &mockRuntimeObject{}
}

func TestGetResourceName(t *testing.T) {
	tests := []struct {
		name     string
		obj      runtime.Object
		wantName string
	}{
		{
			name: "pod with name",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			wantName: "test-pod",
		},
		{
			name:     "object without GetName",
			obj:      &mockRuntimeObject{},
			wantName: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetResourceName(tt.obj)
			if got != tt.wantName {
				t.Errorf("GetResourceName() = %v, want %v", got, tt.wantName)
			}
		})
	}
}

func TestRecorder_Eventf(t *testing.T) {
	recorder := NewRecorder(nil, "test-component")
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
	}

	// Should not panic even with nil client
	recorder.Eventf(pod, corev1.EventTypeNormal, "TestReason", "Test message: %s", "value")
}

func TestRecorder_Event(t *testing.T) {
	recorder := NewRecorder(nil, "test-component")
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
	}

	// Should not panic even with nil client
	recorder.Event(pod, corev1.EventTypeNormal, "TestReason", "Test message")
}
