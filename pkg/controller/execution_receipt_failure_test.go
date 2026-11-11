package controller

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// A receipt must distinguish an unavailable API, a refused deletion, and
// another writer removing an object between observation and deletion.
func TestDeleteWithReceipt_APIOutcomeHonesty(t *testing.T) {
	for _, tc := range []struct {
		name                string
		getErr, errorDelete error
		want                ObjectResultClass
		overall             string
	}{
		{name: "get unavailable", getErr: errors.New("API unavailable"), want: ResultFailed, overall: "FAILED"},
		{name: "delete forbidden", errorDelete: apierrors.NewForbidden(schema.GroupResource{Resource: "configmaps"}, "target", errors.New("denied")), want: ResultFailed, overall: "FAILED"},
		{name: "concurrent deletion", errorDelete: apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, "target"), want: ResultAlreadyAbsent, overall: "SUCCEEDED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := cfgObj("ns1", "target", "original")
			calls := 0
			c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(obj).WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					if tc.getErr != nil {
						return tc.getErr
					}
					return c.Get(ctx, key, obj, opts...)
				},
				Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
					calls++
					return tc.errorDelete
				},
			}).Build()
			got := DeleteWithReceipt(context.Background(), c, []unstructured.Unstructured{*obj})
			if len(got.Results) != 1 || got.Results[0].Class != tc.want || got.OverallStatus != tc.overall {
				t.Fatalf("dishonest receipt: %+v", got)
			}
			if tc.getErr != nil && calls != 0 {
				t.Fatal("attempted deletion after failed observation")
			}
			if tc.getErr == nil && calls != 1 {
				t.Fatalf("delete calls=%d", calls)
			}
			if got.PlanDigest == "" || got.CompletedAt.Before(got.StartedAt) {
				t.Fatalf("missing or invalid receipt binding: %+v", got)
			}
		})
	}
}

func TestReceiptOverall_FailureDominatesStalePrecondition(t *testing.T) {
	for _, rs := range [][]ObjectResult{
		{{Class: ResultFailed}, {Class: ResultSkippedPreconditionMismatch}},
		{{Class: ResultSkippedPreconditionMismatch}, {Class: ResultFailed}},
	} {
		if got := computeOverall(rs); got != "FAILED" {
			t.Fatalf("got %s", got)
		}
	}
}

// ── SUPPORT2-033 §4: DeleteWithReceipt precondition laws ──────────────────
//
// The controller-runtime fake client enforces resourceVersion delete
// preconditions natively but NOT UID preconditions (verified in
// controller-runtime@v0.25.2 pkg/client/fake/client.go Delete). The
// interceptor below therefore MODELS the API-server UID-precondition check
// explicitly. Neither fake proves real apiserver behavior; this repo has no
// envtest/kind harness — these tests deterministically reproduce the
// documented API contract instead.

// preconditionModelingClient returns a fake client whose Delete models the
// real API server: it runs raceHook exactly once at the DELETE boundary
// (after DeleteWithReceipt's verification GET), then validates the UID
// precondition against the current object before delegating. It records the
// delete options that DeleteWithReceipt actually sent. The modeled UID flips
// to "uid-replacement" when the hook fires.
func preconditionModelingClient(t *testing.T, obj *unstructured.Unstructured, raceHook func()) (client.Client, *[]client.DeleteOption, *int) {
	t.Helper()
	replaced := false
	hook := raceHook
	var sentOpts []client.DeleteOption
	deleteCalls := 0
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(obj).WithInterceptorFuncs(interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, o client.Object, opts ...client.DeleteOption) error {
			deleteCalls++
			sentOpts = append(sentOpts, opts...)
			if hook != nil {
				h := hook
				hook = nil
				h()
				replaced = true
			}
			do := client.DeleteOptions{}
			do.ApplyOptions(opts)
			if do.Preconditions != nil && do.Preconditions.UID != nil {
				// The race landed: the object the API server sees NOW.
				currentUID := "uid-original"
				if replaced {
					currentUID = "uid-replacement"
				}
				if currentUID != string(*do.Preconditions.UID) {
					return apierrors.NewConflict(
						schema.GroupResource{Resource: "configmaps"}, o.GetName(),
						errors.New("the UID in the precondition does not match the current UID. The object might have been deleted and then recreated"),
					)
				}
			}
			return c.Delete(ctx, o, opts...)
		},
	}).Build()
	return c, &sentOpts, &deleteCalls
}

