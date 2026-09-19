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

// SUPPORT2-003 §4-§5: bounded target-capability validation.
//
// Before an informer is created for a policy target, the controller
// classifies whether the target API/resource exists and whether the
// configured identity may list it. Unsupported or unauthorized targets are
// SAFE failures: no reflector is started, the policy carries a classified
// condition, and evaluation retries with bounded backoff. Recovery is
// automatic: when the target becomes available/permitted, the next
// capability check succeeds and the informer starts normally.
//
// RBAC is never broadened by this path: validation only OBSERVES
// discoverability and permissions (a one-object List probe), it never
// requests grants.
package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	cleanererrors "github.com/zenmesh/zen-cleaner/internal/errors"
	"github.com/zenmesh/zen-cleaner/pkg/api/v1alpha1"
)

// TargetErrorType is the stable failure-classification vocabulary (§5).
// Raw reflector errors are never the operator-facing state.
type TargetErrorType string

const (
	// TargetAPINotFound: the apiVersion's group/version is not served.
	TargetAPINotFound TargetErrorType = "TARGET_API_NOT_FOUND"
	// TargetResourceNotDiscoverable: group/version is served but the
	// pluralized resource does not exist in discovery.
	TargetResourceNotDiscoverable TargetErrorType = "TARGET_RESOURCE_NOT_DISCOVERABLE"
	// TargetAccessDenied: list/watch of the target is forbidden for the
	// configured identity (RBAC default-deny preserved).
	TargetAccessDenied TargetErrorType = "TARGET_ACCESS_DENIED"
	// TargetConfigurationInvalid: the policy's apiVersion/kind is malformed.
	TargetConfigurationInvalid TargetErrorType = "TARGET_CONFIGURATION_INVALID"
	// TargetTemporarilyUnavailable: API server/network error — retryable.
	TargetTemporarilyUnavailable TargetErrorType = "TARGET_TEMPORARILY_UNAVAILABLE"
)

// TargetCapabilityError is a classified target-validation failure.
type TargetCapabilityError struct {
	Type   TargetErrorType `json:"type"`
	GVK    string          `json:"gvk"`
	Detail string          `json:"detail"`
}

func (e *TargetCapabilityError) Error() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Detail)
}

// ConditionReason maps the classified type to the policy condition reason.
func (e *TargetCapabilityError) ConditionReason() string {
	return string(e.Type)
}

// Retryable reports whether the failure may resolve without operator action.
func (e *TargetCapabilityError) Retryable() bool {
	return e.Type == TargetTemporarilyUnavailable
}

// targetProbeCache caches successful capability checks per GVK+namespace so
// steady-state reconciliation does not hit discovery on every pass. Entries
// expire after the TTL so recovery from a previously failed check happens
// without operator action.
const targetProbeCacheTTL = 5 * time.Minute

type targetProbeEntry struct {
	granted   bool
	class     TargetErrorType
	detail    string
	checkedAt time.Time
}

// targetFailureBackoff is the bounded exponential backoff for repeated
// capability failures (§7): base 30s doubling, capped at 10m. It uses the
// controller-runtime requeue path; no custom scheduler.
const (
	targetBackoffBase = 30 * time.Second
	targetBackoffCap  = 10 * time.Minute
)

// TargetBackoffFor returns the bounded requeue delay for the Nth consecutive
// failure (1-based). Attempt 1 => 30s, 2 => 60s, ... capped at 10m.
func TargetBackoffFor(consecutiveFailures int) time.Duration {
	if consecutiveFailures < 1 {
		consecutiveFailures = 1
	}
	d := targetBackoffBase
	for i := 1; i < consecutiveFailures; i++ {
		d *= 2
		if d >= targetBackoffCap {
			return targetBackoffCap
		}
	}
	if d > targetBackoffCap {
		return targetBackoffCap
	}
	return d
}

