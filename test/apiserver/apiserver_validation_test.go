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

// Package apiserver holds REAL Kubernetes API-server validation for the
// deletion-safety laws (SUPPORT2-033 §4): controller-runtime envtest starts
// a real kube-apiserver + etcd locally (no shared cluster, no user
// infrastructure, nothing persisted). Fake-client tests are NOT
// API-server evidence; these tests are.
//
// Execution model: the suite SKIPS (with the exact dependency named) when
// envtest binaries are unavailable or `go test -short` is used — CI runs
// hermetically without them. Local validation runs with
// KUBEBUILDER_ASSETS (or the setup-envtest default path) set.
//
// What a real API server adds over the reactor-modeled fakes: genuine
// UID/resourceVersion precondition enforcement (409 Conflict semantics),
// genuine generation bumping, and the real client call path end-to-end.
package apiserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/zenmesh/zen-cleaner/internal/ratelimiter"
	gcapi "github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"github.com/zenmesh/zen-cleaner/pkg/config"
	"github.com/zenmesh/zen-cleaner/pkg/controller"
	"github.com/zenmesh/zen-cleaner/pkg/safety"
)

var (
	cmGVR  = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	pcsGVR = schema.GroupVersionResource{Group: "scheduling.k8s.io", Version: "v1", Resource: "priorityclasses"}
)

// envtestAssets locates the kube-apiserver/etcd binaries: KUBEBUILDER_ASSETS
// first, then the setup-envtest default install path. Empty = unavailable.
func envtestAssets() string {
	if a := os.Getenv("KUBEBUILDER_ASSETS"); a != "" {
		if _, err := os.Stat(filepath.Join(a, "kube-apiserver")); err == nil {
			return a
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, v := range []string{"1.31.0-linux-amd64", "1.35.0-linux-amd64"} {
		p := filepath.Join(home, ".local", "share", "kubebuilder-envtest", "k8s", v)
		if fi, err := os.Stat(filepath.Join(p, "kube-apiserver")); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

type server struct {
	t          *testing.T
	env        *envtest.Environment
	dyn        dynamic.Interface
	ctrl       client.Client // UNCACHED direct client (APIReader semantics)
	reconciler *controller.PolicyReconciler
}

func newServer(t *testing.T) *server {
	t.Helper()
	if testing.Short() {
		t.Skip("short mode: real API-server validation requires envtest binaries (KUBEBUILDER_ASSETS / setup-envtest); fake-client tests are not API-server evidence")
	}
	assets := envtestAssets()
	if assets == "" {
		t.Skip("envtest binaries unavailable (dependency: run `setup-envtest use` or set KUBEBUILDER_ASSETS to a directory with kube-apiserver+etcd); skipped, NOT_RUN")
	}

	env := &envtest.Environment{
		BinaryAssetsDirectory: assets,
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "deploy", "crds")},
		CRDInstallOptions: envtest.CRDInstallOptions{
			MaxTime: 60 * time.Second,
		},
	}
	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("starting envtest (kube-apiserver+etcd): %v", err)
	}
	t.Cleanup(func() {
		_ = env.Stop()
	})

	sch := runtimeScheme()
	c, err := client.New(cfg, client.Options{Scheme: sch}) // direct, UNCACHED
	if err != nil {
		t.Fatalf("direct client: %v", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("dynamic client: %v", err)
	}

	cc := config.NewControllerConfig()
	cc.DeleteEnabled = true // envtest gate: emergency stop OFF for the validation
	r := controller.NewPolicyReconcilerWithRESTMapper(c, sch, dyn, nil, controller.NewStatusUpdater(dyn), controller.NewEventRecorder(nil), cc)
	// The direct client IS uncached (no manager, no informer cache) — the
	// APIReader semantics the delete-time policy-freshness check requires.
	r.SetAPIReader(c)

	return &server{t: t, env: env, dyn: dyn, ctrl: c, reconciler: r}
}

func runtimeScheme() *runtime.Scheme {
	sch := clientgoscheme.Scheme
	_ = gcapi.AddToScheme(sch)
	return sch
}

// delClass runs the REAL production delete path (deleteResource →
// performResourceDeletion, reached through the exported BatchDeleter
// method) against the real API server and classifies the outcome from the
// exported error contract: "deleted" / "absent" / a bounded refusal reason.
func (s *server) delClass(obj *unstructured.Unstructured, pol *gcapi.ZenCleanerPolicy) string {
	s.t.Helper()
	err := s.reconciler.DeleteResourceWithBackoff(s.t.Context(), obj, pol, ratelimiter.NewRateLimiter(100))
	var refused *controller.DeletionRefusedError
	switch {
	case err == nil:
		return "deleted"
	case errors.Is(err, controller.ErrAlreadyAbsent):
		return "absent"
	case errors.As(err, &refused):
		return refused.Reason
	default:
		s.t.Fatalf("unexpected delete-mechanism failure: %v", err)
		return ""
	}
}

// cm creates a namespaced ConfigMap fixture carrying the relative-TTL anchor
// (status.completedAt = fixed past date) and returns it.
func (s *server) cm(name string, mods ...func(*unstructured.Unstructured)) *unstructured.Unstructured {
	s.t.Helper()
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "default",
				// TTL anchor in an ANNOTATION: real API schemas prune unknown
				// fields (ConfigMap has no status), so the fake-style
				// status.completedAt anchor does not survive a real create.
				"annotations": map[string]interface{}{
					"completedAt": "2020-01-01T00:00:00Z",
				},
			},
		},
	}
	for _, m := range mods {
		m(obj)
	}
	got, err := s.dyn.Resource(cmGVR).Namespace("default").Create(s.t.Context(), obj, metav1.CreateOptions{})
	if err != nil {
		s.t.Fatalf("create configmap %s: %v", name, err)
	}
	return got
}

