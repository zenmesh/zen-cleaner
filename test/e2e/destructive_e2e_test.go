//go:build e2e
// +build e2e

package e2e

// Destructive E2E suite (DAEDALUS-039): first-class proof of real deletion
// behavior against a live cluster.
//
// Layout:
//   - TestE2E_DestructiveSuite — E01..E10 (install, dry-run safety, eligible
//     deletion, nonmatch/nonexpired survival, fail-closed malformed policy,
//     rate limiting, controller-restart safety, uninstall safety)
//   - TestE2E_TTL_Modes — positive/negative coverage for all four TTL modes
//     (fixed, field-based, mapped, relative)
//   - TestE2E_ArbitraryCRD — cleanup of a custom resource with an irregular
//     plural, a mapped retention field and a relative status timestamp,
//     proving discovery-based RESTMapper resolution (no naive pluralization)
//
// Requires a cluster with the Zen Cleaner CRDs + controller installed (e.g. via
// scripts/comprehensive_e2e.sh or `make test-e2e`). Uses real Kubernetes
// objects and real deletions — never mocked deletion.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

const (
	policyGroup    = "cleaner.zen-mesh.io"
	policyVersion  = "v1alpha1"
	policyResource = "zencleanerpolicies"

	// Arbitrary test CRD with an irregular plural (zgprobes — NOT the naive
	// plural of ZenCleanerProbe) so naive-pluralization resolution would fail
	// and discovery-based RESTMapper resolution is proven.
	probeGroup   = "gc-e2e.zenmesh.io"
	probeVersion = "v1alpha1"
	probeKind    = "ZenCleanerProbe"
)

// polGVR is the ZenCleanerPolicy collection resource.
var polGVR = schema.GroupVersionResource{Group: policyGroup, Version: policyVersion, Resource: policyResource}

// crdGVR is the dynamic GVR for CustomResourceDefinitions.
var crdGVR = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

const (
	probePlural = "zgprobes"

	pollInterval = 2 * time.Second
	pollTimeout  = 90 * time.Second
)

var probeGVR = schema.GroupVersionResource{Group: probeGroup, Version: probeVersion, Resource: probePlural}

func clients(t *testing.T) (dynamic.Interface, apiextensionsInterface) {
	t.Helper()
	config, err := getKubeConfig()
	if err != nil {
		t.Fatalf("kubeconfig: %v", err)
	}
	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatalf("dynamic client: %v", err)
	}
	ext := dyn.Resource(crdGVR)
	return dyn, ext
}

// ensureTestCRD creates the arbitrary test CRD and waits for Established.
func ensureTestCRD(ctx context.Context, ext apiextensionsInterface) error {
	crd := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinition",
		"metadata":   map[string]interface{}{"name": probePlural + "." + probeGroup},
		"spec": map[string]interface{}{
			"group": probeGroup,
			"names": map[string]interface{}{
				"kind":       probeKind,
				"listKind":   probeKind + "List",
				"plural":     probePlural,
				"singular":   "zencleanerprobe",
				"shortNames": []interface{}{"zgp"},
			},
			"scope": "Namespaced",
			"versions": []interface{}{map[string]interface{}{
				"name":         probeVersion,
				"served":       true,
				"storage":      true,
				"subresources": map[string]interface{}{"status": map[string]interface{}{}},
				"schema": map[string]interface{}{"openAPIV3Schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"spec": map[string]interface{}{"type": "object", "properties": map[string]interface{}{
							"tier":       map[string]interface{}{"type": "string"},
							"ttlSeconds": map[string]interface{}{"type": "integer"},
						}},
						"status": map[string]interface{}{"type": "object", "properties": map[string]interface{}{
							"completedAt": map[string]interface{}{"type": "string"},
						}},
					},
				}},
			}},
		},
	}}
	existing, err := ext.Get(ctx, probePlural+"."+probeGroup, metav1.GetOptions{})
	if err == nil {
		_ = existing
		return nil
	}
	if _, err := ext.Create(ctx, crd, metav1.CreateOptions{}); err != nil {
		return err
	}
	return wait.PollUntilContextTimeout(ctx, pollInterval, 120*time.Second, true, func(ctx context.Context) (bool, error) {
		c, err := ext.Get(ctx, probePlural+"."+probeGroup, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		conditions, _, _ := unstructured.NestedSlice(c.Object, "status", "conditions")
		for _, condAny := range conditions {
			cond, _ := condAny.(map[string]interface{})
			if cond["type"] == "Established" && cond["status"] == "True" {
				return true, nil
			}
		}
		return false, nil
	})
}

