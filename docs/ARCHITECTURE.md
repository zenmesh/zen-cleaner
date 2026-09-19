# Architecture

## Overview

`zen-cleaner` is a Kubernetes controller that implements declarative cleanup policies for Kubernetes resources (validated on Pod, ReplicaSet, ConfigMap, Secret, Job). It uses [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) for event-driven reconciliation, built-in leader election, and standard Kubernetes controller patterns.

## System Architecture

```mermaid
graph TB
    subgraph "Kubernetes Cluster"
        API[Kubernetes API Server]
        CRD[ZenCleanerPolicy CRD]
        Resources[Target Resources<br/>Pods, ConfigMaps, Jobs, etc.]
    end
    
    subgraph "Zen Cleaner Controller"
        Main[Main Controller]
        PolicyInformer[Policy Informer]
        ResourceInformer[Resource Informers<br/>Dynamic]
        Queue[Work Queue]
        Reconciler[Reconciler]
        RateLimiter[Rate Limiter]
        Metrics[Metrics Exporter]
    end
    
    subgraph "Components"
        StatusUpdater[Status Updater]
        EventRecorder[Event Recorder]
        Validator[Policy Validator]
    end
    
    API -->|Watch| PolicyInformer
    API -->|Watch| ResourceInformer
    PolicyInformer -->|Events| Queue
    ResourceInformer -->|Cache| Reconciler
    Queue -->|Reconcile| Reconciler
    Reconciler -->|Update| StatusUpdater
    Reconciler -->|Record| EventRecorder
    Reconciler -->|Validate| Validator
    Reconciler -->|Rate Limit| RateLimiter
    Reconciler -->|Delete| Resources
    Reconciler -->|Metrics| Metrics
    StatusUpdater -->|Update Status| CRD
    EventRecorder -->|Create Events| API
    Metrics -->|Expose| Prometheus
```

## Component Details

### 1. Main Controller (`cmd/zen-cleaner/main.go`)

The entry point that:
- Initializes Kubernetes clients (dynamic, core)
- Sets up leader election for HA
- Creates and starts the Zen Cleaner controller
- Configures metrics server
- Handles graceful shutdown

```mermaid
sequenceDiagram
    participant Main
    participant Manager
    participant Reconciler
    participant MetricsServer
    
    Main->>Manager: Create Manager (with leader election)
    Manager->>Manager: Start Leader Election
    Manager->>Reconciler: Setup Controller
    Manager->>Manager: Start Cache Sync
    Manager->>Reconciler: Trigger Reconcile (on policy changes)
    Reconciler->>Reconciler: Evaluate Policy
    Reconciler->>MetricsServer: Record Metrics
    MetricsServer->>MetricsServer: Expose Metrics
    Note over Reconciler: Event-Driven Reconciliation
    Main->>Manager: Stop (on signal)
    Manager->>Manager: Graceful Shutdown
```

### 2. Policy Reconciler (`pkg/controller/reconciler.go`)

Core reconciliation logic using controller-runtime:

**Responsibilities:**
- Reconcile `ZenCleanerPolicy` resources (event-driven)
- Create dynamic informers for target resources
- Evaluate policies against resources
- Delete resources that match TTL/conditions
- Update policy status
- Emit metrics and events

**Key Methods:**
- `NewPolicyReconciler()`: Initialize reconciler with clients
- `Reconcile()`: Main reconciliation function (triggered by policy changes)
- `evaluatePolicy()`: Evaluate single policy
- `deleteResource()`: Delete resource with rate limiting
- `SetupWithManager()`: Register reconciler with controller-runtime Manager

**Architecture:**
- Event-driven: Reconcile is triggered by policy changes (create, update, delete)
- Automatic requeue: Policies are requeued based on evaluation interval
- Built-in leader election: Only the leader processes policies
- Automatic cache sync: Manager handles cache synchronization