// pc creates a cluster-scoped PriorityClass fixture with the TTL anchor.
func (s *server) pc(name string) *unstructured.Unstructured {
	s.t.Helper()
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "scheduling.k8s.io/v1",
			"kind":       "PriorityClass",
			"metadata": map[string]interface{}{
				"name": name,
				"annotations": map[string]interface{}{
					"completedAt": "2020-01-01T00:00:00Z",
				},
			},
		},
	}
	got, err := s.dyn.Resource(pcsGVR).Create(s.t.Context(), obj, metav1.CreateOptions{})
	if err != nil {
		s.t.Fatalf("create priorityclass %s: %v", name, err)
	}
	return got
}

func (s *server) mustGet(gvr schema.GroupVersionResource, ns, name string) *unstructured.Unstructured {
	s.t.Helper()
	obj, err := s.dyn.Resource(gvr).Namespace(ns).Get(s.t.Context(), name, metav1.GetOptions{})
	if err != nil {
		s.t.Fatalf("get %s/%s: %v", ns, name, err)
	}
	return obj
}

func (s *server) assertGone(gvr schema.GroupVersionResource, ns, name string) {
	s.t.Helper()
	_, err := s.dyn.Resource(gvr).Namespace(ns).Get(s.t.Context(), name, metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		s.t.Fatalf("object %s/%s should be deleted, get err: %v", ns, name, err)
	}
}

// ttlPolicy builds a policy whose relative TTL is expired for the fixtures;
// values mirror the unit-test policy (status.completedAt + 1h, anchor 2020).
func (s *server) ttlPolicy(name string) *gcapi.ZenCleanerPolicy {
	return s.ttlPolicyFor(name, "v1", "ConfigMap")
}

// ttlPolicyFor builds AND PERSISTS a policy targeting the given kind (the
// policy's own target scope must match the fixture object for the
// delete-time eligibility re-evaluation).
func (s *server) ttlPolicyFor(name, apiVersion, kind string) *gcapi.ZenCleanerPolicy {
	s.t.Helper()
	hour := int64(3600)
	p := &gcapi.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name},
		Spec: gcapi.ZenCleanerPolicySpec{
			TargetResource: gcapi.TargetResourceSpec{
				APIVersion: apiVersion,
				Kind:       kind,
				Namespace:  "*",
			},
			TTL: gcapi.TTLSpec{
				RelativeTo:   "metadata.annotations.completedAt",
				SecondsAfter: &hour,
			},
			Behavior: gcapi.BehaviorSpec{PropagationPolicy: "Background"},
		},
	}
	if err := s.ctrl.Create(s.t.Context(), p); err != nil {
		s.t.Fatalf("create policy %s: %v", name, err)
	}
	// Read back to pick up server-assigned UID/generation.
	live := &gcapi.ZenCleanerPolicy{}
	if err := s.ctrl.Get(s.t.Context(), client.ObjectKey{Namespace: "default", Name: name}, live); err != nil {
		s.t.Fatalf("get policy %s: %v", name, err)
	}
	return live
}

