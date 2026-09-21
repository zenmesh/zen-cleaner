package controller

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	zenv1alpha1 "github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = zenv1alpha1.AddToScheme(s)
	return s
}

func cfgObj(ns, name, uid string) *unstructured.Unstructured {
	o := &unstructured.Unstructured{}
	o.SetGroupVersionKind(zenv1alpha1.SchemeGroupVersion.WithKind("ZenCleanerPolicy"))
	o.SetNamespace(ns)
	o.SetName(name)
	if uid != "" {
		o.SetUID(types.UID(uid))
	}
	return o
}

// Law 1: plan digest is order-independent (S018/§4 deterministic law).
func TestPlanDigest_OrderIndependent(t *testing.T) {
	a := cfgObj("ns1", "alpha", "uid-a")
	b := cfgObj("ns2", "beta", "uid-b")
	d1 := PlanDigest([]unstructured.Unstructured{*a, *b})
	d2 := PlanDigest([]unstructured.Unstructured{*b, *a})
	if d1 != d2 {
		t.Fatalf("digest must be order-independent: %s vs %s", d1, d2)
	}
	c := cfgObj("ns1", "gamma", "uid-c")
	if PlanDigest([]unstructured.Unstructured{*a, *c}) == d1 {
		t.Error("different object set must produce different digest")
	}
}

// Law 2: precondition/UID protection — a replaced object is skipped, not deleted.
func TestDeleteWithReceipt_UIDPrecondition(t *testing.T) {
	s := testScheme()
	original := cfgObj("ns1", "target", "uid-original")
	replacement := cfgObj("ns1", "target", "uid-replacement")
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(replacement).Build()
	rc := DeleteWithReceipt(context.Background(), c, []unstructured.Unstructured{*original})
	if len(rc.Results) != 1 || rc.Results[0].Class != ResultSkippedPreconditionMismatch {
		t.Fatalf("expected precondition mismatch skip, got %+v", rc.Results)
	}
	if rc.OverallStatus != "PARTIAL" {
		t.Errorf("overall status = %s, want PARTIAL", rc.OverallStatus)
	}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "ns1", Name: "target"}, replacement); err != nil {
		t.Fatalf("replacement must survive: %v", err)
	}
}

// Law 3: already-absent is idempotent (ALREADY_ABSENT), not an error.
func TestDeleteWithReceipt_AlreadyAbsentIdempotent(t *testing.T) {
	s := testScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	ghost := cfgObj("ns1", "gone", "uid-gone")
	rc := DeleteWithReceipt(context.Background(), c, []unstructured.Unstructured{*ghost})
	if rc.Results[0].Class != ResultAlreadyAbsent {
		t.Fatalf("expected ALREADY_ABSENT, got %+v", rc.Results[0])
	}
	if rc.OverallStatus != "SUCCEEDED" {
		t.Errorf("ALREADY_ABSENT must not degrade overall status: %s", rc.OverallStatus)
	}
}

// Law 4: partial failure honesty — mixed results keep per-object classes.
func TestDeleteWithReceipt_PartialFailure(t *testing.T) {
	s := testScheme()
	live := cfgObj("ns1", "deletable", "uid-live")
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(live).Build()
	missing := cfgObj("ns1", "missing", "uid-missing")
	planned := []unstructured.Unstructured{*live, *missing}
	// order: ensure deterministic digest computation still works
	rc := DeleteWithReceipt(context.Background(), c, planned)
	classes := map[string]ObjectResultClass{}
	for _, r := range rc.Results {
		classes[r.Name] = r.Class
	}
	if classes["deletable"] != ResultSucceeded {
		t.Errorf("deletable = %v", classes["deletable"])
	}
	if classes["missing"] != ResultAlreadyAbsent {
		t.Errorf("missing = %v", classes["missing"])
	}
	if rc.OverallStatus != "SUCCEEDED" {
		t.Errorf("overall = %s", rc.OverallStatus)
	}
}

// Law 5: receipt is machine-readable and carries no secret-shaped fields.
func TestReceiptJSONShape(t *testing.T) {
	s := testScheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()
	rc := DeleteWithReceipt(context.Background(), c, []unstructured.Unstructured{*cfgObj("ns", "o", "u")})
	b, err := json.Marshal(rc)
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, forbidden := range []string{"secret", "password", "token"} {
		if containsFold(js, forbidden) {
			t.Errorf("receipt contains forbidden key material: %s", forbidden)
		}
	}
	if rc.Version != ReceiptVersion {
		t.Errorf("receipt version = %s", rc.Version)
	}
}

func containsFold(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if eqFold(s[i:i+len(sub)], sub) {
				return true
			}
		}
		return false
	})()
}

func eqFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 32
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func sortDeterministic(objs []unstructured.Unstructured) {
	sort.Slice(objs, func(i, j int) bool { return objs[i].GetName() < objs[j].GetName() })
}