```mermaid
flowchart TD
    Start[Start Controller] --> Init[Initialize Informers]
    Init --> Watch[Watch Policies]
    Watch --> Event{Policy Event}
    Event -->|Add/Update| CreateInformer[Create Resource Informer]
    Event -->|Delete| RemoveInformer[Remove Resource Informer]
    CreateInformer --> Reconcile[Reconcile Loop]
    RemoveInformer --> Reconcile
    Reconcile --> GetPolicies[Get All Policies]
    GetPolicies --> ForEach{For Each Policy}
    ForEach -->|Active| Evaluate[Evaluate Policy]
    ForEach -->|Paused| Skip[Skip Policy]
    Evaluate --> GetResources[Get Resources from Cache]
    GetResources --> Filter[Filter by Selectors]
    Filter --> CheckTTL[Check TTL]
    CheckTTL -->|Expired| CheckConditions[Check Conditions]
    CheckTTL -->|Not Expired| Next[Next Resource]
    CheckConditions -->|Met| Delete[Delete Resource]
    CheckConditions -->|Not Met| Next
    Delete --> RateLimit[Rate Limiter]
    RateLimit --> DeleteAPI[Delete via API]
    DeleteAPI --> UpdateStatus[Update Policy Status]
    UpdateStatus --> EmitMetrics[Emit Metrics]
    EmitMetrics --> Next
    Next --> More{More Resources?}
    More -->|Yes| GetResources
    More -->|No| MorePolicies{More Policies?}
    MorePolicies -->|Yes| ForEach
    MorePolicies -->|No| Wait[Wait Interval]
    Wait --> Reconcile
```

### 3. Policy Evaluation Flow

```mermaid
flowchart LR
    Policy[ZenCleanerPolicy] --> Target[Target Resource Spec]
    Target --> Selectors[Label/Field Selectors]
    Target --> Namespace[Namespace Scope]
    
    Resources[Kubernetes Resources] --> Match{Match Selectors?}
    Selectors --> Match
    Namespace --> Match
    
    Match -->|Yes| TTL[Calculate TTL]
    Match -->|No| Skip[Skip Resource]
    
    TTL --> Expired{TTL Expired?}
    Expired -->|No| Skip
    Expired -->|Yes| Conditions[Check Conditions]
    
    Conditions --> Phase{Phase Match?}
    Conditions --> Labels{Labels Match?}
    Conditions --> Annotations{Annotations Match?}
    Conditions --> Fields{Field Conditions?}
    
    Phase -->|All Pass| Delete[Delete Resource]
    Labels -->|All Pass| Delete
    Annotations -->|All Pass| Delete
    Fields -->|All Pass| Delete
    
    Phase -->|Fail| Skip
    Labels -->|Fail| Skip
    Annotations -->|Fail| Skip
    Fields -->|Fail| Skip
    
    Delete --> RateLimit[Rate Limiter]
    RateLimit --> Behavior[Apply Behavior]
    Behavior --> DryRun{Dry Run?}
    DryRun -->|Yes| Log[Log Only]
    DryRun -->|No| DeleteAPI[Delete via API]
    DeleteAPI --> Metrics[Record Metrics]
    Metrics --> Event[Create Event]
```

### 4. Informer Architecture

```mermaid
graph TB
    subgraph "Policy Informer"
        PolicyGVR[Policy GVR<br/>cleaner.zen-mesh.io/v1alpha1]
        PolicyInformer[Shared Informer]
        PolicyStore[Policy Store]
    end
    
    subgraph "Resource Informers"
        ResourceGVR1[Resource GVR 1<br/>v1/ConfigMap]
        ResourceGVR2[Resource GVR 2<br/>apps/v1/Deployment]
        ResourceGVRN[Resource GVR N<br/>...]
        
        ResourceInformer1[Resource Informer 1]
        ResourceInformer2[Resource Informer 2]
        ResourceInformerN[Resource Informer N]
        
        ResourceStore1[Resource Store 1]
        ResourceStore2[Resource Store 2]
        ResourceStoreN[Resource Store N]
    end
    
    PolicyGVR --> PolicyInformer
    PolicyInformer --> PolicyStore
    
    ResourceGVR1 --> ResourceInformer1
    ResourceGVR2 --> ResourceInformer2
    ResourceGVRN --> ResourceInformerN
    
    ResourceInformer1 --> ResourceStore1
    ResourceInformer2 --> ResourceStore2
    ResourceInformerN --> ResourceStoreN
    
    PolicyStore -->|Policies| Controller[Zen Cleaner Controller]
    ResourceStore1 -->|Resources| Controller
    ResourceStore2 -->|Resources| Controller
    ResourceStoreN -->|Resources| Controller
```

