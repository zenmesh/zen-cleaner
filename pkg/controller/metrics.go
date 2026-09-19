package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

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
	gcPoliciesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zen_cleaner_policies_total",
			Help: "Total number of cleanup policies",
		},
		[]string{labelPhase},
	)

	// CleanerResourcesMatchedTotal is a counter that tracks the total number of resources matched by GC policies.
	gcResourcesMatchedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zen_cleaner_resources_matched_total",
			Help: "Total number of resources matched by cleanup policies",
		},
		[]string{labelPolicyNamespace, labelPolicyName, labelResourceAPIVersion, labelResourceKind},
	)

	// CleanerResourcesDeletedTotal is a counter that tracks the total number of resources deleted by GC.
	gcResourcesDeletedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zen_cleaner_resources_deleted_total",
			Help: "Total number of resources deleted by cleanup",
		},
		[]string{labelPolicyNamespace, labelPolicyName, labelResourceAPIVersion, labelResourceKind, labelReason},
	)

	// CleanerDeletionDurationSeconds is a histogram that tracks the time taken to delete resources.
	gcDeletionDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "zen_cleaner_deletion_duration_seconds",
			Help:    "Time taken to delete resources",
			Buckets: prometheus.DefBuckets,
		},
		[]string{labelPolicyNamespace, labelPolicyName, labelResourceAPIVersion, labelResourceKind},
	)

	// CleanerErrorsTotal is a counter that tracks the total number of GC errors.
	cleanerErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "zen_cleaner_errors_total",
			Help: "Total number of cleanup errors",
		},
		[]string{labelPolicyNamespace, labelPolicyName, labelErrorType},
	)

	// CleanerEvaluationDurationSeconds is a histogram that tracks the time taken to evaluate policies.
	gcEvaluationDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "zen_cleaner_evaluation_duration_seconds",
			Help:    "Time taken to evaluate cleanup policies",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0},
		},
		[]string{labelPolicyNamespace, labelPolicyName},
	)

	// CleanerInformersTotal is a gauge that tracks the total number of active resource informers.
	gcInformersTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "zen_cleaner_informers_total",
			Help: "Total number of active resource informers",
		},
	)

	// CleanerRateLimitersTotal is a gauge that tracks the total number of active rate limiters.
	gcRateLimitersTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "zen_cleaner_rate_limiters_total",
			Help: "Total number of active rate limiters",
		},
	)

	// GcResourcesPendingTotal is a gauge that tracks the number of resources pending deletion.
	gcResourcesPendingTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zen_cleaner_resources_pending_total",
			Help: "Number of resources pending deletion (matched but TTL not expired)",
		},
		[]string{labelPolicyNamespace, labelPolicyName, labelResourceAPIVersion, labelResourceKind},
	)

	// CleanerLeaderElectionStatus is a gauge that tracks leader election status (1 = leader, 0 = follower).
	gcLeaderElectionStatus = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "zen_cleaner_leader_election_status",
			Help: "Leader election status (1 if this instance is the leader, 0 otherwise)",
		},
	)

	// CleanerLeaderElectionTransitionsTotal is a counter that tracks the number of leader election transitions.
	gcLeaderElectionTransitionsTotal = promauto.NewCounter(
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