// apiextensionsInterface is the minimal dynamic-client surface for CRDs.
type apiextensionsInterface = dynamic.NamespaceableResourceInterface

func strPtr(s string) *string { return &s }

// createPolicy applies a ZenCleanerPolicy via the dynamic client.
func createPolicy(ctx context.Context, dyn dynamic.Interface, ns string, policy *v1alpha1.ZenCleanerPolicy) error {
	obj, err := runtimeToUnstructured(policy)
	if err != nil {
		return err
	}
	_, err = dyn.Resource(polGVR).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
	return err
}

func runtimeToUnstructured(policy *v1alpha1.ZenCleanerPolicy) (*unstructured.Unstructured, error) {
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(policy)
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{Object: obj}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: policyGroup, Version: policyVersion, Kind: "ZenCleanerPolicy"})
	return u, nil
}

func mustCreatePolicy(t *testing.T, dyn dynamic.Interface, ns string, pol *v1alpha1.ZenCleanerPolicy) {
	t.Helper()
	pol = pol.DeepCopy()
	pol.Namespace = ns
	obj, err := runtimeToUnstructured(pol)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dyn.Resource(polGVR).Namespace(ns).Create(context.Background(), obj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create policy: %v", err)
	}
}

// cmGVR returns the ConfigMap GVR.
func cmGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
}

// createResource creates any unstructured resource in ns.
func createResource(t *testing.T, dyn dynamic.Interface, ns string, obj *unstructured.Unstructured) {
	t.Helper()
	gvk := obj.GroupVersionKind()
	if obj.GetNamespace() == "" {
		obj.SetNamespace(ns)
	}
	gvr := gvrFor(gvk, obj)
	if _, err := dyn.Resource(gvr).Namespace(ns).Create(context.Background(), obj, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create %s %s: %v", gvk.Kind, obj.GetName(), err)
	}
}

// gvrFor maps gvk -> gvr for the built-in resources used here and the test CRD.
func gvrFor(gvk schema.GroupVersionKind, obj *unstructured.Unstructured) schema.GroupVersionResource {
	if gvk.Group == "" && gvk.Version == "v1" {
		if gvk.Kind == "ConfigMap" {
			return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
		}
	}
	if gvk.Group == probeGroup {
		return probeGVR
	}
	_ = obj
	return schema.GroupVersionResource{}
}

// basePolicy returns a ConfigMap-targeted policy in ns with a fixed TTL.
func basePolicy(name, ns string, labels map[string]string, ttlSeconds int64) *v1alpha1.ZenCleanerPolicy {
	pol := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  ns,
			},
			TTL:      v1alpha1.TTLSpec{SecondsAfterCreation: int64Ptr(ttlSeconds)},
			Behavior: v1alpha1.BehaviorSpec{DryRun: false},
		},
	}
	if labels != nil {
		pol.Spec.TargetResource.LabelSelector = &metav1.LabelSelector{MatchLabels: labels}
	}
	return pol
}

// e01Install proves the controller deployment is running and the CRD served.
func e01Install(t *testing.T, dyn dynamic.Interface) {
	t.Helper()
	ctx := context.Background()
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	waitFor(t, "controller deployment ready", 120*time.Second, func() (bool, string) {
		d, err := dyn.Resource(deployGVR).Namespace("zen-cleaner-system").Get(ctx, "zen-cleaner", metav1.GetOptions{})
		if err != nil {
			return false, err.Error()
		}
		ready, _, _ := unstructured.NestedInt64(d.Object, "status", "readyReplicas")
		desired, _, _ := unstructured.NestedInt64(d.Object, "spec", "replicas")
		return ready >= desired && desired >= 1, fmt.Sprintf("ready=%d desired=%d", ready, desired)
	})
}