### 5. Rate Limiting

```mermaid
sequenceDiagram
    participant Reconciler
    participant RateLimiter
    participant TokenBucket
    participant API
    
    Reconciler->>RateLimiter: Wait() for deletion
    RateLimiter->>TokenBucket: Request Token
    TokenBucket->>TokenBucket: Check Available Tokens
    alt Tokens Available
        TokenBucket->>Reconciler: Token Granted
        Reconciler->>API: Delete Resource
        API-->>Reconciler: Success
    else No Tokens
        TokenBucket->>Reconciler: Wait
        Note over TokenBucket: Refill Tokens
        TokenBucket->>Reconciler: Token Granted
        Reconciler->>API: Delete Resource
    end
```

### 6. Metrics Flow

```mermaid
graph LR
    Controller[Zen Cleaner Controller] --> Metrics[Metrics Package]
    Metrics --> Gauge[Gauges<br/>Policies by Phase]
    Metrics --> Counter[Counters<br/>Deletions, Errors]
    Metrics --> Histogram[Histograms<br/>Durations]
    
    Gauge --> Prometheus[Prometheus]
    Counter --> Prometheus
    Histogram --> Prometheus
    
    Prometheus --> Grafana[Grafana Dashboards]
    Prometheus --> Alerts[PrometheusRules]
```

## Component Interaction Diagrams

### Controller Lifecycle

```mermaid
stateDiagram-v2
    [*] --> Initializing: Start
    Initializing --> Starting: Initialize Clients
    Starting --> Running: Start Informers
    Running --> Evaluating: Cleanup Loop
    Evaluating --> Running: Evaluation Complete
    Running --> Stopping: Stop Signal
    Stopping --> [*]: Cleanup Complete
    
    note right of Running
        Main operational state
        Processing policies
        Deleting resources
    end note
```

### Policy Lifecycle

```mermaid
stateDiagram-v2
    [*] --> Created: Policy Created
    Created --> Validating: Webhook Validation
    Validating --> Active: Valid
    Validating --> Error: Invalid
    Active --> Evaluating: Cleanup Loop
    Evaluating --> Active: Continue
    Active --> Paused: Pause Request
    Paused --> Active: Resume Request
    Active --> Deleted: Policy Deleted
    Error --> Active: Fixed
    Deleted --> [*]: Cleanup Complete
    
    note right of Active
        Policy is processing resources
        Updating status
        Deleting expired resources
    end note
```

### Resource Deletion Flow

```mermaid
flowchart TD
    Start[Resource Matches Policy] --> CheckTTL{TTL Expired?}
    CheckTTL -->|No| Pending[Mark as Pending]
    CheckTTL -->|Yes| CheckConditions{Conditions Met?}
    CheckConditions -->|No| Skip[Skip Deletion]
    CheckConditions -->|Yes| RateLimit[Rate Limiter Wait]
    RateLimit --> BatchCheck{In Batch?}
    BatchCheck -->|No| AddBatch[Add to Batch]
    AddBatch --> WaitMore[Wait for More]
    BatchCheck -->|Yes| ProcessBatch[Process Batch]
    ProcessBatch --> Delete[Delete Resource]
    Delete --> Backoff{Success?}
    Backoff -->|Yes| UpdateStatus[Update Policy Status]
    Backoff -->|No| Retry{Retries Left?}
    Retry -->|Yes| ExponentialBackoff[Exponential Backoff]
    ExponentialBackoff --> Delete
    Retry -->|No| RecordError[Record Error]
    UpdateStatus --> Metrics[Update Metrics]
    RecordError --> Metrics
    Metrics --> Done[Done]
    Pending --> Done
    Skip --> Done
```

