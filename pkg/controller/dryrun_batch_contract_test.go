package controller

import (
	"context"
	"testing"

	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// SUPPORT2-041 deletion-safety law: behavior.dryRun must prevent ANY real
// deletion on every execution path. Found live: a policy stored with
// dryRun: true deleted matching objects (operation=delete_batch).
// This test pins the batch-path contract at unit level.
func TestDeleteBatch_DryRunMustNotDelete(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	staleCM := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":              "dry-target",
				"namespace":         "default",
				"creationTimestamp": "2020-01-01T00:00:00Z",
				"uid":               "dry-uid-1",
			},
			"data": map[string]interface{}{"k": "v"},
		},
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{gvr: "ConfigMapList"}, staleCM)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	statusUpdater := NewStatusUpdater(dynamicClient)
	eventRecorder := NewEventRecorder(nil)
	reconciler := NewPolicyReconcilerWithRESTMapper(fakeClient, scheme, dynamicClient, nil, statusUpdater, eventRecorder, config.NewControllerConfig())

	ttl30 := int64(30)
	policy := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "dry-policy", Namespace: "default"},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{APIVersion: "v1", Kind: "ConfigMap", Namespace: "default"},
			TTL:            v1alpha1.TTLSpec{SecondsAfterCreation: &ttl30},
			Behavior:       v1alpha1.BehaviorSpec{DryRun: true},
		},
	}

	batch := []*unstructured.Unstructured{staleCM}
	reasons := map[string]string{"dry-uid-1": "ttl_expired"}
	deleted, errs := reconciler.deleteBatch(context.Background(), batch, policy, ratelimiter.NewRateLimiter(100), reasons)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if deleted != 0 {
		t.Fatalf("dry-run policy reported %d deletions; must report 0", deleted)
	}

	// The resource must still exist in the authoritative store.
	obj, err := dynamicClient.Resource(gvr).Namespace("default").Get(context.Background(), "dry-target", metav1.GetOptions{})
	found := obj != nil
	if err != nil || !found {
		t.Fatalf("DELETION SAFETY VIOLATION: dryRun policy deleted the resource (err=%v, found=%v)", err, found)
	}
}