// ValidateTargetCapability classifies the policy target:
//
//	configuration validity -> API served? -> resource discoverable? -> list permitted?
//
// Returns nil when the target exists and is listable in the policy's
// namespace scope. The returned error is always a *TargetCapabilityError.
func (r *PolicyReconciler) ValidateTargetCapability(ctx context.Context, policy *v1alpha1.ZenCleanerPolicy) error {
	gvk := schema.GroupVersionKind{
		Group:   groupOf(policy.Spec.TargetResource.APIVersion),
		Version: versionOf(policy.Spec.TargetResource.APIVersion),
		Kind:    policy.Spec.TargetResource.Kind,
	}
	gvkStr := gvk.Group + "/" + gvk.Kind

	// TARGET_CONFIGURATION_INVALID: malformed apiVersion.
	if strings.TrimSpace(policy.Spec.TargetResource.APIVersion) == "" ||
		strings.TrimSpace(policy.Spec.TargetResource.Kind) == "" ||
		gvk.Version == "" {
		return &TargetCapabilityError{Type: TargetConfigurationInvalid, GVK: gvkStr,
			Detail: fmt.Sprintf("apiVersion %q / kind %q is malformed", policy.Spec.TargetResource.APIVersion, policy.Spec.TargetResource.Kind)}
	}

	// Steady-state cache: a recently granted probe short-circuits discovery
	// and the list probe (bounded; expires so recovery is still automatic).
	r.targetProbeMu.RLock()
	if entry, ok := r.targetProbes[probeKey(gvkStr, policy.Spec.TargetResource.Namespace)]; ok {
		r.targetProbeMu.RUnlock()
		if time.Since(entry.checkedAt) < targetProbeCacheTTL {
			if entry.granted {
				return nil
			}
			return &TargetCapabilityError{Type: entry.class, GVK: gvkStr, Detail: entry.detail}
		}
	} else {
		r.targetProbeMu.RUnlock()
	}

	// 1. Is the group/version served? (TARGET_API_NOT_FOUND)
	// Without a discovery client, skip straight to the list probe — the
	// list itself classifies API-not-found vs access-denied vs unavailable.
	plural := pluralForKind(gvk.Kind)
	if r.discoveryClient != nil {
		gvList, err := r.discoveryClient.ServerResourcesForGroupVersion(
			schema.GroupVersion{Group: gvk.Group, Version: gvk.Version}.String())
		if err != nil {
			class, detail := classifyDiscoveryError(err, gvk)
			return r.recordProbe(gvkStr, policy.Spec.TargetResource.Namespace, class, detail)
		}

		// 2. Is the resource discoverable within the group/version?
		found := false
		for _, ar := range gvList.APIResources {
			if strings.EqualFold(ar.Kind, gvk.Kind) && !strings.Contains(ar.Name, "/") {
				plural = ar.Name
				found = true
				break
			}
		}
		if !found {
			return r.recordProbe(gvkStr, policy.Spec.TargetResource.Namespace,
				TargetResourceNotDiscoverable,
				fmt.Sprintf("kind %q is not served by group/version %q", gvk.Kind, gvList.GroupVersion))
		}
	}

	gvr := schema.GroupVersionResource{Group: gvk.Group, Version: gvk.Version, Resource: plural}

	// 3. Is list permitted (RBAC probe, one object)? (TARGET_ACCESS_DENIED)
	ns := normalizeNamespace(policy.Spec.TargetResource.Namespace)
	listCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ri := r.dynamicClient.Resource(gvr)
	var listErr error
	if ns == "" || ns == "*" {
		_, listErr = ri.Namespace(metav1.NamespaceAll).List(listCtx, metav1.ListOptions{Limit: 1})
	} else {
		_, listErr = ri.Namespace(ns).List(listCtx, metav1.ListOptions{Limit: 1})
	}
	if listErr != nil {
		class, detail := classifyListError(listErr, gvr, ns)
		return r.recordProbe(gvkStr, ns, class, detail)
	}

	return r.recordProbeGranted(gvkStr, ns)
}

func (r *PolicyReconciler) recordProbe(gvk, ns string, class TargetErrorType, detail string) error {
	// Failures are NOT cached — only grants are. A denied/missing target must
	// be re-probed on every reconcile so recovery is automatic as soon as
	// the API/RBAC surface changes (frequency is bounded by the reconcile
	// backoff, §7).
	return &TargetCapabilityError{Type: class, GVK: gvk, Detail: detail}
}

func (r *PolicyReconciler) recordProbeGranted(gvk, ns string) error {
	r.targetProbeMu.Lock()
	if r.targetProbes == nil {
		r.targetProbes = map[string]targetProbeEntry{}
	}
	r.targetProbes[probeKey(gvk, ns)] = targetProbeEntry{granted: true, checkedAt: time.Now()}
	r.targetProbeMu.Unlock()
	return nil
}

func probeKey(gvk, ns string) string { return gvk + "|" + ns }

func classifyDiscoveryError(err error, gvk schema.GroupVersionKind) (TargetErrorType, string) {
	if apierrors.IsNotFound(err) {
		return TargetAPINotFound, fmt.Sprintf("API group/version %q is not served by this cluster", gvk.Group+"/"+gvk.Version)
	}
	return TargetTemporarilyUnavailable, fmt.Sprintf("discovery failed: %v", err)
}