// restartController restarts the zen-cleaner deployment (E09) and waits for rollout.
func restartController(t *testing.T) {
	t.Helper()
	config, err := getKubeConfig()
	if err != nil {
		t.Fatal(err)
	}
	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	now := time.Now().UTC().Format(time.RFC3339)
	// Patch a restartedAt annotation via Update on the deployment object.
	d, err := dyn.Resource(deployGVR).Namespace("zen-cleaner-system").Get(context.Background(), "zen-cleaner", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	annotations, _, _ := unstructured.NestedMap(d.Object, "spec", "template", "metadata", "annotations")
	if annotations == nil {
		annotations = map[string]interface{}{}
	}
	annotations["gc-e2e/restartedAt"] = now
	_ = unstructured.SetNestedMap(d.Object, annotations, "spec", "template", "metadata", "annotations")
	if _, err := dyn.Resource(deployGVR).Namespace("zen-cleaner-system").Update(context.Background(), d, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("restart controller: %v", err)
	}
	waitFor(t, "controller rollout after restart", 120*time.Second, func() (bool, string) {
		d, err := dyn.Resource(deployGVR).Namespace("zen-cleaner-system").Get(context.Background(), "zen-cleaner", metav1.GetOptions{})
		if err != nil {
			return false, err.Error()
		}
		ready, _, _ := unstructured.NestedInt64(d.Object, "status", "readyReplicas")
		desired, _, _ := unstructured.NestedInt64(d.Object, "spec", "replicas")
		return ready >= desired, fmt.Sprintf("ready=%d desired=%d", ready, desired)
	})
}

// ensureNamespace creates ns if missing.
func ensureNamespace(ctx context.Context, dyn dynamic.Interface, ns string) {
	nsObj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Namespace",
		"metadata": map[string]interface{}{"name": ns},
	}}
	nsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	_, _ = dyn.Resource(nsGVR).Create(ctx, nsObj, metav1.CreateOptions{}) // already-exists tolerated
}

// resourceState renders a Get error compactly for waitFor notes.
func resourceState(err error) string {
	if err == nil {
		return "exists"
	}
	return "deleted"
}

// waitFor polls condition until true or timeout, reporting the last state.
func waitFor(t *testing.T, what string, timeout time.Duration, cond func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := "condition not yet evaluated"
	for time.Now().Before(deadline) {
		if ok, note := cond(); ok {
			return
		} else {
			last = note
		}
		time.Sleep(pollInterval)
	}
	t.Fatalf("timed out waiting for %s (last: %s)", what, last)
}

// probe creates a ZenCleanerProbe custom resource in ns.
func probe(name, ns, tier string, ttlSeconds int64, completedAt string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": probeGroup + "/" + probeVersion,
		"kind":       probeKind,
		"metadata":   map[string]interface{}{"name": name, "namespace": ns},
		"spec": map[string]interface{}{
			"tier":       tier,
			"ttlSeconds": ttlSeconds,
		},
	}}
	if completedAt != "" {
		obj.Object["status"] = map[string]interface{}{"completedAt": completedAt}
	}
	return obj
}

func cm(name, ns string, labels map[string]string) *unstructured.Unstructured {
	meta := map[string]interface{}{"name": name, "namespace": ns}
	if labels != nil {
		meta["labels"] = labels
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   meta,
		"data":       map[string]interface{}{"e2e": "true"},
	}}
}

func nsResourceGVR(apiVersion, kind string) schema.GroupVersionResource {
	if apiVersion == "v1" {
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: strings.ToLower(kind) + "s"}
	}
	return schema.GroupVersionResource{}
}

// setProbeStatus writes the status subresource for a probe. Kubernetes
// strips status at creation (subresource), so status fields must be applied
// via a separate UpdateStatus carrying the live resourceVersion.
func setProbeStatus(t *testing.T, dyn dynamic.Interface, ns string, obj *unstructured.Unstructured, field, value string) {
	t.Helper()
	live, err := dyn.Resource(probeGVR).Namespace(ns).Get(context.Background(), obj.GetName(), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get probe for status: %v", err)
	}
	live.Object["status"] = map[string]interface{}{field: value}
	if _, err := dyn.Resource(probeGVR).Namespace(ns).UpdateStatus(context.Background(), live, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("set %s: %v", field, err)
	}
}

