// Copyright 2026 Zen Mesh. All rights reserved.

// Package opportunity defines the canonical MAINTENANCE OPPORTUNITY model:
// the typed fact that Cleaner's detectors emit and that policies, dry-runs,
// actions and receipts consume.
//
// LAWS (HELPER North-Star campaign):
//   - an opportunity is EVIDENCE, never authority: age is evidence, never
//     deletion authority; no detector output may execute by itself;
//   - no secrets, no raw high-cardinality metrics, no AI prose in canonical
//     fields — narrative explanation is DERIVED from deterministic fields;
//   - "unknown" biases toward protection (owner, cost, tenant, authority);
//   - cost classes are distinct: measured values are never called savings
//     and estimates are never called measured;
//   - every opportunity is stable under re-discovery (DedupKey) so repeat
//     scans surface the same fact, not a new one.
package opportunity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Type enumerates the maintenance taxonomy (§3). Detectors register the
// classes they emit; unregistered classes are a construction error.
type Type string

const (
	TypeStorageOrphan           Type = "STORAGE_ORPHAN"
	TypeStorageLongUnbound      Type = "STORAGE_LONG_UNBOUND"
	TypeStorageUnused           Type = "STORAGE_UNUSED"
	TypeStorageGrowthAnomaly    Type = "STORAGE_GROWTH_ANOMALY"
	TypeStorageRetentionMis     Type = "STORAGE_RETENTION_MISMATCH"
	TypeComputeOverRequested    Type = "COMPUTE_OVERREQUESTED"
	TypeComputeUnderRequested   Type = "COMPUTE_UNDERREQUESTED"
	TypeComputeRequestUnset     Type = "COMPUTE_REQUEST_UNSET"
	TypeComputeLimitSuspicious  Type = "COMPUTE_LIMIT_SUSPICIOUS"
	TypeComputeSingletonRisk    Type = "COMPUTE_SINGLETON_AVAILABILITY_RISK"
	TypeWorkloadCompletedStale  Type = "WORKLOAD_COMPLETED_STALE"
	TypeWorkloadCrashloopStale  Type = "WORKLOAD_CRASHLOOP_STALE"
	TypeWorkloadReplicaSetStale Type = "WORKLOAD_REPLICASET_STALE"
	TypeWorkloadTemporaryStale  Type = "WORKLOAD_TEMPORARY_STALE"
	TypeWorkloadOrphan          Type = "WORKLOAD_ORPHAN"
	TypeNetworkUnusedLB         Type = "NETWORK_UNUSED_LB"
	TypeNetworkUnusedRoute      Type = "NETWORK_UNUSED_ROUTE"
	TypeNetworkUnusedService    Type = "NETWORK_UNUSED_SERVICE"
	TypeNetworkBroadExposure    Type = "NETWORK_BROAD_EXPOSURE"
	TypeNetworkStaleGateway     Type = "NETWORK_STALE_GATEWAY_RESOURCE"
	TypeBackupStale             Type = "BACKUP_STALE"
	TypeBackupUnverified        Type = "BACKUP_UNVERIFIED"
	TypeBackupRetentionExcess   Type = "BACKUP_RETENTION_EXCESS"
	TypeRestoreNeverDrilled     Type = "RESTORE_NEVER_DRILLED"
	TypeCertExpiring            Type = "CERT_EXPIRING"
	TypeCertStale               Type = "CERT_STALE"
	TypeCertUnusedRef           Type = "CERT_UNUSED_REFERENCE"
	TypeImageStale              Type = "IMAGE_STALE"
	TypeImageUnreferenced       Type = "IMAGE_UNREFERENCED"
	TypeBuildCacheStale         Type = "BUILD_CACHE_STALE"
	TypeQualificationStale      Type = "QUALIFICATION_ARTIFACT_STALE"
	TypeSyntheticStale          Type = "SYNTHETIC_RESOURCE_STALE"
	TypeTmpResourceStale        Type = "TMP_RESOURCE_STALE"
	TypeDatabaseMaintenance     Type = "DATABASE_MAINTENANCE"
	TypeDatabaseEphemeralStale  Type = "DATABASE_STALE_EPHEMERAL_STATE"
	TypeDatabaseIndexOppty      Type = "DATABASE_INDEX_OPPORTUNITY"
	TypeDatabaseUnboundedGrowth Type = "DATABASE_UNBOUNDED_GROWTH"
	TypeObservabilityRetention  Type = "OBSERVABILITY_RETENTION"
	TypeObservabilityCard       Type = "OBSERVABILITY_CARDINALITY"
	TypeObservabilityStaleTgt   Type = "OBSERVABILITY_STALE_TARGET"
	TypeConfigDrift             Type = "CONFIG_DRIFT"
	TypeGenerationDrift         Type = "GENERATION_DRIFT"
	TypeReconciliationStuck     Type = "RECONCILIATION_STUCK"
	TypeSecurityConfigDebt      Type = "SECURITY_CONFIGURATION_DEBT"
	TypeCostOpportunity         Type = "COST_OPPORTUNITY"
	TypeManualReviewRequired    Type = "MANUAL_REVIEW_REQUIRED"
)

