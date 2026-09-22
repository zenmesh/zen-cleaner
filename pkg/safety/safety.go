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

// Package safety is the single choke point for every destructive decision
// zen-cleaner makes (SUPPORT2-033 §2). Every path between "object matched a
// policy" and a DELETE call must pass Evaluate; any ambiguity denies.
//
// Refusal reasons are a bounded, closed vocabulary — they are metric label
// values and status surfaces, never free-form object identity.
package safety

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	gcapi "github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

// Refusal reason classes (bounded; metric label values).
const (
	// ReasonProtectedNamespace: the object lives in a protected namespace.
	ReasonProtectedNamespace = "protected_namespace"
	// ReasonProtectedKind: the object kind is hard-protected (data plane,
	// cluster structure, or admission plumbing).
	ReasonProtectedKind = "protected_kind"
	// ReasonExcludedLabel: the object carries the exclusion label/annotation.
	ReasonExcludedLabel = "excluded_label"
	// ReasonWorkloadManaged: workload kind without the explicit behavior opt-in.
	ReasonWorkloadManaged = "workload_managed"
	// ReasonActiveWorkload: workload explicitly allowed by policy but not in a
	// terminal phase.
	ReasonActiveWorkload = "active_workload"
	// ReasonStaleUID: the live object's UID differs from the observation the
	// decision was made on (stale-cache protection).
	ReasonStaleUID = "stale_uid"
	// ReasonDeletesDisabled: the emergency stop is engaged.
	ReasonDeletesDisabled = "deletes_disabled"
	// ReasonPolicyInvalid: the driving policy failed runtime validation.
	ReasonPolicyInvalid = "policy_invalid"
	// ReasonUnknown: fail-safe bucket for any unrecognized situation.
	ReasonUnknown = "unknown"
)

// ExclusionLabelKey is the label (and annotation) key that marks an object as
// permanently invisible to zen-cleaner, regardless of policy.
const ExclusionLabelKey = "zen-cleaner.zen-mesh.io/exclude"

// protectedNamespaces are always ineligible for cleanup. The values are
// infrastructure law, not configuration.
var protectedNamespaces = map[string]bool{
	"kube-system":     true,
	"kube-public":     true,
	"kube-node-lease": true,
	"kube-flannel":    true, // k3s CNI (disposable clusters run it)
	"kube-registry":   true, // in-cluster registry (k3d)
	"metallb-system":  true, // CNI/service-lb add-ons commonly present
}

// hardProtectedKinds can never be cleanup targets: they are cluster
// structure, admission plumbing, or data-bearing volumes.
var hardProtectedKinds = map[string]bool{
	"Namespace":                      true,
	"PersistentVolume":               true,
	"PersistentVolumeClaim":          true,
	"StorageClass":                   true,
	"CustomResourceDefinition":       true,
	"ValidatingWebhookConfiguration": true,
	"MutatingWebhookConfiguration":   true,
}

// workloadKinds require the explicit behavior.AllowWorkloadDeletion opt-in,
// and (for Pods) a terminal phase.
var workloadKinds = map[string]bool{
	"Pod":     true,
	"Job":     true,
	"CronJob": true,
}

// Config carries the deployment-level safety configuration.
type Config struct {
	// ProtectedNamespaces are denied in addition to the built-in set.
	ProtectedNamespaces []string
	// OwnNamespace is the namespace the controller runs in (POD_NAMESPACE).
	OwnNamespace string
	// DeletesDisabled engages the emergency stop: nothing is deleted,
	// candidates are logged and counted instead.
	DeletesDisabled bool
}

// Gate evaluates safety for concrete objects.
type Gate struct {
	cfg       Config
	protected map[string]bool
}

// NewGate builds a Gate from deployment configuration.
func NewGate(cfg Config) *Gate {
	protected := map[string]bool{}
	for ns := range protectedNamespaces {
		protected[ns] = true
	}
	if cfg.OwnNamespace != "" {
		protected[cfg.OwnNamespace] = true
	}
	for _, ns := range cfg.ProtectedNamespaces {
		if ns = strings.TrimSpace(ns); ns != "" {
			protected[ns] = true
		}
	}
	return &Gate{cfg: cfg, protected: protected}
}

// IsProtectedNamespace reports whether a namespace is protected.
func (g *Gate) IsProtectedNamespace(ns string) bool {
	return g.protected[ns]
}

// ProtectedNamespaces returns the effective protected set (sorted by caller
// for display purposes; map form here).
func (g *Gate) EffectiveProtectedNamespaces() []string {
	out := make([]string, 0, len(g.protected))
	for ns := range g.protected {
		out = append(out, ns)
	}
	return out
}

// EvaluateObject is the last-line gate immediately before a DELETE call on a
// concrete object. It returns the refusal reason when deletion is denied and
// "" when allowed. Ambiguity denies with ReasonUnknown.
func (g *Gate) EvaluateObject(obj *unstructured.Unstructured, behavior gcapi.BehaviorSpec) (allowed bool, reason string) {
	if obj == nil || obj.GetName() == "" || obj.GetKind() == "" {
		return false, ReasonUnknown
	}

	if g.cfg.DeletesDisabled {
		return false, ReasonDeletesDisabled
	}

	if obj.GetNamespace() != "" && g.protected[obj.GetNamespace()] {
		return false, ReasonProtectedNamespace
	}

	if hardProtectedKinds[obj.GetKind()] {
		return false, ReasonProtectedKind
	}

	if excludedByLabelOrAnnotation(obj) {
		return false, ReasonExcludedLabel
	}

	if workloadKinds[obj.GetKind()] && !behavior.AllowWorkloadDeletion {
		return false, ReasonWorkloadManaged
	}

	if obj.GetKind() == "Pod" && behavior.AllowWorkloadDeletion {
		phase, found, _ := unstructured.NestedString(obj.Object, "status", "phase")
		if !found {
			return false, ReasonActiveWorkload // cannot prove terminal -> deny
		}
		switch phase {
		case "Succeeded", "Failed":
			// terminal: deletable
		default:
			return false, ReasonActiveWorkload
		}
	}

	return true, ""
}

// EvaluatePolicyTarget is the policy-admission-time check for the target
// scope itself. It returns the refusal reason for the TARGET (independent of
// any concrete object), or "" when the target is acceptable.
func (g *Gate) EvaluatePolicyTarget(target gcapi.TargetResourceSpec) (allowed bool, reason string) {
	if hardProtectedKinds[target.Kind] {
		return false, ReasonProtectedKind
	}
	if target.Namespace != "" && target.Namespace != "*" && g.protected[target.Namespace] {
		return false, ReasonProtectedNamespace
	}
	return true, ""
}

// excludedByLabelOrAnnotation checks the exclusion key as both label and
// annotation; only the explicit value "true" opts out.
func excludedByLabelOrAnnotation(obj *unstructured.Unstructured) bool {
	for _, v := range obj.GetLabels() {
		if isExclusionEntry(v) {
			return true
		}
	}
	for _, v := range obj.GetAnnotations() {
		if isExclusionEntry(v) {
			return true
		}
	}
	return false
}

func isExclusionEntry(v string) bool {
	return v == "true"
}

// HasExclusion reports whether the exclusion key is present with value "true"
// (helper for tests and tooling).
func HasExclusion(obj *unstructured.Unstructured) bool {
	return excludedByLabelOrAnnotation(obj)
}