## Data Flow

### Policy Creation Flow

```mermaid
sequenceDiagram
    participant User
    participant API
    participant CRD
    participant Controller
    participant Informer
    
    User->>API: Create ZenCleanerPolicy
    API->>CRD: Store Policy
    CRD->>Informer: Policy Added Event
    Informer->>Controller: Policy Event
    Controller->>Controller: Create Resource Informer
    Controller->>Informer: Watch Target Resources
    Informer->>Controller: Resource Events
    Controller->>Controller: Evaluate Policy
    Controller->>API: Delete Resources (if needed)
    Controller->>CRD: Update Policy Status
```

### Resource Deletion Flow

```mermaid
sequenceDiagram
    participant Controller
    participant RateLimiter
    participant Validator
    participant API
    participant Resource
    participant StatusUpdater
    participant EventRecorder
    
    Controller->>Validator: Validate Policy
    Validator-->>Controller: Valid
    Controller->>RateLimiter: Wait for Rate Limit
    RateLimiter-->>Controller: Token Granted
    Controller->>API: Delete Resource
    API->>Resource: Delete Request
    Resource-->>API: Deleted
    API-->>Controller: Success
    Controller->>StatusUpdater: Update Status
    Controller->>EventRecorder: Record Event
    Controller->>Controller: Record Metrics
```

## High Availability

```mermaid
graph TB
    subgraph "Leader Election"
        Pod1[Zen Cleaner Controller Pod 1]
        Pod2[Zen Cleaner Controller Pod 2]
        Pod3[Zen Cleaner Controller Pod 3]
        Lease[Lease Resource]
    end
    
    Pod1 -->|Acquire Lease| Lease
    Pod2 -->|Try Acquire| Lease
    Pod3 -->|Try Acquire| Lease
    
    Lease -->|Leader| Pod1
    Lease -.->|Follower| Pod2
    Lease -.->|Follower| Pod3
    
    Pod1 -->|Active| Work[Process Policies]
    Pod2 -.->|Standby| Wait[Wait]
    Pod3 -.->|Standby| Wait
    
    Pod1 -.->|Fail| Lease
    Lease -->|New Leader| Pod2
```

## Security Model

```mermaid
graph TB
    subgraph "RBAC"
        SA[ServiceAccount]
        CR[ClusterRole]
        CRB[ClusterRoleBinding]
    end
    
    subgraph "Permissions"
        Read[Read Policies]
        Watch[Watch Resources]
        Delete[Delete Resources]
        Update[Update Status]
        Events[Create Events]
    end
    
    subgraph "Pod Security"
        NonRoot[Non-Root User]
        ReadOnly[Read-Only Root FS]
        DropAll[Drop All Capabilities]
    end
    
    SA --> CRB
    CRB --> CR
    CR --> Read
    CR --> Watch
    CR --> Delete
    CR --> Update
    CR --> Events
    
    Pod --> NonRoot
    Pod --> ReadOnly
    Pod --> DropAll
```

## Deployment Architecture

```mermaid
graph TB
    subgraph "Kubernetes Cluster"
        subgraph "Namespace: zen-cleaner-system"
            Deployment[Zen Cleaner Controller Deployment]
            Service[Service<br/>Metrics Port]
            SA[ServiceAccount]
        end
        
        subgraph "Cluster Scope"
            CRD[ZenCleanerPolicy CRD]
            CR[ClusterRole]
            CRB[ClusterRoleBinding]
        end
        
        subgraph "User Namespaces"
            Policy1[Policy: cleanup-configmaps]
            Policy2[Policy: cleanup-pods]
            PolicyN[Policy: ...]
        end
    end
    
    Deployment --> SA
    SA --> CRB
    CRB --> CR
    CR --> CRD
    CR --> Resources[Target Resources]
    
    Policy1 --> Deployment
    Policy2 --> Deployment
    PolicyN --> Deployment
    
    Deployment --> Service
    Service --> Prometheus[Prometheus]
```