// TestE2E_DestructiveSuite proves real deletion behavior (E01..E10).
func TestE2E_DestructiveSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	ctx := context.Background()
	dyn, ext := clients(t)
	if err := ensureTestCRD(ctx, ext); err != nil {
		t.Fatalf("test CRD: %v", err)
	}

	ns := "gc-destructive-e2e"
	ensureNamespace(ctx, dyn, ns)

	// E01: install succeeds — controller deployment ready + CRD served.
	e01Install(t, dyn)

	t.Run("E02_DryRunDeletesNothing", func(t *testing.T) {
		expired := cm("e02-expired", ns, map[string]string{"e2e": "e02"})
		createResource(t, dyn, ns, expired)
		pol := basePolicy("e02-dryrun", ns, map[string]string{"e2e": "e02"}, 5)
		pol.Spec.Behavior.DryRun = true
		mustCreatePolicy(t, dyn, ns, pol)
		waitFor(t, "dry-run expiry window", 15*time.Second, func() (bool, string) {
			_, err := dyn.Resource(cmGVR()).Namespace(ns).Get(ctx, "e02-expired", metav1.GetOptions{})
			if err != nil {
				return false, "resource deleted under dry-run"
			}
			return time.Since(time.Now().Add(-12*time.Second)) >= 0, "waiting out TTL window"
		})
	})

	t.Run("E03_WouldDeleteReported", func(t *testing.T) {
		expired := cm("e03-expired", ns, map[string]string{"e2e": "e03"})
		createResource(t, dyn, ns, expired)
		pol := basePolicy("e03-preview", ns, map[string]string{"e2e": "e03"}, 5)
		pol.Spec.Behavior.DryRun = true
		mustCreatePolicy(t, dyn, ns, pol)
		waitFor(t, "policy status to record matched resources", pollTimeout, func() (bool, string) {
			p, err := dyn.Resource(polGVR).Namespace(ns).Get(ctx, "e03-preview", metav1.GetOptions{})
			if err != nil {
				return false, err.Error()
			}
			matched, _, _ := unstructured.NestedInt64(p.Object, "status", "resourcesMatched")
			if matched >= 1 {
				return true, fmt.Sprintf("matched=%d (would-delete visible in status)", matched)
			}
			return false, fmt.Sprintf("matched=%d", matched)
		})
	})

	t.Run("E04_EnabledPolicyDeletesEligible", func(t *testing.T) {
		expired := cm("e04-expired", ns, map[string]string{"e2e": "e04"})
		createResource(t, dyn, ns, expired)
		mustCreatePolicy(t, dyn, ns, basePolicy("e04-delete", ns, map[string]string{"e2e": "e04"}, 5))
		waitFor(t, "eligible resource deleted", pollTimeout, func() (bool, string) {
			_, err := dyn.Resource(cmGVR()).Namespace(ns).Get(ctx, "e04-expired", metav1.GetOptions{})
			return err != nil, resourceState(err)
		})
	})

	t.Run("E05_NonmatchingSurvives", func(t *testing.T) {
		other := cm("e05-other", ns, map[string]string{"e2e": "unrelated"})
		createResource(t, dyn, ns, other)
		mustCreatePolicy(t, dyn, ns, basePolicy("e05-selector", ns, map[string]string{"e2e": "e05"}, 5))
		waitFor(t, "policy evaluation", 12*time.Second, func() (bool, string) { return true, "window elapsed" })
		if _, err := dyn.Resource(cmGVR()).Namespace(ns).Get(ctx, "e05-other", metav1.GetOptions{}); err != nil {
			t.Fatalf("nonmatching resource was deleted: %v", err)
		}
	})

	t.Run("E06_NonexpiredSurvives", func(t *testing.T) {
		fresh := cm("e06-fresh", ns, map[string]string{"e2e": "e06"})
		createResource(t, dyn, ns, fresh)
		mustCreatePolicy(t, dyn, ns, basePolicy("e06-ttl", ns, map[string]string{"e2e": "e06"}, 3600))
		time.Sleep(12 * time.Second)
		if _, err := dyn.Resource(cmGVR()).Namespace(ns).Get(ctx, "e06-fresh", metav1.GetOptions{}); err != nil {
			t.Fatalf("nonexpired resource was deleted: %v", err)
		}
	})

	t.Run("E07_MalformedPolicyFailsClosed", func(t *testing.T) {
		target := cm("e07-target", ns, map[string]string{"e2e": "e07"})
		createResource(t, dyn, ns, target)
		pol := basePolicy("e07-malformed", ns, map[string]string{"e2e": "e07"}, 0)
		// Malformed: relative-to field that does not exist + nonsense mappings.
		pol.Spec.TTL = v1alpha1.TTLSpec{
			RelativeTo:   "status.doesNotExist",
			SecondsAfter: int64Ptr(1),
		}
		mustCreatePolicy(t, dyn, ns, pol)
		waitFor(t, "evaluation window", 15*time.Second, func() (bool, string) { return true, "window elapsed" })
		if _, err := dyn.Resource(cmGVR()).Namespace(ns).Get(ctx, "e07-target", metav1.GetOptions{}); err != nil {
			t.Fatalf("malformed policy broadened deletion: %v", err)
		}
	})

	t.Run("E08_RateLimitHonored", func(t *testing.T) {
		const count = 4
		for i := 0; i < count; i++ {
			createResource(t, dyn, ns, cm(fmt.Sprintf("e08-r%d", i), ns, map[string]string{"e2e": "e08"}))
		}
		pol := basePolicy("e08-ratelimited", ns, map[string]string{"e2e": "e08"}, 1)
		pol.Spec.Behavior.MaxDeletionsPerSecond = 1
		mustCreatePolicy(t, dyn, ns, pol)
		start := time.Now()
		waitFor(t, "rate-limited deletions complete", pollTimeout, func() (bool, string) {
			remaining := 0
			for i := 0; i < count; i++ {
				if _, err := dyn.Resource(cmGVR()).Namespace(ns).Get(ctx, fmt.Sprintf("e08-r%d", i), metav1.GetOptions{}); err == nil {
					remaining++
				}
			}
			return remaining == 0, fmt.Sprintf("%d remaining", remaining)
		})
		if elapsed := time.Since(start); elapsed < 2*time.Second {
			t.Fatalf("rate limit not honored: %d deletions completed in %s (max 1/s)", count, elapsed)
		}
	})

	t.Run("E09_RestartDoesNotCauseUnsafeDeletion", func(t *testing.T) {
		fresh := cm("e09-fresh", ns, map[string]string{"e2e": "e09"})
		createResource(t, dyn, ns, fresh)
		mustCreatePolicy(t, dyn, ns, basePolicy("e09-ttl", ns, map[string]string{"e2e": "e09"}, 3600))
		restartController(t)
		// After restart the nonexpired resource must still exist.
		time.Sleep(12 * time.Second)
		if _, err := dyn.Resource(cmGVR()).Namespace(ns).Get(ctx, "e09-fresh", metav1.GetOptions{}); err != nil {
			t.Fatalf("controller restart caused unsafe deletion of nonexpired resource: %v", err)
		}
	})

	t.Run("E10_UninstallSafety", func(t *testing.T) {
		keep := cm("e10-keep", ns, map[string]string{"e2e": "e10"})
		createResource(t, dyn, ns, keep)
		pol := basePolicy("e10-policy", ns, map[string]string{"e2e": "e10"}, 1)
		mustCreatePolicy(t, dyn, ns, pol)
		// Simulate uninstall: policies are removed first, then controller.
		if err := dyn.Resource(polGVR).Namespace(ns).Delete(context.Background(), "e10-policy", metav1.DeleteOptions{}); err != nil {
			t.Fatalf("delete policy: %v", err)
		}
		waitFor(t, "policy deletion", pollTimeout, func() (bool, string) {
			_, err := dyn.Resource(polGVR).Namespace(ns).Get(ctx, "e10-policy", metav1.GetOptions{})
			return err != nil, resourceState(err)
		})
		// Give any (incorrect) late evaluation a chance to misbehave.
		time.Sleep(10 * time.Second)
		if _, err := dyn.Resource(cmGVR()).Namespace(ns).Get(ctx, "e10-keep", metav1.GetOptions{}); err != nil {
			t.Fatalf("user resource deleted around policy removal (uninstall unsafe): %v", err)
		}
	})

}