func (s *server) deletePolicy(name string) {
	s.t.Helper()
	pol := &gcapi.ZenCleanerPolicy{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name}}
	if err := s.ctrl.Delete(s.t.Context(), pol); err != nil {
		s.t.Fatalf("delete policy %s: %v", name, err)
	}
}

// ── R1: successful deletion, namespaced AND cluster-scoped, real server ──

func TestRealAPIServer_SuccessfulDeletion_Preconditioned(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(s *server) (*unstructured.Unstructured, *gcapi.ZenCleanerPolicy)
		gvr  schema.GroupVersionResource
	}{
		{"namespaced", func(s *server) (*unstructured.Unstructured, *gcapi.ZenCleanerPolicy) {
			return s.cm("ok-ns"), s.ttlPolicy("pol-ns")
		}, cmGVR},
		{"cluster-scoped", func(s *server) (*unstructured.Unstructured, *gcapi.ZenCleanerPolicy) {
			obj := s.pc("ok-cluster")
			return obj, s.ttlPolicyFor("pol-cluster", "scheduling.k8s.io/v1", "PriorityClass")
		}, pcsGVR},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newServer(t)
			live, pol := tc.run(s)
			observed := live.DeepCopy()
			if class := s.delClass(observed, pol); class != "deleted" {
				t.Fatalf("outcome = %s, want deleted", class)
			}
			s.assertGone(tc.gvr, live.GetNamespace(), live.GetName())
		})
	}
}

// ── R2: the REAL API server enforces the exact preconditions the client
// sends (raw preconditioned DELETEs; deterministic, no race needed) ──────

func TestRealAPIServer_PreconditionsEnforcedByRealAPIServer(t *testing.T) {
	s := newServer(t)
	ctx := t.Context()
	live := s.cm("pre")

	// Wrong UID precondition → real 409, object intact.
	wrongUID := types.UID("uid-WRONG")
	err := s.dyn.Resource(cmGVR).Namespace("default").Delete(ctx, live.GetName(), metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &wrongUID},
	})
	if !apierrors.IsConflict(err) {
		t.Fatalf("wrong-UID precondition must be a real Conflict, got: %v", err)
	}
	s.mustGet(cmGVR, "default", live.GetName())

	// Correct UID but STALE resourceVersion (after a real same-UID update) →
	// real 409, object intact, UID unchanged.
	current := s.mustGet(cmGVR, "default", live.GetName())
	staleRV := current.GetResourceVersion()
	if serr := unstructured.SetNestedField(current.Object, "bump", "metadata", "annotations", "race.example/touched"); serr != nil {
		t.Fatalf("set annotation: %v", serr)
	}
	updated, uerr := s.dyn.Resource(cmGVR).Namespace("default").Update(ctx, current, metav1.UpdateOptions{})
	if uerr != nil {
		t.Fatalf("same-UID update: %v", uerr)
	}
	if updated.GetUID() != live.GetUID() {
		t.Fatal("metadata update must not change the UID")
	}
	correctUID := live.GetUID()
	err = s.dyn.Resource(cmGVR).Namespace("default").Delete(ctx, live.GetName(), metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &correctUID, ResourceVersion: &staleRV},
	})
	if !apierrors.IsConflict(err) {
		t.Fatalf("stale-RV precondition must be a real Conflict, got: %v", err)
	}
	sameUID := s.mustGet(cmGVR, "default", live.GetName())
	if sameUID.GetUID() != live.GetUID() {
		t.Fatal("object must survive a conflicted preconditioned delete with the same UID")
	}

	// Matching UID + evaluated RV → real success.
	fresh := s.mustGet(cmGVR, "default", live.GetName())
	freshRV := fresh.GetResourceVersion()
	err = s.dyn.Resource(cmGVR).Namespace("default").Delete(ctx, live.GetName(), metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &correctUID, ResourceVersion: &freshRV},
	})
	if err != nil {
		t.Fatalf("matching preconditions must delete: %v", err)
	}
	s.assertGone(cmGVR, "default", live.GetName())
}

// ── R3: replacement before GET (stale snapshot) through the real path ──

