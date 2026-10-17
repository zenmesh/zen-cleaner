package controller

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