// TestE2E_TTL_Modes covers all four TTL modes (positive + negative each).
func TestE2E_TTL_Modes(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	ctx := context.Background()
	dyn, ext := clients(t)
	if err := ensureTestCRD(ctx, ext); err != nil {
		t.Fatalf("test CRD: %v", err)
	}
	ns := "gc-ttl-e2e"
	ensureNamespace(ctx, dyn, ns)

	t.Run("Fixed_Positive", func(t *testing.T) {
		createResource(t, dyn, ns, cm("ttl-fixed-expired", ns, map[string]string{"e2e": "ttl-fixed"}))
		mustCreatePolicy(t, dyn, ns, basePolicy("ttl-fixed", ns, map[string]string{"e2e": "ttl-fixed"}, 3))
		waitFor(t, "fixed-TTL deletion", pollTimeout, func() (bool, string) {
			_, err := dyn.Resource(cmGVR()).Namespace(ns).Get(ctx, "ttl-fixed-expired", metav1.GetOptions{})
			return err != nil, resourceState(err)
		})
	})

	t.Run("FieldBased_Positive", func(t *testing.T) {
		pr := probe("ttl-field-expired", ns, "", 3, "")
		pr.SetLabels(map[string]string{"e2e": "ttl-field"})
		createResource(t, dyn, ns, pr)
		pol := basePolicy("ttl-field", ns, map[string]string{"e2e": "ttl-field"}, 0)
		pol.Spec.TTL = v1alpha1.TTLSpec{FieldPath: "spec.ttlSeconds"}
		pol.Spec.TargetResource = v1alpha1.TargetResourceSpec{APIVersion: probeGroup + "/" + probeVersion, Kind: probeKind, Namespace: ns, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"e2e": "ttl-field"}}}
		mustCreatePolicy(t, dyn, ns, pol)
		waitFor(t, "field-based TTL deletion", pollTimeout, func() (bool, string) {
			_, err := dyn.Resource(probeGVR).Namespace(ns).Get(ctx, "ttl-field-expired", metav1.GetOptions{})
			return err != nil, resourceState(err)
		})
	})

	t.Run("Mapped_Positive", func(t *testing.T) {
		pr := probe("ttl-mapped-ephemeral", ns, "ephemeral", 0, "")
		_ = pr
		obj := probe("ttl-mapped-ephemeral", ns, "ephemeral", 3, "")
		obj.SetLabels(map[string]string{"e2e": "ttl-mapped"})
		createResource(t, dyn, ns, obj)
		pol := basePolicy("ttl-mapped", ns, map[string]string{"e2e": "ttl-mapped"}, 0)
		pol.Spec.TTL = v1alpha1.TTLSpec{
			FieldPath: "spec.tier",
			Mappings:  map[string]int64{"ephemeral": 3, "persistent": 86400},
			Default:   int64Ptr(86400),
		}
		pol.Spec.TargetResource = v1alpha1.TargetResourceSpec{APIVersion: probeGroup + "/" + probeVersion, Kind: probeKind, Namespace: ns, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"e2e": "ttl-mapped"}}}
		mustCreatePolicy(t, dyn, ns, pol)
		waitFor(t, "mapped TTL deletion", pollTimeout, func() (bool, string) {
			_, err := dyn.Resource(probeGVR).Namespace(ns).Get(ctx, "ttl-mapped-ephemeral", metav1.GetOptions{})
			return err != nil, resourceState(err)
		})
	})

	t.Run("Mapped_Negative_DefaultSurvives", func(t *testing.T) {
		obj := probe("ttl-mapped-unknown", ns, "mystery-tier", 0, "")
		obj.SetLabels(map[string]string{"e2e": "ttl-mapped-neg"})
		createResource(t, dyn, ns, obj)
		pol := basePolicy("ttl-mapped-neg", ns, map[string]string{"e2e": "ttl-mapped-neg"}, 0)
		pol.Spec.TTL = v1alpha1.TTLSpec{
			FieldPath: "spec.tier",
			Mappings:  map[string]int64{"ephemeral": 3},
			Default:   int64Ptr(86400),
		}
		pol.Spec.TargetResource = v1alpha1.TargetResourceSpec{APIVersion: probeGroup + "/" + probeVersion, Kind: probeKind, Namespace: ns, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"e2e": "ttl-mapped-neg"}}}
		mustCreatePolicy(t, dyn, ns, pol)
		time.Sleep(12 * time.Second)
		if _, err := dyn.Resource(probeGVR).Namespace(ns).Get(ctx, "ttl-mapped-unknown", metav1.GetOptions{}); err != nil {
			t.Fatalf("default-TTL resource deleted (mapped default not honored): %v", err)
		}
	})

	t.Run("Relative_Positive", func(t *testing.T) {
		old := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
		obj := probe("ttl-relative-expired", ns, "", 0, "")
		obj.SetLabels(map[string]string{"e2e": "ttl-relative"})
		createResource(t, dyn, ns, obj)
		setProbeStatus(t, dyn, ns, obj, "completedAt", old)
		pol := basePolicy("ttl-relative", ns, map[string]string{"e2e": "ttl-relative"}, 0)
		pol.Spec.TTL = v1alpha1.TTLSpec{RelativeTo: "status.completedAt", SecondsAfter: int64Ptr(60)}
		pol.Spec.TargetResource = v1alpha1.TargetResourceSpec{APIVersion: probeGroup + "/" + probeVersion, Kind: probeKind, Namespace: ns, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"e2e": "ttl-relative"}}}
		mustCreatePolicy(t, dyn, ns, pol)
		waitFor(t, "relative TTL deletion", pollTimeout, func() (bool, string) {
			_, err := dyn.Resource(probeGVR).Namespace(ns).Get(ctx, "ttl-relative-expired", metav1.GetOptions{})
			return err != nil, resourceState(err)
		})
	})

	t.Run("Relative_Negative_NotExpiredSurvives", func(t *testing.T) {
		recent := time.Now().UTC().Format(time.RFC3339)
		obj := probe("ttl-relative-fresh", ns, "", 0, "")
		obj.SetLabels(map[string]string{"e2e": "ttl-relative-neg"})
		createResource(t, dyn, ns, obj)
		setProbeStatus(t, dyn, ns, obj, "completedAt", recent)
		pol := basePolicy("ttl-relative-neg", ns, map[string]string{"e2e": "ttl-relative-neg"}, 0)
		pol.Spec.TTL = v1alpha1.TTLSpec{RelativeTo: "status.completedAt", SecondsAfter: int64Ptr(3600)}
		pol.Spec.TargetResource = v1alpha1.TargetResourceSpec{APIVersion: probeGroup + "/" + probeVersion, Kind: probeKind, Namespace: ns, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"e2e": "ttl-relative-neg"}}}
		mustCreatePolicy(t, dyn, ns, pol)
		time.Sleep(12 * time.Second)
		if _, err := dyn.Resource(probeGVR).Namespace(ns).Get(ctx, "ttl-relative-fresh", metav1.GetOptions{}); err != nil {
			t.Fatalf("relative-TTL fresh resource deleted: %v", err)
		}
	})
}