func TestRealAPIServer_ReplacementBeforeGET_Refused(t *testing.T) {
	s := newServer(t)
	ctx := t.Context()
	v1 := s.cm("repl")
	uidV1 := v1.GetUID()
	// Destroy and recreate: same name, NEW UID.
	_ = s.dyn.Resource(cmGVR).Namespace("default").Delete(ctx, "repl", metav1.DeleteOptions{})
	s.assertGone(cmGVR, "default", "repl")
	v2 := s.cm("repl")
	if v2.GetUID() == uidV1 {
		t.Fatal("fixture error: replacement must have a new UID")
	}

	stale := v1.DeepCopy() // decision made on the DEAD object
	if class := s.delClass(stale, s.ttlPolicy("pol-repl")); class != safety.ReasonStaleUID {
		t.Fatalf("outcome = %s, want refused/stale_uid", class)
	}
	if s.mustGet(cmGVR, "default", "repl").GetUID() != v2.GetUID() {
		t.Fatal("replacement was mutated")
	}
}

// ── R4: the REAL GET→DELETE race — concurrent same-UID updates ──────────
// Deterministic per-round invariants: either the delete succeeded (object
// gone) or the real server returned 409 (outcome precondition_conflict,
// object present with the SAME UID). A replacement/rv-bump in the window
// can never be destroyed by name.

func TestRealAPIServer_ConcurrentSameUIDUpdates_NeverUnsafeDelete(t *testing.T) {
	s := newServer(t)
	ctx := t.Context()
	pol := s.ttlPolicy("pol-race")

	updCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	updater := func(name string) {
		defer wg.Done()
		i := 0
		for {
			select {
			case <-updCtx.Done():
				return
			default:
			}
			cur, err := s.dyn.Resource(cmGVR).Namespace("default").Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				continue // deleted: updater loses; race round over
			}
			_ = unstructured.SetNestedField(cur.Object, fmt.Sprintf("u-%d", i), "metadata", "annotations", "race.example/iter") // annotation shape is fixed; error cannot occur
			i++
			_, _ = s.dyn.Resource(cmGVR).Namespace("default").Update(ctx, cur, metav1.UpdateOptions{})
			time.Sleep(time.Millisecond)
		}
	}

	rounds := 25
	stats := map[string]int{}
	for r := 0; r < rounds; r++ {
		name := fmt.Sprintf("race-%d", r)
		obj := s.cm(name)
		observed := obj.DeepCopy()
		wg.Add(1)
		go updater(name)
		class := s.delClass(observed, pol)
		cancel() // stop the updater for this round
		wg.Wait()
		switch class {
		case "deleted":
			stats["deleted"]++
			s.assertGone(cmGVR, "default", name) // gone: no updater can revive the UID
		case safety.ReasonPreconditionConflict:
			stats["conflict"]++
			live := s.mustGet(cmGVR, "default", name)
			if live.GetUID() != obj.GetUID() {
				t.Fatalf("round %d: object mutated after a conflicted delete", r)
			}
		case "absent":
			stats["absent"]++
		default:
			t.Fatalf("round %d: unexpected outcome %s", r, class)
		}
		// Clean up survivors for the next round.
		_ = s.dyn.Resource(cmGVR).Namespace("default").Delete(ctx, name, metav1.DeleteOptions{})
		s.assertGone(cmGVR, "default", name)
		updCtx, cancel = context.WithCancel(ctx)
		defer func() { cancel() }()
	}
	if stats["deleted"]+stats["conflict"]+stats["absent"] != rounds {
		t.Fatalf("accounting mismatch: %+v", stats)
	}
	if stats["conflict"] == 0 {
		t.Logf("NOTE: no 409 landed in the window this run (stats=%+v); the deterministic 409 law is proven by R2", stats)
	}
	t.Logf("real-server race stats over %d rounds: %+v", rounds, stats)
}

// ── R5: exclusion label at delete time, real gate ────────────────────────

func TestRealAPIServer_ExclusionLabel_Refused(t *testing.T) {
	s := newServer(t)
	live := s.cm("excluded", func(o *unstructured.Unstructured) {
		o.SetLabels(map[string]string{safety.ExclusionLabelKey: "true"})
	})
	observed := live.DeepCopy()
	observed.SetLabels(nil) // the BATCH decision saw no label (stale cache view)
	if class := s.delClass(observed, s.ttlPolicy("pol-excluded")); class != safety.ReasonExcludedLabel {
		t.Fatalf("outcome = %s, want refused/excluded_label", class)
	}
	s.mustGet(cmGVR, "default", "excluded") // intact
}

// ── R6: the second delete route — DeleteWithReceipt on the real server ──

