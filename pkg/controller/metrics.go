package controller

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// served registers custom metrics with the registry actually exported by
// the manager's metrics endpoint. promauto's default (the global
// prometheus.DefaultRegisterer) is invisible to that endpoint, which left
// every documented zen_cleaner_* metric absent from :8080/metrics
// (SUPPORT2-032R).
var served = promauto.With(metrics.Registry)

const (
	labelPhase              = "phase"
	labelPolicyNamespace    = "policy_namespace"
	labelPolicyName         = "policy_name"
	labelResourceAPIVersion = "resource_api_version"
	labelResourceKind       = "resource_kind"
	labelReason             = "reason"
	labelErrorType          = "error_type"
)

var (
	// CleanerPoliciesTotal is a gauge that tracks the total number of GC policies by phase.
	gcPoliciesTotal = served.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zen_cleaner_policies_total",
			Help: "Total number of cleanup policies",
		},
		[]string{labelPhase},
	)

	// CleanerResourcesMatchedTotal is a counter that tracks the total number of resources matched by GC policies.
	gcResourcesMatchedTotal = served.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zen_cleaner_resources_matched_total",
			Help: "Total number of resources matched by cleanup policies",
		},
		[]string{labelPolicyNamespace, labelPolicyName, labelResourceAPIVersion, labelResourceKind},
	)

	// CleanerResourcesDeletedTotal is a counter that tracks the total number of resources deleted by GC.
	gcResourcesDeletedTotal = served.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zen_cleaner_resources_deleted_total",
			Help: "Total number of resources deleted by cleanup",
		},
		[]string{labelPolicyNamespace, labelPolicyName, labelResourceAPIVersion, labelResourceKind, labelReason},
	)

	// CleanerDeletionDurationSeconds is a histogram that tracks the time taken to delete resources.
	gcDeletionDurationSeconds = served.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "zen_cleaner_deletion_duration_seconds",
			Help:    "Time taken to delete resources",
			Buckets: prometheus.DefBuckets,
		},
		[]string{labelPolicyNamespace, labelPolicyName, labelResourceAPIVersion, labelResourceKind},
	)

	// CleanerErrorsTotal is a counter that tracks the total number of GC errors.
	cleanerErrorsTotal = served.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zen_cleaner_errors_total",
			Help: "Total number of cleanup errors",
		},
		[]string{labelPolicyNamespace, labelPolicyName, labelErrorType},
	)

	// CleanerEvaluationDurationSeconds is a histogram that tracks the time taken to evaluate policies.
	gcEvaluationDurationSeconds = served.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "zen_cleaner_evaluation_duration_seconds",
			Help:    "Time taken to evaluate cleanup policies",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0},
		},
		[]string{labelPolicyNamespace, labelPolicyName},
	)

	// CleanerInformersTotal is a gauge that tracks the total number of active resource informers.
	gcInformersTotal = served.NewGauge(
		prometheus.GaugeOpts{
			Name: "zen_cleaner_informers_total",
			Help: "Total number of active resource informers",
		},
	)

	// CleanerRateLimitersTotal is a gauge that tracks the total number of active rate limiters.
	gcRateLimitersTotal = served.NewGauge(
		prometheus.GaugeOpts{
			Name: "zen_cleaner_rate_limiters_total",
			Help: "Total number of active rate limiters",
		},
	)

	// GcResourcesPendingTotal is a gauge that tracks the number of resources pending deletion.
	gcResourcesPendingTotal = served.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zen_cleaner_resources_pending_total",
			Help: "Number of resources pending deletion (matched but TTL not expired)",
		},
		[]string{labelPolicyNamespace, labelPolicyName, labelResourceAPIVersion, labelResourceKind},
	)

	// CleanerLeaderElectionStatus is a gauge that tracks leader election status (1 = leader, 0 = follower).
	gcLeaderElectionStatus = served.NewGauge(
		prometheus.GaugeOpts{
			Name: "zen_cleaner_leader_election_status",
			Help: "Leader election status (1 if this instance is the leader, 0 otherwise)",
		},
	)

	// CleanerLeaderElectionTransitionsTotal is a counter that tracks the number of leader election transitions.
	gcLeaderElectionTransitionsTotal = served.NewCounter(
		prometheus.CounterOpts{
			Name: "zen_cleaner_leader_election_transitions_total",
			Help: "Total number of leader election transitions (becoming leader or losing leadership)",
		},
	)
)

// recordPolicyPhase records the current phase of a policy.
// This should be called with the actual count of policies in each phase,
// not incremented on every evaluation. The caller should count policies and call Set().
func recordPolicyPhase(phase string, count float64) {
	gcPoliciesTotal.WithLabelValues(phase).Set(count)
}

// recordResourceMatched records that a resource was matched by a policy.
func recordResourceMatched(policyNamespace, policyName, resourceAPIVersion, resourceKind string) {
	gcResourcesMatchedTotal.WithLabelValues(policyNamespace, policyName, resourceAPIVersion, resourceKind).Inc()
}

// recordResourceDeleted records that a resource was deleted.
func recordResourceDeleted(policyNamespace, policyName, resourceAPIVersion, resourceKind, reason string, duration float64) {
	gcResourcesDeletedTotal.WithLabelValues(policyNamespace, policyName, resourceAPIVersion, resourceKind, reason).Inc()
	gcDeletionDurationSeconds.WithLabelValues(policyNamespace, policyName, resourceAPIVersion, resourceKind).Observe(duration)
}

// recordError records an error that occurred during cleanup.
func recordError(policyNamespace, policyName, errorType string) {
	cleanerErrorsTotal.WithLabelValues(policyNamespace, policyName, errorType).Inc()
}