## Performance Considerations

### Informer Caching

- **Policy Informer**: Single informer for all policies (cluster-wide or namespace-scoped)
- **Resource Informers**: One informer per unique GVR (GroupVersionResource)
- **Cache Efficiency**: Resources cached in memory, reducing API server load
- **Resync Period**: Configurable resync interval (default: 1 minute)

### Selector Performance

**Label Selectors vs Field Selectors:**

- **Label Selectors** (`labelSelector`): 
  - ✅ Pushed down to the Kubernetes API server
  - ✅ Reduces network traffic and API server load
  - ✅ Only matching resources are fetched and cached
  - ✅ Recommended for best performance

- **Field Selectors** (`fieldSelector`):
  - ⚠️ Evaluated in-memory only (not pushed to API server)
  - ⚠️ All resources matching GVR/namespace/labelSelector are fetched first
  - ⚠️ Filtering happens in controller memory after fetch
  - ⚠️ Does not reduce API server load or network traffic
  - ⚠️ Can increase memory usage for large resource sets

**Best Practice:** Prefer `labelSelector` when possible. Use `fieldSelector` only when label-based filtering is not feasible.

### Rate Limiting

- **Token Bucket Algorithm**: Smooth rate limiting with burst support
- **Per-Policy Rate**: Each policy can specify `maxDeletionsPerSecond`
- **Default Rate**: 10 deletions/second (configurable)
- **Batching**: Optional batch size for efficient deletions

### Scalability

- **Horizontal Scaling**: Multiple controller replicas with leader election
- **Resource Limits**: Configurable CPU/memory limits
- **Worker Threads**: Configurable number of worker goroutines
- **Queue Depth**: Work queue prevents memory bloat

## Error Handling

```mermaid
flowchart TD
    Error[Error Occurs] --> Type{Error Type}
    
    Type -->|API Error| Retry{Retry?}
    Type -->|Validation Error| Log[Log Error]
    Type -->|Rate Limit| Wait[Wait & Retry]
    
    Retry -->|Yes| Backoff[Exponential Backoff]
    Retry -->|No| Log
    
    Backoff --> RetryAPI[Retry API Call]
    RetryAPI --> Success{Success?}
    Success -->|Yes| Continue[Continue]
    Success -->|No| MaxRetries{Max Retries?}
    
    MaxRetries -->|No| Backoff
    MaxRetries -->|Yes| RecordError[Record Error Metric]
    RecordError --> Event[Create Error Event]
    Event --> Log
    
    Log --> Metrics[Update Error Metrics]
    Metrics --> Continue
```

## Monitoring & Observability

### Metrics

- **Policy Metrics**: Number of policies by phase
- **Resource Metrics**: Matched, deleted, pending resources
- **Performance Metrics**: Evaluation duration, deletion duration
- **Error Metrics**: Error counts by type

### Events

- **Policy Events**: Policy evaluation started/completed/failed
- **Resource Events**: Resource deleted with reason
- **Error Events**: Deletion failures, status update failures

### Logging

- **Structured Logging**: Using klog with structured fields
- **Log Levels**: Configurable verbosity (V levels)
- **Context**: Policy name, resource name, namespace in logs

## Extension Points

### Custom TTL Calculations

The controller supports multiple TTL calculation methods:
- Fixed TTL (`secondsAfterCreation`)
- Field-based TTL (`fieldPath`)
- Relative TTL (`relativeTo`)
- Mapped TTL (`mappings`)

### Custom Conditions

Policies can specify complex conditions:
- Phase matching
- Label matching
- Annotation matching
- Field conditions (equals, not equals, in, not in, etc.)

### Behavior Customization

Each policy can customize deletion behavior:
- Rate limiting (`maxDeletionsPerSecond`)
- Batch size (`batchSize`)
- Dry run mode (`dryRun`)
- Grace period (`gracePeriodSeconds`)
- Propagation policy (`propagationPolicy`)

## Operational Maturity