func TestRealAPIServer_DeleteWithReceipt_Laws(t *testing.T) {
	s := newServer(t)
	ctx := t.Context()

	// Matching UID → real SUCCEEDED.
	live := s.cm("r6-hit")
	matching := *live.DeepCopy()
	rc := controller.DeleteWithReceipt(ctx, s.ctrl, []unstructured.Unstructured{matching})
	if rc.Results[0].Class != controller.ResultSucceeded {
		t.Fatalf("matching-UID receipt = %+v, want SUCCEEDED", rc.Results)
	}
	s.assertGone(cmGVR, "default", "r6-hit")

	// Stale plan (replacement carries a new UID) → SKIPPED, survivor intact.
	repl := s.cm("r6-repl")
	ghost := *repl.DeepCopy()
	ghost.SetUID(types.UID("uid-OLD"))
	rc = controller.DeleteWithReceipt(ctx, s.ctrl, []unstructured.Unstructured{ghost})
	if rc.Results[0].Class != controller.ResultSkippedPreconditionMismatch {
		t.Fatalf("stale-plan receipt = %+v, want SKIPPED_PRECONDITION_MISMATCH", rc.Results)
	}
	if s.mustGet(cmGVR, "default", "r6-repl").GetUID() != repl.GetUID() {
		t.Fatal("replacement mutated")
	}

	// Already absent → ALREADY_ABSENT.
	gone := *repl.DeepCopy()
	gone.SetName("r6-absent")
	rc = controller.DeleteWithReceipt(ctx, s.ctrl, []unstructured.Unstructured{gone})
	if rc.Results[0].Class != controller.ResultAlreadyAbsent {
		t.Fatalf("absent receipt = %+v, want ALREADY_ABSENT", rc.Results)
	}

	// Empty planned UID → SKIPPED, nothing read or deleted.
	noUID := *repl.DeepCopy()
	noUID.SetUID(types.UID(""))
	rc = controller.DeleteWithReceipt(ctx, s.ctrl, []unstructured.Unstructured{noUID})
	if rc.Results[0].Class != controller.ResultSkippedPreconditionMismatch {
		t.Fatalf("empty-UID receipt = %+v, want SKIPPED_PRECONDITION_MISMATCH", rc.Results)
	}
	s.mustGet(cmGVR, "default", "r6-repl")
}

// ── R7: policy deletion/spec change mid-flight (uncached freshness check) ──

func TestRealAPIServer_PolicyChanged_Refused(t *testing.T) {
	s := newServer(t)

	live := s.cm("r7")
	observed := live.DeepCopy()
	pol := s.ttlPolicy("pol-r7a")

	// Policy DELETED after the decision: refuse, object intact.
	s.deletePolicy("pol-r7a")
	if class := s.delClass(observed, pol); class != safety.ReasonPolicyChanged {
		t.Fatalf("outcome = %s, want refused/policy_changed", class)
	}
	s.mustGet(cmGVR, "default", "r7")

	// Policy RECREATED (new UID) — the old snapshot's UID no longer exists.
	pol2 := s.ttlPolicy("pol-r7b")
	_ = pol2
	if class := s.delClass(observed, pol); class != safety.ReasonPolicyChanged {
		t.Fatalf("outcome = %s, want policy_changed (snapshot UID belongs to the deleted object)", class)
	}
	s.mustGet(cmGVR, "default", "r7")

	// SPEC EDIT after the snapshot: a real update moves metadata.generation
	// server-side (status subresource) — the stale snapshot refuses.
	pol3 := s.ttlPolicy("pol-r7c")
	pol3.Spec.TTL.SecondsAfter = ptrInt64(7200)
	if err := s.ctrl.Update(s.t.Context(), pol3); err != nil {
		t.Fatalf("policy spec edit: %v", err)
	}
	if pol3.Generation == 0 {
		t.Fatal("fixture error: expected a generation on the policy snapshot")
	}
	stale := pol3.DeepCopy()
	stale.Generation-- // the snapshot from before the edit
	if class := s.delClass(observed, stale); class != safety.ReasonPolicyChanged {
		t.Fatalf("outcome = %s, want policy_changed (generation moved)", class)
	}
	s.mustGet(cmGVR, "default", "r7")

	// Unchanged policy: deletes for real.
	if class := s.delClass(observed, pol3); class != "deleted" {
		t.Fatalf("outcome = %s, want deleted", class)
	}
	s.assertGone(cmGVR, "default", "r7")
}

func ptrInt64(i int64) *int64 { return &i }
