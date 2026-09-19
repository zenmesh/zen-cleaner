package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
)

func TestHealthChecker_probes(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	dynamicClient := fake.NewSimpleDynamicClient(scheme)
	reconciler := NewPolicyReconcilerWithRESTMapper(
		fakeClient,
		scheme,
		dynamicClient,
		nil,
		NewStatusUpdater(dynamicClient),
		NewEventRecorder(nil),
		config.NewControllerConfig(),
	)

	h := NewHealthChecker(reconciler)
	h.SetMaxTimeSinceLastEvaluation(time.Minute)
	h.UpdateLastEvaluationTime()

	req := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
	if err := h.ReadinessCheck(req); err != nil {
		t.Errorf("ReadinessCheck: %v", err)
	}
	if err := h.LivenessCheck(req); err != nil {
		t.Errorf("LivenessCheck: %v", err)
	}
	if err := h.StartupCheck(req); err != nil {
		t.Errorf("StartupCheck: %v", err)
	}
}