// A replacement landing between the verification GET and the DELETE is
// refused by the UID precondition: SKIPPED (never FAILED, never a success),
// receipt truthful, nothing deleted.
func TestDeleteWithReceipt_ReplacementBetweenGetAndDelete_Refused(t *testing.T) {
	original := cfgObj("ns1", "target", "uid-original")
	c, sentOpts, deleteCalls := preconditionModelingClient(t, original, func() {
		// The hook runs at the DELETE boundary; the modeled object is now a
		// different one (uid-replacement) — the model client flips its UID.
	})

	planned := cfgObj("ns1", "target", "uid-original")
	rc := DeleteWithReceipt(context.Background(), c, []unstructured.Unstructured{*planned})

	if len(rc.Results) != 1 || rc.Results[0].Class != ResultSkippedPreconditionMismatch {
		t.Fatalf("replacement race must classify SKIPPED_PRECONDITION_MISMATCH, got %+v", rc.Results)
	}
	if rc.OverallStatus != "PARTIAL" {
		t.Fatalf("overall = %s, want PARTIAL", rc.OverallStatus)
	}
	if *deleteCalls != 1 {
		t.Fatalf("expected exactly one delete attempt, got %d", *deleteCalls)
	}
	// The DELETE carried the decision-time UID precondition.
	if len(*sentOpts) == 0 {
		t.Fatal("delete must carry precondition options")
	}
	do := client.DeleteOptions{}
	do.ApplyOptions(*sentOpts)
	if do.Preconditions == nil || do.Preconditions.UID == nil || string(*do.Preconditions.UID) != "uid-original" {
		t.Fatalf("delete must carry the planned UID precondition, got %+v", do.Preconditions)
	}
	// The object survives (the modeled replacement was not destroyed by name).
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "ns1", Name: "target"}, cfgObj("ns1", "target", "")); err != nil {
		t.Fatalf("object must survive: %v", err)
	}
}

// A same-UID update landing between the verification GET and the DELETE is
// refused by the resourceVersion precondition (atomic refusal; the changed
// object survives).
func TestDeleteWithReceipt_SameUIDUpdateBetweenGetAndDelete_Refused(t *testing.T) {
	obj := cfgObj("ns1", "target", "uid-original")
	updated := false
	var sentOpts []client.DeleteOption
	deleteCalls := 0
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(obj).WithInterceptorFuncs(interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, o client.Object, opts ...client.DeleteOption) error {
			deleteCalls++
			sentOpts = append(sentOpts, opts...)
			if !updated {
				updated = true
				// Model the same-UID update: the UID precondition still
				// matches, but the RV precondition no longer does — the fake
				// client natively enforces the RV check against its store;
				// the modeled API server rejects before the native check.
				do := client.DeleteOptions{}
				do.ApplyOptions(opts)
				if do.Preconditions != nil && do.Preconditions.ResourceVersion != nil {
					return apierrors.NewConflict(
						schema.GroupResource{Resource: "configmaps"}, o.GetName(),
						errors.New("the ResourceVersion in the precondition does not match the ResourceVersion in record. The object might have been modified"),
					)
				}
			}
			return c.Delete(ctx, o, opts...)
		},
	}).Build()

	rc := DeleteWithReceipt(context.Background(), c, []unstructured.Unstructured{*cfgObj("ns1", "target", "uid-original")})
	if len(rc.Results) != 1 || rc.Results[0].Class != ResultSkippedPreconditionMismatch {
		t.Fatalf("same-UID update race must classify SKIPPED_PRECONDITION_MISMATCH, got %+v", rc.Results)
	}
	if rc.OverallStatus != "PARTIAL" {
		t.Fatalf("overall = %s, want PARTIAL", rc.OverallStatus)
	}
	_ = sentOpts
}