// Risk is the policy risk vocabulary (§5).
type Risk string

const (
	RiskR0Observe           Risk = "R0_OBSERVE"
	RiskR1SafeRegenerable   Risk = "R1_SAFE_REGENERABLE"
	RiskR2BoundedReversible Risk = "R2_BOUNDED_REVERSIBLE"
	RiskR3Consequential     Risk = "R3_CONSEQUENTIAL"
	RiskR4SecurityCritical  Risk = "R4_SECURITY_CRITICAL"
	RiskForbidden           Risk = "FORBIDDEN"
)

// Action is the typed action vocabulary (§4). There is deliberately NO
// generic DELETE, PATCH, kubectl or HTTP action.
type Action string

const (
	ActionReportOnly          Action = "REPORT_ONLY"
	ActionAcknowledge         Action = "ACKNOWLEDGE"
	ActionSuppressUntil       Action = "SUPPRESS_UNTIL"
	ActionRecheck             Action = "RECHECK"
	ActionTag                 Action = "TAG"
	ActionDeleteEphemeral     Action = "DELETE_EPHEMERAL"
	ActionDeleteConfirmedOrph Action = "DELETE_CONFIRMED_ORPHAN"
	ActionPruneCompletedJob   Action = "PRUNE_COMPLETED_JOB"
	ActionPruneTempCache      Action = "PRUNE_TEMP_CACHE"
	ActionRequestRightsize    Action = "REQUEST_RIGHTSIZE"
	ActionApplyRightsize      Action = "APPLY_RIGHTSIZE"
	ActionRequestStorageRel   Action = "REQUEST_STORAGE_RELEASE"
	ActionReleaseStorage      Action = "RELEASE_STORAGE"
	ActionRunRetention        Action = "RUN_RETENTION"
	ActionRequestBackup       Action = "REQUEST_BACKUP"
	ActionRequestRestoreDrill Action = "REQUEST_RESTORE_DRILL"
	ActionRotate              Action = "ROTATE"
	ActionReconcile           Action = "RECONCILE"
	ActionEscalateSupport     Action = "ESCALATE_SUPPORT"
	ActionStartMaestroWork    Action = "START_MAESTRO_WORKFLOW"
	ActionRequestRemediation  Action = "REQUEST_REMEDIATION"
)

// Evidence quality classes (§2 confidence/evidence quality).
type EvidenceQuality string

const (
	QualityMeasured EvidenceQuality = "MEASURED"
	QualityDerived  EvidenceQuality = "DERIVED"
	QualityHeuristic EvidenceQuality = "HEURISTIC"
	QualityUnknown  EvidenceQuality = "UNKNOWN"
)

// CostClass keeps cost truth distinct (§10, §63, §64).
type CostClass string

const (
	CostMeasuredResource CostClass = "MEASURED_RESOURCE"
	CostMeasuredLocal    CostClass = "MEASURED_LOCAL"
	CostEstListPrice     CostClass = "ESTIMATED_LIST_PRICE"
	CostCloudMeasured    CostClass = "CLOUD_MEASURED"
	CostUnknown          CostClass = "UNKNOWN"
)

// Protection is the protection lattice (§6): any applicable rule protects
// the candidate; unknown biases protected.
type Protection string