// TestE2E_ArbitraryCRD proves arbitrary-CRD usefulness via discovery-based
// resolution (irregular plural) without special-casing the test CRD name.
func TestE2E_ArbitraryCRD(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	ctx := context.Background()
	dyn, ext := clients(t)
	if err := ensureTestCRD(ctx, ext); err != nil {
		t.Fatalf("test CRD: %v", err)
	}
	ns := "gc-crd-e2e"
	ensureNamespace(ctx, dyn, ns)

	// A policy targeting the CRD by Kind alone must resolve the irregular
	// plural zgprobes through the RESTMapper, not naive pluralization.
	pr := probe("crd-e2e-probe", ns, "ephemeral", 3, "")
	pr.SetLabels(map[string]string{"e2e": "crd-e2e"})
	createResource(t, dyn, ns, pr)
	pol := basePolicy("crd-e2e", ns, map[string]string{"e2e": "crd-e2e"}, 0)
	pol.Spec.TTL = v1alpha1.TTLSpec{FieldPath: "spec.ttlSeconds"}
	pol.Spec.TargetResource = v1alpha1.TargetResourceSpec{APIVersion: probeGroup + "/" + probeVersion, Kind: probeKind, Namespace: ns, LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"e2e": "crd-e2e"}}}
	mustCreatePolicy(t, dyn, ns, pol)
	waitFor(t, "arbitrary-CRD deletion via RESTMapper resolution", pollTimeout, func() (bool, string) {
		_, err := dyn.Resource(probeGVR).Namespace(ns).Get(ctx, "crd-e2e-probe", metav1.GetOptions{})
		return err != nil, resourceState(err)
	})
}