// A planned object with an EMPTY UID refuses before any read or delete:
// delete-by-name is never silently degraded to.
func TestDeleteWithReceipt_EmptyUIDRefused(t *testing.T) {
	obj := cfgObj("ns1", "target", "uid-original")
	c, _, deleteCalls := preconditionModelingClient(t, obj, nil)

	uidLess := cfgObj("ns1", "target", "")
	rc := DeleteWithReceipt(context.Background(), c, []unstructured.Unstructured{*uidLess})

	if len(rc.Results) != 1 || rc.Results[0].Class != ResultSkippedPreconditionMismatch {
		t.Fatalf("empty-UID plan must classify SKIPPED_PRECONDITION_MISMATCH, got %+v", rc.Results)
	}
	if rc.Results[0].Detail == "" || !containsFold(rc.Results[0].Detail, "uid") {
		t.Fatalf("refusal detail must name the UID law: %q", rc.Results[0].Detail)
	}
	if rc.OverallStatus != "PARTIAL" {
		t.Fatalf("overall = %s, want PARTIAL", rc.OverallStatus)
	}
	if *deleteCalls != 0 {
		t.Fatal("a DELETE was issued for a plan with no UID")
	}
}

// Matching UID + verification RV deletes successfully, and the DELETE
// carries BOTH preconditions.
func TestDeleteWithReceipt_MatchingUIDDeletesWithPreconditions(t *testing.T) {
	obj := cfgObj("ns1", "target", "uid-original")
	obj.SetResourceVersion("rv-7") // deterministic verification RV
	c, sentOpts, deleteCalls := preconditionModelingClient(t, obj, nil)

	rc := DeleteWithReceipt(context.Background(), c, []unstructured.Unstructured{*cfgObj("ns1", "target", "uid-original")})
	if len(rc.Results) != 1 || rc.Results[0].Class != ResultSucceeded {
		t.Fatalf("matching-UID delete must SUCCEED, got %+v", rc.Results)
	}
	if rc.OverallStatus != "SUCCEEDED" {
		t.Fatalf("overall = %s", rc.OverallStatus)
	}
	if *deleteCalls != 1 {
		t.Fatalf("delete calls = %d", *deleteCalls)
	}
	do := client.DeleteOptions{}
	do.ApplyOptions(*sentOpts)
	if do.Preconditions == nil || do.Preconditions.UID == nil || string(*do.Preconditions.UID) != "uid-original" {
		t.Fatalf("UID precondition missing: %+v", do.Preconditions)
	}
	if do.Preconditions.ResourceVersion == nil || *do.Preconditions.ResourceVersion != "rv-7" {
		t.Fatalf("resourceVersion precondition missing or wrong: %+v", do.Preconditions)
	}
	// The object is gone.
	err := c.Get(context.Background(), client.ObjectKey{Namespace: "ns1", Name: "target"}, cfgObj("ns1", "target", ""))
	if !apierrors.IsNotFound(err) {
		t.Fatalf("object should be deleted, got err: %v", err)
	}
}

// A verified object without a resourceVersion cannot be preconditioned:
// skip truthfully (never a success, never a failure) instead of sending an
// unenforceable precondition. NOTE: the controller-runtime fake AUTO-ASSIGNS
// resourceVersion "1" to objects added via WithObjects, so the broken
// backend shape is modeled with a Get interceptor that strips the RV from
// every read.
func TestDeleteWithReceipt_EmptyVerifiedResourceVersion_Refused(t *testing.T) {
	obj := cfgObj("ns1", "target", "uid-original")
	deleteCalls := 0
	c := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(obj).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, o client.Object, opts ...client.GetOption) error {
			if err := c.Get(ctx, key, o, opts...); err != nil {
				return err
			}
			if acc, aerr := meta.Accessor(o); aerr == nil {
				acc.SetResourceVersion("") // model the RV-less backend
			}
			return nil
		},
		Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.DeleteOption) error {
			deleteCalls++
			return nil
		},
	}).Build()

	rc := DeleteWithReceipt(context.Background(), c, []unstructured.Unstructured{*cfgObj("ns1", "target", "uid-original")})
	if len(rc.Results) != 1 || rc.Results[0].Class != ResultSkippedPreconditionMismatch {
		t.Fatalf("empty-RV verification must classify SKIPPED_PRECONDITION_MISMATCH, got %+v", rc.Results)
	}
	if !containsFold(rc.Results[0].Detail, "resourceVersion") {
		t.Fatalf("detail must name the resourceVersion law: %q", rc.Results[0].Detail)
	}
	if rc.OverallStatus != "PARTIAL" {
		t.Fatalf("overall = %s, want PARTIAL", rc.OverallStatus)
	}
	if deleteCalls != 0 {
		t.Fatal("a DELETE was issued without an enforceable resourceVersion precondition")
	}
}
