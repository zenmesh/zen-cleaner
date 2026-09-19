package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLabelSelectorsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    *metav1.LabelSelector
		b    *metav1.LabelSelector
		want bool
	}{
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "one nil",
			a:    nil,
			b:    &metav1.LabelSelector{},
			want: false,
		},
		{
			name: "both empty",
			a:    &metav1.LabelSelector{},
			b:    &metav1.LabelSelector{},
			want: true,
		},
		{
			name: "same match labels",
			a:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			b:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			want: true,
		},
		{
			name: "different match labels",
			a:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
			b:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": "other"}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := labelSelectorsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("labelSelectorsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchLabelsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    map[string]string
		b    map[string]string
		want bool
	}{
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "both empty",
			a:    map[string]string{},
			b:    map[string]string{},
			want: true,
		},
		{
			name: "same labels",
			a:    map[string]string{"app": "test", "env": "prod"},
			b:    map[string]string{"app": "test", "env": "prod"},
			want: true,
		},
		{
			name: "different values",
			a:    map[string]string{"app": "test"},
			b:    map[string]string{"app": "other"},
			want: false,
		},
		{
			name: "different counts",
			a:    map[string]string{"app": "test"},
			b:    map[string]string{"app": "test", "env": "prod"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchLabelsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("matchLabelsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchExpressionsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []metav1.LabelSelectorRequirement
		b    []metav1.LabelSelectorRequirement
		want bool
	}{
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "both empty",
			a:    []metav1.LabelSelectorRequirement{},
			b:    []metav1.LabelSelectorRequirement{},
			want: true,
		},
		{
			name: "same expressions",
			a: []metav1.LabelSelectorRequirement{
				{Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{"test"}},
			},
			b: []metav1.LabelSelectorRequirement{
				{Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{"test"}},
			},
			want: true,
		},
		{
			name: "different key",
			a: []metav1.LabelSelectorRequirement{
				{Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{"test"}},
			},
			b: []metav1.LabelSelectorRequirement{
				{Key: "env", Operator: metav1.LabelSelectorOpIn, Values: []string{"test"}},
			},
			want: false,
		},
		{
			name: "different count",
			a:    []metav1.LabelSelectorRequirement{{Key: "app"}},
			b:    []metav1.LabelSelectorRequirement{{Key: "app"}, {Key: "env"}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchExpressionsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("matchExpressionsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}
