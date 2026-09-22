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

package validation

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

func TestValidateNamespace(t *testing.T) {
	tests := []struct {
		name        string
		namespace   string
		expectError bool
	}{
		{
			// SUPPORT2-033 §2: empty namespace historically meant
			// cluster-wide; it is now rejected as an unsafe implicit default.
			name:        "empty namespace rejected",
			namespace:   "",
			expectError: true,
		},
		{
			name:        "wildcard namespace",
			namespace:   "*",
			expectError: false,
		},
		{
			name:        "valid namespace",
			namespace:   "default",
			expectError: false,
		},
		{
			name:        "valid namespace with dash",
			namespace:   "kube-system",
			expectError: false,
		},
		{
			name:        "invalid namespace with uppercase",
			namespace:   "Default",
			expectError: true,
		},
		{
			name:        "invalid namespace with underscore",
			namespace:   "my_namespace",
			expectError: true,
		},
		{
			name:        "invalid namespace starting with number",
			namespace:   "123namespace",
			expectError: true,
		},
		{
			name:        "invalid namespace too long",
			namespace:   string(make([]byte, 64)), // DNS-1123 label max is 63
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNamespace(tt.namespace)
			if tt.expectError {
				if err == nil {
					t.Errorf("validateNamespace(%q) expected error but got none", tt.namespace)
				}
			} else {
				if err != nil {
					t.Errorf("validateNamespace(%q) returned error: %v", tt.namespace, err)
				}
			}
		})
	}
}

func TestValidateLabelSelector(t *testing.T) {
	tests := []struct {
		name        string
		selector    *metav1.LabelSelector
		expectError bool
	}{
		{
			name:        "nil selector",
			selector:    nil,
			expectError: false,
		},
		{
			name: "valid matchLabels",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":     "test",
					"version": "v1",
				},
			},
			expectError: false,
		},
		{
			name: "invalid label key",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"invalid-key_": "value",
				},
			},
			expectError: true,
		},
		{
			name: "invalid label value",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "invalid\nvalue",
				},
			},
			expectError: true,
		},
		{
			name: "valid matchExpressions",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "app",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"test", "prod"},
					},
				},
			},
			expectError: false,
		},
		{
			name: "matchExpressions missing operator",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:    "app",
						Values: []string{"test"},
					},
				},
			},
			expectError: true,
		},
		{
			name: "matchExpressions In operator without values",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "app",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{},
					},
				},
			},
			expectError: true,
		},
		{
			name: "matchExpressions invalid value",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "app",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"valid", "invalid\nvalue"},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLabelSelector(tt.selector)
			if tt.expectError {
				if err == nil {
					t.Errorf("validateLabelSelector() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("validateLabelSelector() returned error: %v", err)
				}
			}
		})
	}
}

func TestValidateTargetResource_Namespace(t *testing.T) {
	tests := []struct {
		name        string
		target      *v1alpha1.TargetResourceSpec
		expectError bool
	}{
		{
			name: "valid namespace",
			target: &v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  "default",
			},
			expectError: false,
		},
		{
			name: "wildcard namespace with explicit selector",
			target: &v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  "*",
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "disposable"},
				},
			},
			expectError: false,
		},
		{
			// SUPPORT2-033 §2/§3: cluster-wide without a selector is unsafe.
			name: "wildcard namespace without selector rejected",
			target: &v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  "*",
			},
			expectError: true,
		},
		{
			// SUPPORT2-033 §2: empty namespace is no longer implicit
			// cluster-wide; it must be explicit.
			name: "empty namespace rejected",
			target: &v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  "",
			},
			expectError: true,
		},
		{
			name: "protected namespace rejected",
			target: &v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  "kube-system",
			},
			expectError: true,
		},
		{
			name: "protected kind rejected",
			target: &v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "PersistentVolumeClaim",
				Namespace:  "default",
			},
			expectError: true,
		},
		{
			// SUPPORT2-033 §2: empty namespace must be explicit now.
			name: "empty namespace rejected",
			target: &v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  "",
			},
			expectError: true,
		},
		{
			name: "invalid namespace",
			target: &v1alpha1.TargetResourceSpec{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Namespace:  "Invalid_Namespace",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTargetResource(tt.target)
			if tt.expectError {
				if err == nil {
					t.Errorf("validateTargetResource() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("validateTargetResource() returned error: %v", err)
				}
			}
		})
	}
}

func TestValidateTargetResource_APIVersion(t *testing.T) {
	tests := []struct {
		name        string
		target      *v1alpha1.TargetResourceSpec
		expectError bool
	}{
		{
			name: "valid apiVersion with leading space",
			target: &v1alpha1.TargetResourceSpec{
				APIVersion: " v1",
				Kind:       "ConfigMap",
			},
			expectError: true,
		},
		{
			name: "valid apiVersion with trailing space",
			target: &v1alpha1.TargetResourceSpec{
				APIVersion: "v1 ",
				Kind:       "ConfigMap",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTargetResource(tt.target)
			if tt.expectError {
				if err == nil {
					t.Errorf("validateTargetResource() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("validateTargetResource() returned error: %v", err)
				}
			}
		})
	}
}
