package controller

// TRUTHFUL nextGCRun LAW (H260 continuation): the status field is a PROMISE
// to operators. It must (a) reflect the POLICY's own evaluation interval —
// not the controller default — and (b) VANISH while the policy is paused,
// including stale promises written before the pause. The same shared law
// drives reconcile requeue, so status and behavior cannot diverge.

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
)

var policyGVR = schema.GroupVersionResource{
	Group: "cleaner.zen-mesh.io", Version: "v1alpha1", Resource: "zencleanerpolicies",
}

func createPolicy(t *testing.T, p *v1alpha1.ZenCleanerPolicy) *dynamicfake.FakeDynamicClient {
	t.Helper()
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme)
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Resource(policyGVR).Namespace(p.Namespace).Create(
		context.Background(), &unstructured.Unstructured{Object: obj}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	return client
}

func statusOf(t *testing.T, c *dynamicfake.FakeDynamicClient, ns, name string) map[string]interface{} {
	t.Helper()
	got, err := c.Resource(policyGVR).Namespace(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := got.Object["status"].(map[string]interface{})
	return st
}

// A per-policy interval must surface in nextGCRun — the controller default
// would misreport per-policy pacing.
func TestNextGCRunReflectsPolicyEvaluationInterval(t *testing.T) {
	p := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "slow-policy", Namespace: "ops"},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource:     v1alpha1.TargetResourceSpec{APIVersion: "v1", Kind: "ConfigMap"},
			EvaluationInterval: &metav1.Duration{Duration: 2 * time.Hour},
		},
	}
	c := createPolicy(t, p)
	updater := NewStatusUpdaterWithConfig(c, &config.ControllerConfig{CleanupInterval: time.Minute})
	if err := updater.UpdateStatus(context.Background(), p, 1, 0, 0, nil); err != nil {
		t.Fatal(err)
	}
	st := statusOf(t, c, "ops", "slow-policy")
	raw, ok := st["nextGCRun"].(string)
	if !ok {
		t.Fatalf("nextGCRun missing: %v", st)
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("nextGCRun not RFC3339: %v", err)
	}
	// ~2h in the future, NOT ~1m.
	until := time.Until(ts)
	if until < time.Hour || until > 3*time.Hour {
		t.Fatalf("nextGCRun must reflect the policy's 2h interval, got %s from now", until.Round(time.Minute))
	}
}

// Without a policy interval, the controller default is the honest value.
func TestNextGCRunFallsBackToControllerDefault(t *testing.T) {
	p := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default-policy", Namespace: "ops"},
		Spec:       v1alpha1.ZenCleanerPolicySpec{TargetResource: v1alpha1.TargetResourceSpec{APIVersion: "v1", Kind: "ConfigMap"}},
	}
	c := createPolicy(t, p)
	updater := NewStatusUpdaterWithConfig(c, &config.ControllerConfig{CleanupInterval: time.Minute})
	if err := updater.UpdateStatus(context.Background(), p, 0, 0, 0, nil); err != nil {
		t.Fatal(err)
	}
	st := statusOf(t, c, "ops", "default-policy")
	raw, _ := st["nextGCRun"].(string)
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("nextGCRun missing/invalid: %v (%v)", st["nextGCRun"], err)
	}
	if until := time.Until(ts); until > 5*time.Minute || until < 0 {
		t.Fatalf("default-interval promise must be ~now+1m, got %s", until.Round(time.Second))
	}
}

// Paused policies never run: no nextGCRun may exist — and a stale promise
// written before the pause must be REMOVED, not left to keep lying.
func TestPausedPolicyHasNoNextGCRunEvenStale(t *testing.T) {
	p := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "paused-policy", Namespace: "ops"},
		Spec:       v1alpha1.ZenCleanerPolicySpec{TargetResource: v1alpha1.TargetResourceSpec{APIVersion: "v1", Kind: "ConfigMap"}},
	}
	c := createPolicy(t, p)
	active := NewStatusUpdaterWithConfig(c, &config.ControllerConfig{CleanupInterval: time.Minute})
	if err := active.UpdateStatus(context.Background(), p, 0, 0, 0, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := statusOf(t, c, "ops", "paused-policy")["nextGCRun"]; !ok {
		t.Fatal("precondition: active policy must promise nextGCRun")
	}

	// The owner pauses; the next status write must clear the stale promise.
	p.Spec.Paused = true
	if err := active.UpdateStatus(context.Background(), p, 0, 0, 0, nil); err != nil {
		t.Fatal(err)
	}
	st := statusOf(t, c, "ops", "paused-policy")
	if _, exists := st["nextGCRun"]; exists {
		t.Fatalf("paused policy must not promise a next run; found %v", st["nextGCRun"])
	}
	if st["phase"] != PolicyPhasePaused {
		t.Fatalf("paused policy phase must be Paused, got %v", st["phase"])
	}
}

// The shared interval law: policy interval wins when set, default otherwise;
// nil policy is safe. Requeue scheduling and status reporting share it.
func TestEffectiveEvaluationIntervalLaw(t *testing.T) {
	def := 90 * time.Second
	if got := EffectiveEvaluationInterval(nil, def); got != def {
		t.Fatalf("nil policy must use default, got %s", got)
	}
	p := &v1alpha1.ZenCleanerPolicy{}
	if got := EffectiveEvaluationInterval(p, def); got != def {
		t.Fatalf("unset interval must use default, got %s", got)
	}
	zero := &v1alpha1.ZenCleanerPolicy{Spec: v1alpha1.ZenCleanerPolicySpec{
		EvaluationInterval: &metav1.Duration{Duration: 0},
	}}
	if got := EffectiveEvaluationInterval(zero, def); got != def {
		t.Fatalf("zero interval must use default (not stop the loop), got %s", got)
	}
	set := &v1alpha1.ZenCleanerPolicy{Spec: v1alpha1.ZenCleanerPolicySpec{
		EvaluationInterval: &metav1.Duration{Duration: 5 * time.Hour},
	}}
	if got := EffectiveEvaluationInterval(set, def); got != 5*time.Hour {
		t.Fatalf("policy interval must win, got %s", got)
	}
}