// recordEvaluationDuration records the time taken to evaluate a policy.
func recordEvaluationDuration(policyNamespace, policyName string, duration float64) {
	gcEvaluationDurationSeconds.WithLabelValues(policyNamespace, policyName).Observe(duration)
}

// recordInformerCount records the current number of active resource informers.
func recordInformerCount(count int) {
	gcInformersTotal.Set(float64(count))
}

// recordRateLimiterCount records the current number of active rate limiters.
func recordRateLimiterCount(count int) {
	gcRateLimitersTotal.Set(float64(count))
}

// recordResourcesPending records the number of resources pending deletion.
func recordResourcesPending(policyNamespace, policyName, resourceAPIVersion, resourceKind string, count int64) {
	gcResourcesPendingTotal.WithLabelValues(policyNamespace, policyName, resourceAPIVersion, resourceKind).Set(float64(count))
}

// recordLeaderElectionStatus records the current leader election status.
func recordLeaderElectionStatus(isLeader bool) {
	if isLeader {
		gcLeaderElectionStatus.Set(1)
	} else {
		gcLeaderElectionStatus.Set(0)
	}
}

// recordLeaderElectionTransition records a leader election transition.
func recordLeaderElectionTransition() {
	gcLeaderElectionTransitionsTotal.Inc()
}

// SUPPORT2-033 §7: bounded productization metrics. Labels are closed
// vocabularies (refusal reason classes, reconcile outcome classes, API error
// classes) — never namespace or object identity.

// safetyRefusalsTotal counts objects the safety gate refused to delete,
// keyed by bounded refusal reason class.
var safetyRefusalsTotal = served.NewCounterVec(
	prometheus.CounterOpts{
		Name: "zen_cleaner_safety_refusals_total",
		Help: "Objects the safety gate refused to delete, by refusal reason class",
	},
	[]string{"reason"},
)

// candidatesConsideredTotal counts objects that passed selector+condition
// matching and reached the safety/deletion decision point.
var candidatesConsideredTotal = served.NewCounter(
	prometheus.CounterOpts{
		Name: "zen_cleaner_candidates_considered_total",
		Help: "Matching objects that reached the deletion decision point",
	},
)

// deletionsAttemptedTotal counts DELETE calls issued (after safety gate).
var deletionsAttemptedTotal = served.NewCounter(
	prometheus.CounterOpts{
		Name: "zen_cleaner_deletions_attempted_total",
		Help: "DELETE calls issued after the safety gate allowed them",
	},
)

// deletionsFailedTotal counts DELETE calls that returned an error.
var deletionsFailedTotal = served.NewCounter(
	prometheus.CounterOpts{
		Name: "zen_cleaner_deletions_failed_total",
		Help: "DELETE calls that failed (after bounded retries)",
	},
)

// dryRunCandidatesTotal counts objects a dry-run policy would have deleted.
var dryRunCandidatesTotal = served.NewCounter(
	prometheus.CounterOpts{
		Name: "zen_cleaner_dry_run_candidates_total",
		Help: "Objects a dry-run policy would have deleted",
	},
)

// reconcileOutcomeTotal counts reconcile outcomes by bounded result class.
var reconcileOutcomeTotal = served.NewCounterVec(
	prometheus.CounterOpts{
		Name: "zen_cleaner_reconcile_total",
		Help: "Reconcile outcomes by result class (success, error, invalid_policy)",
	},
	[]string{"result"},
)

// apiErrorsTotal counts API errors by bounded class.
var apiErrorsTotal = served.NewCounterVec(
	prometheus.CounterOpts{
		Name: "zen_cleaner_api_errors_total",
		Help: "API errors by bounded class (conflict, forbidden, notfound, throttled, server_error, network, other)",
	},
	[]string{"class"},
)

// RecordSafetyRefusal counts a safety-gate refusal by reason class.
func RecordSafetyRefusal(reason string) {
	safetyRefusalsTotal.WithLabelValues(reason).Inc()
}

// RecordCandidateConsidered counts an object reaching the deletion decision.
func RecordCandidateConsidered() {
	candidatesConsideredTotal.Inc()
}

// RecordDeletionAttempted counts an issued DELETE call.
func RecordDeletionAttempted() {
	deletionsAttemptedTotal.Inc()
}

// RecordDeletionFailed counts a failed DELETE call.
func RecordDeletionFailed() {
	deletionsFailedTotal.Inc()
}

// RecordDryRunCandidate counts an object a dry-run policy would delete.
func RecordDryRunCandidate() {
	dryRunCandidatesTotal.Inc()
}

// RecordReconcileOutcome counts a reconcile outcome by result class.
func RecordReconcileOutcome(result string) {
	reconcileOutcomeTotal.WithLabelValues(result).Inc()
}

// RecordAPIError counts an API error by bounded class.
func RecordAPIError(err error) {
	apiErrorsTotal.WithLabelValues(apiErrorClass(err)).Inc()
}

// apiErrorClass maps a Kubernetes API error to a bounded class label.
func apiErrorClass(err error) string {
	if err == nil {
		return "other"
	}
	switch {
	case apierrors.IsConflict(err):
		return "conflict"
	case apierrors.IsNotFound(err):
		return "notfound"
	case apierrors.IsForbidden(err):
		return "forbidden"
	case apierrors.IsUnauthorized(err):
		return "unauthorized"
	case apierrors.IsTooManyRequests(err):
		return "throttled"
	case apierrors.IsServiceUnavailable(err), apierrors.IsInternalError(err), apierrors.IsServerTimeout(err), apierrors.IsTimeout(err):
		return "server_error"
	}
	msg := err.Error()
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "TLS handshake") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "proxy") {
		return "network"
	}
	return "other"
}
