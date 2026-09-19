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

package testing

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

// stdPolicyEvalFixture holds shared objects for PolicyEvaluationService tests that only differ in wiring.
type stdPolicyEvalFixture struct {
	Resource *unstructured.Unstructured
	Policy   *v1alpha1.ZenCleanerPolicy
	Lister   *MockResourceLister
	Sel      *MockSelectorMatcher
	Cond     *MockConditionMatcher
	RL       *MockRateLimiterProvider
	Deleter  *MockBatchDeleterCore
}

func newStdPolicyEvalFixture(t *testing.T) *stdPolicyEvalFixture {
	t.Helper()
	now := time.Now()
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			k8sKeyAPIVersion: k8sAPIV1,
			k8sKeyKind:       k8sKindConfigMap,
			k8sKeyMetadata: map[string]interface{}{
				k8sKeyName:       k8sNameTestCM,
				k8sKeyNamespace:  k8sNSDefault,
				k8sKeyUID:        k8sUIDTest,
				k8sKeyCreationTS: metav1.NewTime(now.Add(-2 * time.Hour)).Format(time.RFC3339),
			},
		},
	}

	policy := &v1alpha1.ZenCleanerPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sPolicyNameTest,
			Namespace: k8sNSDefault,
			UID:       types.UID("policy-uid"),
		},
		Spec: v1alpha1.ZenCleanerPolicySpec{
			TargetResource: v1alpha1.TargetResourceSpec{
				APIVersion: k8sAPIV1,
				Kind:       k8sKindConfigMap,
			},
			TTL: v1alpha1.TTLSpec{
				SecondsAfterCreation: func() *int64 { v := int64(3600); return &v }(),
			},
		},
	}

	mockLister := NewMockResourceLister()
	gvr := schema.GroupVersionResource{Group: "", Version: k8sAPIV1, Resource: k8sResConfigMaps}
	mockLister.SetResources(gvr, "default", []*unstructured.Unstructured{resource})

	mockSelectorMatcher := NewMockSelectorMatcher()
	mockSelectorMatcher.SetMatch(resource, true)

	mockConditionMatcher := NewMockConditionMatcher()
	mockConditionMatcher.SetMeetsConditions(resource, true)

	mockRateLimiter := NewMockRateLimiterProvider()
	mockDeleter := NewMockBatchDeleterCore()
	mockDeleter.SetDeleteResult(resource, nil)

	return &stdPolicyEvalFixture{
		Resource: resource,
		Policy:   policy,
		Lister:   mockLister,
		Sel:      mockSelectorMatcher,
		Cond:     mockConditionMatcher,
		RL:       mockRateLimiter,
		Deleter:  mockDeleter,
	}
}