const (
	ProtCurrent            Protection = "CURRENT"
	ProtReferenced         Protection = "REFERENCED"
	ProtActive             Protection = "ACTIVE"
	ProtOwnerUnknown       Protection = "OWNER_UNKNOWN"
	ProtLegalRetention     Protection = "LEGAL_SECURITY_RETENTION"
	ProtAuditEvidence      Protection = "AUDIT_EVIDENCE"
	ProtBackupRequired     Protection = "BACKUP_REQUIRED"
	ProtRestoreSource      Protection = "RESTORE_SOURCE"
	ProtCustomerDurable    Protection = "CUSTOMER_DURABLE"
	ProtSystemCritical     Protection = "SYSTEM_CRITICAL"
	ProtForeignManaged     Protection = "FOREIGN_MANAGED"
	ProtAuthorityUnknown   Protection = "CROSS_NAMESPACE_AUTHORITY_UNKNOWN"
	ProtRecentGeneration   Protection = "RECENT_GENERATION"
	ProtRollbackState      Protection = "ROLLBACK_IMAGE_STATE"
	ProtMounted            Protection = "CURRENTLY_MOUNTED"
	ProtFinalizerActive    Protection = "FINALIZER_ACTIVE"
	ProtActiveLease        Protection = "ACTIVE_LEASE"
	ProtControllerUnresolv Protection = "CONTROLLER_OWNERSHIP_UNRESOLVED"
	ProtPolicyDenied       Protection = "POLICY_DENIED_SCOPE"
)

// State is the opportunity lifecycle.
type State string

const (
	StateOpen       State = "OPEN"
	StateSuppressed State = "SUPPRESSED"
	StateAckNowledged State = "ACKNOWLEDGED"
	StateActionQueued State = "ACTION_QUEUED"
	StateActionDone State = "ACTION_DONE"
	StateResolved   State = "RESOLVED"
)

// Opportunity is the canonical maintenance fact.
type Opportunity struct {
	// DedupKey is stable across re-discovery (derived, see DedupKey()).
	DedupKey string `json:"dedupKey"`

	// Type is the taxonomy class.
	Type Type `json:"type"`

	// ResourceClass / Identity reference the candidate without leaking
	// high-cardinality labels into metrics (identity lives HERE, not in
	// metric label sets).
	ResourceClass string `json:"resourceClass"`
	Identity      string `json:"identity"` // group/kind/namespace/name

	// Estate disambiguates multi-cluster facts (§44).
	Estate string `json:"estate,omitempty"`

	// Namespace / Tenant / Environment when applicable. Empty = N/A, and
	// empty MUST be treated as unknown, not global.
	Namespace    string `json:"namespace,omitempty"`
	Tenant       string `json:"tenant,omitempty"`
	Environment  string `json:"environment,omitempty"`

	// OwningProduct is the product/owner when PROVEN; empty = unknown
	// (protected, §62).
	OwningProduct string `json:"owningProduct,omitempty"`
	Detector      string `json:"detector"`

	// Discovery timestamps and age. Age is evidence only.
	DiscoveredAt time.Time  `json:"discoveredAt"`
	LastSeenAt   time.Time  `json:"lastSeenAt"`
	FirstObservedAge time.Duration `json:"firstObservedAge"`

	// CurrentState / ExpectedState are deterministic field snapshots.
	CurrentState  string `json:"currentState"`
	ExpectedState string `json:"expectedState"`

	// Reason is a bounded deterministic reason code, never prose.
	Reason string `json:"reason"`

	// Evidence carries measured fact references (digests/paths/query ids),
	// not payloads.
	Evidence        []EvidenceRef   `json:"evidence,omitempty"`
	EvidenceQuality EvidenceQuality `json:"evidenceQuality"`

	// Recommendation and available action classes.
	Recommendation    string   `json:"recommendation"` // deterministic verb phrase
	AvailableActions  []Action `json:"availableActions"`
	DryRunSupported   bool     `json:"dryRunSupported"`
	ExecutionPrereqs  []string `json:"executionPrereqs,omitempty"`

	// Impact / cost / risk / authority.
	Impact        string    `json:"impact,omitempty"`        // measured-resource phrasing only
	CostClass     CostClass `json:"costClass,omitempty"`     // UNKNOWN when not proven (§63)
	RiskClass     Risk      `json:"riskClass"`
	AuthorityClass string   `json:"authorityClass,omitempty"`

	// Policy binding and protections.
	PolicyRef    string       `json:"policyRef,omitempty"`
	Protections  []Protection `json:"protections,omitempty"` // non-empty => no destructive action

	// Suppression / disposition.
	State          State      `json:"state"`
	SuppressedUntil *time.Time `json:"suppressedUntil,omitempty"`
	ReceiptRefs    []string   `json:"receiptRefs,omitempty"`
}

// EvidenceRef points at measured evidence without embedding payloads.
type EvidenceRef struct {
	Kind   string `json:"kind"`   // e.g. metric-query, receipt, kube-object
	Ref    string `json:"ref"`    // bounded reference (path/digest/query id)
	At     time.Time `json:"at,omitempty"`
}

