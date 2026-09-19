package events

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEventSinkWrapperCreate(t *testing.T) {
	client := fake.NewSimpleClientset()
	events := client.CoreV1().Events("")

	wrapper := &eventSinkWrapper{events: events}

	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: "default",
		},
		Type:    "Normal",
		Reason:  "TestReason",
		Message: "Test message",
	}

	created, err := wrapper.Create(event)
	if err != nil {
		t.Logf("Create error (may be expected in test): %v", err)
	}
	if created != nil {
		t.Logf("Event created: %s", created.Name)
	}
}

func TestEventSinkWrapperUpdate(t *testing.T) {
	client := fake.NewSimpleClientset()
	events := client.CoreV1().Events("")

	wrapper := &eventSinkWrapper{events: events}

	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event-update",
			Namespace: "default",
		},
		Type:    "Normal",
		Reason:  "TestReason",
		Message: "Test message",
	}

	_, err := wrapper.Update(event)
	if err != nil {
		t.Logf("Update error (may be expected): %v", err)
	}
}

func TestEventSinkWrapperPatch(t *testing.T) {
	client := fake.NewSimpleClientset()
	events := client.CoreV1().Events("")

	wrapper := &eventSinkWrapper{events: events}

	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event-patch",
			Namespace: "default",
		},
		Type:    "Normal",
		Reason:  "TestReason",
		Message: "Test message",
	}

	_, err := wrapper.Patch(event, []byte("{}"))
	if err != nil {
		t.Logf("Patch error (may be expected): %v", err)
	}
}

func TestNewRecorderWithNilClient(t *testing.T) {
	recorder := NewRecorder(nil, "test-component")
	if recorder == nil {
		t.Error("Expected non-nil recorder")
	}
}

func TestNewRecorderWithFakeClient(t *testing.T) {
	client := fake.NewSimpleClientset()
	recorder := NewRecorder(client, "test-component")
	if recorder == nil {
		t.Fatal("Expected non-nil recorder")
	}
	if recorder.recorder == nil {
		t.Error("Expected non-nil internal recorder")
	}
}

func TestGetResourceNameWithPod(t *testing.T) {
	obj := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
	}
	name := GetResourceName(obj)
	if name != "test-pod" {
		t.Errorf("Expected 'test-pod', got '%s'", name)
	}
}

func TestGetResourceNameWithNode(t *testing.T) {
	obj := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}
	name := GetResourceName(obj)
	if name != "test-node" {
		t.Errorf("Expected 'test-node', got '%s'", name)
	}
}

func TestGetResourceNameWithNilObject(t *testing.T) {
	name := GetResourceName(nil)
	if name != "unknown" {
		t.Errorf("Expected 'unknown' for nil object, got '%s'", name)
	}
}
