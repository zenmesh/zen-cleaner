package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
)

func TestDefaultResourceLister_ListResources(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	client := fake.NewSimpleDynamicClient(scheme,
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
		},
	)

	ctx := context.Background()
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	lister := NewDefaultResourceLister(client)

	nsList, err := lister.ListResources(ctx, gvr, "default")
	if err != nil || len(nsList) != 1 {
		t.Fatalf("namespace list: err=%v len=%d", err, len(nsList))
	}

	allNs, err := lister.ListResources(ctx, gvr, "")
	if err != nil {
		t.Fatalf("cluster list: %v", err)
	}
	if len(allNs) != 1 {
		t.Fatalf("expected 1 pod cluster-wide, got %d", len(allNs))
	}
}

func TestDefaultMatchers_andRateLimiter(t *testing.T) {
	sm := NewDefaultSelectorMatcher()
	res := &unstructured.Unstructured{}
	res.SetNamespace("default")
	if !sm.MatchesSelectors(res, &v1alpha1.TargetResourceSpec{}) {
		t.Error("MatchesSelectors empty target")
	}

	cm := NewDefaultConditionMatcher()
	if !cm.MeetsConditions(res, &v1alpha1.ConditionsSpec{}) {
		t.Error("MeetsConditions empty")
	}

	p := NewDefaultRateLimiterProvider(config.NewControllerConfig())
	pol := &v1alpha1.ZenCleanerPolicy{}
	pol.UID = types.UID("test-uid")
	rl1 := p.GetOrCreateRateLimiter(pol)
	if rl1 == nil {
		t.Fatal("rate limiter")
	}
	if p.GetOrCreateRateLimiter(pol) != rl1 {
		t.Error("expected cached limiter")
	}
}

func TestDefaultResourceInformer_wrappers(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewSimpleDynamicClient(scheme)
	factory := dynamicinformer.NewDynamicSharedInformerFactory(client, 0)
	infFactory := NewDefaultResourceInformerFactory(factory)

	stopCh := make(chan struct{})
	go infFactory.Start(stopCh)
	defer close(stopCh)

	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	ri := infFactory.ForResource(gvr)
	if ri.GetStore() == nil {
		t.Error("expected store")
	}
	_ = ri.HasSynced()
	if _, err := ri.AddEventHandler(cache.ResourceEventHandlerFuncs{}); err != nil {
		t.Errorf("AddEventHandler: %v", err)
	}
}
