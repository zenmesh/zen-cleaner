# Metrics Documentation

This document describes all Prometheus metrics exposed by the Zen Cleaner controller.

## Metrics Endpoint

The Zen Cleaner controller exposes metrics on the `/metrics` endpoint, defaulting to port `8080`.

## Available Metrics

### `zen_cleaner_policies_total`
**Type**: Gauge  
**Description**: Total number of cleanup policies  
**Labels**:
- `phase`: Policy phase (Active, Paused, Error)

**Example**:
```
zen_cleaner_policies_total{phase="Active"} 5
zen_cleaner_policies_total{phase="Paused"} 1
```

---

### `zen_cleaner_resources_matched_total`
**Type**: Counter  
**Description**: Total number of resources matched by cleanup policies  
**Labels**:
- `policy_namespace`: Namespace of the cleanup policy
- `policy_name`: Name of the cleanup policy
- `resource_api_version`: API version of the matched resource
- `resource_kind`: Kind of the matched resource

**Example**:
```
zen_cleaner_resources_matched_total{policy_namespace="default",policy_name="cleanup-temp-configmaps",resource_api_version="v1",resource_kind="ConfigMap"} 1250
```

---

### `zen_cleaner_resources_deleted_total`
**Type**: Counter  
**Description**: Total number of resources deleted by cleanup  
**Labels**:
- `policy_namespace`: Namespace of the cleanup policy
- `policy_name`: Name of the cleanup policy
- `resource_api_version`: API version of the deleted resource
- `resource_kind`: Kind of the deleted resource
- `reason`: Reason for deletion (ttl_expired, condition_not_met, etc.)

**Example**:
```
zen_cleaner_resources_deleted_total{policy_namespace="default",policy_name="cleanup-temp-configmaps",resource_api_version="v1",resource_kind="ConfigMap",reason="ttl_expired"} 1200
```

---

### `zen_cleaner_deletion_duration_seconds`
**Type**: Histogram  
**Description**: Time taken to delete resources  
**Labels**:
- `policy_namespace`: Namespace of the cleanup policy
- `policy_name`: Name of the cleanup policy
- `resource_api_version`: API version of the deleted resource
- `resource_kind`: Kind of the deleted resource

**Buckets**: Default Prometheus buckets (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)

**Example**:
```
zen_cleaner_deletion_duration_seconds_bucket{policy_namespace="default",policy_name="cleanup-temp-configmaps",resource_api_version="v1",resource_kind="ConfigMap",le="0.1"} 1150
```

---

### `zen_cleaner_errors_total`
**Type**: Counter  
**Description**: Total number of cleanup errors  
**Labels**:
- `policy_namespace`: Namespace of the cleanup policy
- `policy_name`: Name of the cleanup policy
- `error_type`: Type of error (informer_creation_failed, deletion_failed, status_update_failed, etc.)

**Example**:
```
zen_cleaner_errors_total{policy_namespace="default",policy_name="cleanup-temp-configmaps",error_type="deletion_failed"} 5
```

---

### `zen_cleaner_evaluation_duration_seconds`
**Type**: Histogram  
**Description**: Time taken to evaluate cleanup policies  
**Labels**:
- `policy_namespace`: Namespace of the cleanup policy
- `policy_name`: Name of the cleanup policy

**Buckets**: [0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0]

**Example**:
```
zen_cleaner_evaluation_duration_seconds_bucket{policy_namespace="default",policy_name="cleanup-temp-configmaps",le="0.1"} 1
```

---

### `zen_cleaner_informers_total`
**Type**: Gauge  
**Description**: Number of active resource informers (one per policy)  
**Labels**:
- `policy_namespace`: Namespace of the cleanup policy
- `policy_name`: Name of the cleanup policy

**Example**:
```
zen_cleaner_informers_total{policy_namespace="default",policy_name="cleanup-temp-configmaps"} 1
```

---

### `zen_cleaner_rate_limiters_total`
**Type**: Gauge  
**Description**: Number of active rate limiters (one per policy)  
**Labels**:
- `policy_namespace`: Namespace of the cleanup policy
- `policy_name`: Name of the cleanup policy

**Example**:
```
zen_cleaner_rate_limiters_total{policy_namespace="default",policy_name="cleanup-temp-configmaps"} 1
```

---

### `zen_cleaner_resources_pending_total`
**Type**: Gauge  
**Description**: Number of resources pending deletion (matched but TTL not expired)  
**Labels**:
- `policy_namespace`: Namespace of the cleanup policy
- `policy_name`: Name of the cleanup policy
- `resource_api_version`: API version of the resource
- `resource_kind`: Kind of the resource

**Example**:
```
zen_cleaner_resources_pending_total{policy_namespace="default",policy_name="cleanup-temp-configmaps",resource_api_version="v1",resource_kind="ConfigMap"} 50
```

---

### `zen_cleaner_leader_election_status`
**Type**: Gauge  
**Description**: Leader election status (1 if this instance is the leader, 0 otherwise)  
**Labels**: None

**Example**:
```
zen_cleaner_leader_election_status 1
```

---

### `zen_cleaner_leader_election_transitions_total`
**Type**: Counter  
**Description**: Total number of leader election transitions (becoming leader or losing leadership)  
**Labels**: None

**Example**:
```
zen_cleaner_leader_election_transitions_total 3
```

---

## Health Check Endpoints

### `/healthz`
**Description**: Health check endpoint  
**Returns**: `200 OK` if the controller is running

### `/readyz`
**Description**: Readiness check endpoint  
**Returns**: 
- `200 OK` if the controller is ready to serve requests
- `503 Service Unavailable` if leader election is enabled and this instance is not the leader

---

## Example Prometheus Queries

### Total resources deleted per policy
```promql
sum by (policy_namespace, policy_name) (zen_cleaner_resources_deleted_total)
```

### Deletion rate per policy
```promql
rate(zen_cleaner_resources_deleted_total[5m])
```

### Average deletion duration
```promql
histogram_quantile(0.95, zen_cleaner_deletion_duration_seconds)
```

### Error rate
```promql
rate(zen_cleaner_errors_total[5m])
```

### Policies by phase
```promql
zen_cleaner_policies_total
```

### Leader election status
```promql
zen_cleaner_leader_election_status
```

### Leader election transition rate
```promql
rate(zen_cleaner_leader_election_transitions_total[5m])
```

### Active informers per policy
```promql
zen_cleaner_informers_total
```

### Active rate limiters per policy
```promql
zen_cleaner_rate_limiters_total
```

### Resources pending deletion
```promql
sum by (policy_namespace, policy_name) (zen_cleaner_resources_pending_total)
```

---

## Grafana Dashboard

A sample Grafana dashboard JSON is available in `config/dashboards/zen-cleaner.json` (to be created).