zen-cleaner is a community OSS project at **v0.0.1-alpha** maturity (API version `v1alpha1`). It includes metrics, alerting, and health checks for self-managed deployments. Review [docs/claims.md](docs/claims.md) and [docs/evidence/README.md](docs/evidence/README.md) for evidenced coverage before using in production-critical paths.

| Category | Status | Notes |
|----------|--------|-------|
| **Metrics** | ✅ Implemented | 11 Prometheus gauges/counters/histograms |
| **Tests** | ⚠️ Partial | 65% overall coverage gate enforced; integration tests in CI |
| **Documentation** | ✅ Present | Full docs tree including operations, security, trust layer |
| **Alert Rules** | ⚠️ Good | Prometheus alerting rules included |
| **Health Checks** | ⚠️ Basic | Leader election + reconciler health |
| **Security** | ⚠️ Present | Non-root, read-only root FS, restricted PSP, govulncheck + gosec in CI |

### Metrics

**11 Comprehensive Metrics**:
- `zen_cleaner_policies_total` - Policies by phase (gauge)
- `zen_cleaner_resources_matched_total` - Resources matched (counter)
- `zen_cleaner_resources_deleted_total` - Resources deleted (counter)
- `gc_deletion_duration_seconds` - Deletion latency (histogram)
- `zen_cleaner_errors_total` - Errors by type (counter)
- `gc_evaluation_duration_seconds` - Evaluation latency (histogram)
- `gc_informers_total` - Active informers (gauge)
- `gc_rate_limiters_total` - Active rate limiters (gauge)
- `zen_cleaner_resources_pending_total` - Pending deletions (gauge)
- `gc_leader_election_status` - Leader status (gauge)
- `gc_leader_election_transitions_total` - Transitions (counter)

See [METRICS.md](METRICS.md) for complete documentation.

### Test Coverage

**Current Coverage**: **65.4%** overall ✅ (meets 65% minimum via `make coverage`; see [TESTING.md](TESTING.md))

| Package | Coverage | Status |
|---------|----------|--------|
| `pkg/config` | 95.0% | ✅ Excellent |
| `pkg/errors` | 100.0% | ✅ Perfect |
| `pkg/validation` | 87.6% | ✅ Excellent |
| `pkg/webhook` | 80.3% | ✅ Good |
| `internal/config` | 93.9% | ✅ Excellent |
| `pkg/controller` | 51.4% | ⚠️ Low unit % (see below) |

**Coverage Requirements**:
- **Minimum**: 65% overall (`make coverage`; **enforced in CI** on every PR)
- **Stretch goal**: >80% overall; >85% on critical packages
- **`pkg/controller`**: Unit coverage is intentionally lower (~51%); critical paths are covered by [integration tests](TESTING.md#integration-tests) and **E2E/kind** (`make e2e-kind`). Do not treat controller unit % alone as a merge blocker—see [TESTING.md — pkg/controller unit coverage](TESTING.md#pkgcontroller-unit-coverage).

### Security

- ✅ Non-root container execution
- ✅ Read-only root filesystem
- ✅ Dropped capabilities
- ✅ RBAC with least-privilege principles
- ✅ Image security scanning
- ✅ Secret management best practices

See [SECURITY.md](../SECURITY.md) and [SECRET_MANAGEMENT.md](SECRET_MANAGEMENT.md) for details.

### Known Limitations

1. **Per-Policy Informers**: Each policy creates its own informer, which can scale to ~50-100 policies. For larger deployments, consider shared informer architecture (see [ROADMAP.md](../ROADMAP.md)).

2. **GVR Resolution**: Uses pluralization fallback when RESTMapper is unavailable. Most resources work correctly, but irregular CRDs may require explicit resource names.

### Recommendations

⚠️ **Review claims and evidence** — see [docs/claims.md](docs/claims.md) and [docs/evidence/README.md](docs/evidence/README.md) before using in production-critical paths. This is a v0.0.1-alpha community OSS project.

**Future Enhancements** (optional):
- Raise `pkg/controller` coverage toward 65%+ per package and 80%+ overall
- Enhanced health checks with informer sync verification
- Shared informer architecture for >100 policies

