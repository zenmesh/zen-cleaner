package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
)

func TestNewInformerStoreResourceLister(t *testing.T) {
	lister := NewInformerStoreResourceLister(nil)
	if lister == nil {
		t.Fatal("Expected non-nil lister")
	}
}

func TestInformerStoreResourceLister_ListResources(t *testing.T) {
	store := cache.NewStore(cache.MetaNamespaceKeyFunc)
	pod := &unstructured.Unstructured{}
	pod.SetAPIVersion("v1")
	pod.SetKind("Pod")
	pod.SetNamespace("default")
	pod.SetName("p1")
	if err := store.Add(pod); err != nil {
		t.Fatal(err)
	}
	_ = store.Add("not-an-unstructured")

	lister := NewInformerStoreResourceLister(store)
	ctx := context.Background()
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

	got, err := lister.ListResources(ctx, gvr, "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d resources", len(got))
	}

	gotAll, err := lister.ListResources(ctx, gvr, "")
	if err != nil || len(gotAll) != 1 {
		t.Fatalf("empty ns: err=%v len=%d", err, len(gotAll))
	}

	gotStar, err := lister.ListResources(ctx, gvr, "*")
	if err != nil || len(gotStar) != 1 {
		t.Fatalf("* ns: err=%v len=%d", err, len(gotStar))
	}

	gotOther, err := lister.ListResources(ctx, gvr, "kube-system")
	if err != nil || len(gotOther) != 0 {
		t.Fatalf("wrong ns: err=%v len=%d", err, len(gotOther))
	}
}

func TestPolicyReconcilerAdapter_zeroValue(t *testing.T) {
	var adapter PolicyReconcilerAdapter
	if adapter.reconciler != nil {
		t.Error("expected nil reconciler on zero value")
	}
}

func TestNewPolicyReconcilerAdapter(t *testing.T) {
	adapter := NewPolicyReconcilerAdapter(nil)
	if adapter == nil {
		t.Fatal("Expected non-nil adapter")
	}
	if adapter.reconciler != nil {
		t.Error("Expected nil reconciler")
	}
}

func TestPolicyReconcilerAdapter_Delegates(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	fakeClient := clientfake.NewClientBuilder().WithScheme(scheme).Build()
	dynamicClient := fake.NewSimpleDynamicClient(scheme)
	statusUpdater := NewStatusUpdater(dynamicClient)
	eventRecorder := NewEventRecorder(nil)
	reconciler := NewPolicyReconcilerWithRESTMapper(
		fakeClient,
		scheme,
		dynamicClient,
		nil,
		statusUpdater,
		eventRecorder,
		config.NewControllerConfig(),
	)

	adapter := NewPolicyReconcilerAdapter(reconciler)
	res := &unstructured.Unstructured{}
	res.SetNamespace("default")
	target := &v1alpha1.TargetResourceSpec{}
	if !adapter.GetSelectorMatcher().MatchesSelectors(res, target) {
		t.Error("expected match for empty selector")
	}
	if !adapter.GetConditionMatcher().MeetsConditions(res, &v1alpha1.ConditionsSpec{}) {
		t.Error("expected empty conditions to pass")
	}

	policy := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "pol", Namespace: "default"},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			Behavior: v1alpha1.BehaviorSpec{DryRun: true},
		},
	}
	rl := adapter.GetRateLimiterProvider().GetOrCreateRateLimiter(policy)
	if rl == nil {
		t.Fatal("expected rate limiter")
	}

	ctx := context.Background()
	n, errs := adapter.GetBatchDeleter().DeleteBatch(ctx, []*unstructured.Unstructured{}, policy, ratelimiter.NewRateLimiter(10), map[string]string{})
	if n != 0 || len(errs) != 0 {
		t.Errorf("empty batch: n=%d errs=%v", n, errs)
	}
}