// DedupKey derives the stable identity: estate + type + identity + reason.
func (o *Opportunity) DedupKeyHash() string {
	h := sha256.Sum256([]byte(o.DedupKey))
	return hex.EncodeToString(h[:16])
}

// ComposeDedupKey builds the canonical key from the stable fields.
func ComposeDedupKey(estate string, t Type, identity, reason string) string {
	return strings.Join([]string{estate, string(t), identity, reason}, "|")
}

// Protected reports whether ANY protection applies (§6: unknown biases
// protected; a single rule is sufficient).
func (o *Opportunity) Protected() bool { return len(o.Protections) > 0 }

// ExecutableActions returns the available actions filtered by the
// protection law: destructive classes are removed while any protection
// holds; REPORT_ONLY always survives.
func (o *Opportunity) ExecutableActions() []Action {
	if !o.Protected() {
		return o.AvailableActions
	}
	out := make([]Action, 0, len(o.AvailableActions))
	for _, a := range o.AvailableActions {
		switch a {
		case ActionReportOnly, ActionAcknowledge, ActionSuppressUntil,
			ActionRecheck, ActionEscalateSupport, ActionRequestRemediation,
			ActionRequestRightsize, ActionRequestBackup,
			ActionRequestRestoreDrill, ActionStartMaestroWork:
			out = append(out, a) // advisory classes survive protection
		default:
			// destructive/boundable classes are suppressed by protection
		}
	}
	return out
}

// Explain derives the human narrative from deterministic fields (§2: no AI
// prose in canonical fields; explanation is GENERATED).
func (o *Opportunity) Explain() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s on %s", o.Type, o.Identity)
	if o.Estate != "" {
		fmt.Fprintf(&b, " (estate %s)", o.Estate)
	}
	fmt.Fprintf(&b, ": %s", o.Reason)
	if o.FirstObservedAge > 0 {
		fmt.Fprintf(&b, "; observed for %s", o.FirstObservedAge.Truncate(time.Second))
	}
	if len(o.Protections) > 0 {
		prots := make([]string, 0, len(o.Protections))
		for _, p := range o.Protections {
			prots = append(prots, string(p))
		}
		sort.Strings(prots)
		fmt.Fprintf(&b, "; PROTECTED (%s)", strings.Join(prots, ", "))
	}
	fmt.Fprintf(&b, "; recommendation: %s; risk %s; evidence %s",
		o.Recommendation, o.RiskClass, o.EvidenceQuality)
	return b.String()
}

// Registry validates that emitted types are part of the taxonomy.
func Registry() []Type {
	return []Type{
		TypeStorageOrphan, TypeStorageLongUnbound, TypeStorageUnused,
		TypeStorageGrowthAnomaly, TypeStorageRetentionMis,
		TypeComputeOverRequested, TypeComputeUnderRequested,
		TypeComputeRequestUnset, TypeComputeLimitSuspicious,
		TypeComputeSingletonRisk,
		TypeWorkloadCompletedStale, TypeWorkloadCrashloopStale,
		TypeWorkloadReplicaSetStale, TypeWorkloadTemporaryStale,
		TypeWorkloadOrphan,
		TypeNetworkUnusedLB, TypeNetworkUnusedRoute,
		TypeNetworkUnusedService, TypeNetworkBroadExposure,
		TypeNetworkStaleGateway,
		TypeBackupStale, TypeBackupUnverified, TypeBackupRetentionExcess,
		TypeRestoreNeverDrilled,
		TypeCertExpiring, TypeCertStale, TypeCertUnusedRef,
		TypeImageStale, TypeImageUnreferenced, TypeBuildCacheStale,
		TypeQualificationStale, TypeSyntheticStale, TypeTmpResourceStale,
		TypeDatabaseMaintenance, TypeDatabaseEphemeralStale,
		TypeDatabaseIndexOppty, TypeDatabaseUnboundedGrowth,
		TypeObservabilityRetention, TypeObservabilityCard,
		TypeObservabilityStaleTgt,
		TypeConfigDrift, TypeGenerationDrift, TypeReconciliationStuck,
		TypeSecurityConfigDebt, TypeCostOpportunity,
		TypeManualReviewRequired,
	}
}

// Valid reports whether t is in the registry.
func Valid(t Type) bool {
	for _, k := range Registry() {
		if k == t {
			return true
		}
	}
	return false
}