func classifyListError(err error, gvr schema.GroupVersionResource, ns string) (TargetErrorType, string) {
	switch {
	case apierrors.IsForbidden(err):
		return TargetAccessDenied, fmt.Sprintf("list of %s in namespace %q is forbidden for the configured identity", gvr.String(), ns)
	case apierrors.IsNotFound(err):
		return TargetResourceNotDiscoverable, fmt.Sprintf("resource %s not found", gvr.String())
	case apierrors.IsServiceUnavailable(err), apierrors.IsTimeout(err), apierrors.IsTooManyRequests(err):
		return TargetTemporarilyUnavailable, fmt.Sprintf("API temporarily unavailable: %v", err)
	default:
		if strings.Contains(strings.ToLower(err.Error()), "connection refused") ||
			strings.Contains(strings.ToLower(err.Error()), "timeout") {
			return TargetTemporarilyUnavailable, fmt.Sprintf("API temporarily unavailable: %v", err)
		}
		return TargetTemporarilyUnavailable, fmt.Sprintf("list probe failed: %v", err)
	}
}

// reconcileTargetCondition writes the classified failure/recovery onto the
// policy status conditions (operator-visible, bounded — §4/§5).
func (r *PolicyReconciler) reconcileTargetCondition(ctx context.Context, policy *v1alpha1.ZenCleanerPolicy, tcErr *TargetCapabilityError) {
	if r.statusUpdater == nil {
		return
	}
	_ = ctx
	if tcErr == nil {
		_ = r.statusUpdater.UpdateTargetCondition(ctx, policy,
			"TargetReady", "TargetAvailable",
			"target API/resource is served and listable", false)
		return
	}
	permanent := tcErr.Type == TargetConfigurationInvalid
	_ = r.statusUpdater.UpdateTargetCondition(ctx, policy,
		"TargetReady", tcErr.ConditionReason(),
		truncateConditionMessage(tcErr.Detail), permanent)
}

func truncateConditionMessage(msg string) string {
	const max = 512
	if len(msg) <= max {
		return msg
	}
	return msg[:max]
}

// validateTargetOrClassify is the capability-gate hook: validate, classify,
// and record the bounded failure state for backoff. Returns nil when the
// informer may be created.
func (r *PolicyReconciler) validateTargetOrClassify(ctx context.Context, policy *v1alpha1.ZenCleanerPolicy) error {
	tcErr, isCap := func() (*TargetCapabilityError, bool) {
		err := r.ValidateTargetCapability(ctx, policy)
		if err == nil {
			return nil, false
		}
		if ce, ok := err.(*TargetCapabilityError); ok {
			return ce, true
		}
		return &TargetCapabilityError{Type: TargetTemporarilyUnavailable, Detail: err.Error()}, true
	}()
	if !isCap {
		// Success: clear failure state, mark recovered.
		r.clearTargetFailure(policy.UID)
		r.reconcileTargetCondition(ctx, policy, nil)
		return nil
	}
	r.reconcileTargetCondition(ctx, policy, tcErr)
	count := r.recordTargetFailure(policy.UID)
	wrapped := cleanererrors.Wrapf(tcErr, string(tcErr.Type),
		"target capability validation failed (attempt %d, next retry in %s)", count, TargetBackoffFor(count))
	wrapped = wrapped.WithContext("policy_namespace", policy.Namespace)
	wrapped = wrapped.WithContext("policy_name", policy.Name)
	wrapped = wrapped.WithContext("target", tcErr.GVK)
	return wrapped
}

// recordTargetFailure increments and returns the consecutive-failure count.
func (r *PolicyReconciler) recordTargetFailure(uid types.UID) int {
	r.targetFailuresMu.Lock()
	defer r.targetFailuresMu.Unlock()
	if r.targetFailures == nil {
		r.targetFailures = map[types.UID]int{}
	}
	r.targetFailures[uid]++
	return r.targetFailures[uid]
}

// clearTargetFailure resets the consecutive-failure state.
func (r *PolicyReconciler) clearTargetFailure(uid types.UID) {
	r.targetFailuresMu.Lock()
	defer r.targetFailuresMu.Unlock()
	delete(r.targetFailures, uid)
}

// targetFailureCount reads the consecutive-failure count.
func (r *PolicyReconciler) targetFailureCount(uid types.UID) int {
	r.targetFailuresMu.RLock()
	defer r.targetFailuresMu.RUnlock()
	return r.targetFailures[uid]
}

// groupOf / versionOf split an apiVersion string.
func groupOf(apiVersion string) string {
	if i := strings.Index(apiVersion, "/"); i >= 0 {
		return apiVersion[:i]
	}
	return ""
}

func versionOf(apiVersion string) string {
	if i := strings.Index(apiVersion, "/"); i >= 0 {
		return apiVersion[i+1:]
	}
	return apiVersion
}

// pluralForKind is the shared naive-pluralization fallback used only when
// discovery does not list an explicit resource name for the kind.
func pluralForKind(kind string) string {
	return pluralizeKindLower(kind)
}

func pluralizeKindLower(kind string) string {
	if kind == "" {
		return kind
	}
	lower := strings.ToLower(kind)
	switch {
	case strings.HasSuffix(lower, "s"):
		return lower + "es"
	case strings.HasSuffix(lower, "y"):
		return lower[:len(lower)-1] + "ies"
	default:
		return lower + "s"
	}
}
