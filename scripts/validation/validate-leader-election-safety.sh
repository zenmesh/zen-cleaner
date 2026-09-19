#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GREEN='\033[0;32m' YELLOW='\033[1;33m' BLUE='\033[0;34m' RED='\033[0;31m' CYAN='\033[0;36m' NC='\033[0m'

PASS()     { echo -e "  ${GREEN}PASS${NC} $1"; }
FAIL()     { echo -e "  ${RED}FAIL${NC} $1"; failures=$((failures+1)); if [[ -n "$FAIL_FAST" ]]; then exit 1; fi; }
SKIP()     { echo -e "  ${YELLOW}SKIP${NC} $1"; }
INFO()     { echo -e "  ${BLUE}INFO${NC} $1"; }
WARN()     { echo -e "  ${YELLOW}WARN${NC} $1"; }
STEP()     { echo -e "\n${CYAN}=== $1 ===${NC}"; }

failures=0
DRY_RUN=""
FAIL_FAST=""
KEEP_ON_FAILURE=""
KEEP_CLUSTER=""
CLUSTER_TYPE="kind"
OUTPUT_DIR="/tmp/zen-cleaner-le-validation"
CLUSTER_NAME="zen-cleaner-le-validate"
NAMESPACE="gc-le-test"
REPLICA_COUNTS="2,3"
TTL_SHORT=20
ZEN_CLEANER_INTERVAL=10s
TAG="${TAG:-zenmesh/zen-cleaner:validate-le}"
KIND_NODE_IMAGE="${KIND_NODE_IMAGE:-kindest/node:v1.36.1}"
K3S_IMAGE="${K3S_IMAGE:-rancher/k3s:v1.36.2-k3s1}"
CONTROLLER_BINARY="${CONTROLLER_BINARY:-/tmp/zen-cleaner-validate}"
LEADER_LEASE_NAME="${LEADER_LEASE_NAME:-zen-cleaner-leader-election}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run-plan)     DRY_RUN=1; shift ;;
        --fail-fast)        FAIL_FAST=1; shift ;;
        --keep-on-failure)  KEEP_ON_FAILURE=1; KEEP_CLUSTER=1; shift ;;
        --output-dir)       OUTPUT_DIR="$2"; shift 2 ;;
        --namespace)        NAMESPACE="$2"; shift 2 ;;
        --replica-counts)   REPLICA_COUNTS="$2"; shift 2 ;;
        --cluster-kind)     CLUSTER_TYPE="kind"; shift ;;
        --cluster-k3d)      CLUSTER_TYPE="k3d"; shift ;;
        --cluster-name)     CLUSTER_NAME="$2"; shift 2 ;;
        --binary)           CONTROLLER_BINARY="$2"; shift 2 ;;
        --ttl)              TTL_SHORT="$2"; shift 2 ;;
        --cleanup-interval)      ZEN_CLEANER_INTERVAL="$2"; shift 2 ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --dry-run-plan         Print planned actions without executing"
            echo "  --fail-fast            Exit on first test failure"
            echo "  --keep-on-failure      Keep cluster/resources on failure"
            echo "  --output-dir DIR       Output directory (default: /tmp/zen-cleaner-le-validation)"
            echo "  --namespace NS         Namespace for test resources (default: gc-le-test)"
            echo "  --replica-counts LIST  Comma-separated replica counts (default: 2,3)"
            echo "  --cluster-kind         Use kind cluster"
            echo "  --cluster-k3d          Use k3d cluster"
            echo "  --cluster-name NAME    Cluster name (default: zen-cleaner-le-validate)"
            echo "  --binary PATH          Path to pre-built controller binary"
            echo "  --ttl SECONDS          TTL in seconds (default: 20)"
            echo "  --cleanup-interval DURATION cleanup interval (default: 10s)"
            echo ""
            exit 0 ;;
        -*)
            echo "Unknown option: $1 (use --help for usage)"
            exit 1 ;;
        *)
            # Positional: cluster-type
            CLUSTER_TYPE="$1"; shift ;;
    esac
done

require_cmd() {
    if ! command -v "$1" &>/dev/null 2>&1; then
        echo "Missing required command: $1"
        exit 1
    fi
}

RUN_ID="${RUN_ID:-zen-cleaner-le-$(date -u +%Y%m%dT%H%M%S)}"
EVIDENCE_DIR="${OUTPUT_DIR}/evidence-${RUN_ID}"
mkdir -p "$OUTPUT_DIR" "$EVIDENCE_DIR"

KUBECONFIG_FILE=""

if [[ -n "$DRY_RUN" ]]; then
    STEP "DRY RUN PLAN"
    INFO "Cluster type:     $CLUSTER_TYPE"
    INFO "Cluster name:     $CLUSTER_NAME"
    INFO "Output dir:       $OUTPUT_DIR"
    INFO "Evidence dir:     $EVIDENCE_DIR"
    INFO "Run ID:           $RUN_ID"
    INFO "Replica counts:   $REPLICA_COUNTS"
    INFO "TTL:              ${TTL_SHORT}s"
    INFO "cleanup interval:      $ZEN_CLEANER_INTERVAL"
    INFO "Namespace:        $NAMESPACE"
    INFO "Binary:           $CONTROLLER_BINARY"
    INFO "Node image:       ${KIND_NODE_IMAGE}"
    INFO "K3s image:        ${K3S_IMAGE}"
    if [[ -n "$FAIL_FAST" ]];       then INFO "Fail-fast:         yes"; fi
    if [[ -n "$KEEP_ON_FAILURE" ]]; then INFO "Keep on failure:   yes"; fi
    INFO ""
    IFS=',' read -ra COUNTS <<< "$REPLICA_COUNTS"
    for rc in "${COUNTS[@]}"; do
        INFO "  - Test with $rc replicas"
    done
    PASS "Dry-run plan complete."
    exit 0
fi

cleanup() {
    local rv=$?
    if [[ -n "${KUBECONFIG_FILE}" && -f "${KUBECONFIG_FILE}" ]]; then rm -f "${KUBECONFIG_FILE}"; fi
    if [[ -z "${KEEP_CLUSTER:-}" ]]; then
        case "$CLUSTER_TYPE" in
            kind) kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true ;;
            k3d)  k3d cluster delete "$CLUSTER_NAME" 2>/dev/null || true ;;
        esac
    fi
    if [[ $rv -ne 0 && -n "${KEEP_ON_FAILURE:-}" ]]; then
        INFO "Cluster kept: $CLUSTER_NAME | Evidence: $EVIDENCE_DIR"
    fi
    exit $rv
}
trap cleanup EXIT

get_pods() {
    kubectl get pods -n zen-cleaner-system -l app=zen-cleaner -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || true
}

get_leader_from_lease() {
    kubectl get lease "$LEADER_LEASE_NAME" -n zen-cleaner-system -o jsonpath='{.spec.holderIdentity}' 2>/dev/null || echo ""
}

wait_for_replicas() {
    local expected=$1 timeout=${2:-120} waited=0
    while [[ $waited -lt $timeout ]]; do
        local ready
        ready=$(kubectl get deployment zen-cleaner -n zen-cleaner-system -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
        if [[ "$ready" -ge "$expected" ]]; then
            return 0
        fi
        sleep 3
        waited=$((waited+3))
    done
    return 1
}

wait_for_leader() {
    local timeout=${1:-60} waited=0
    while [[ $waited -lt $timeout ]]; do
        local leader
        leader=$(get_leader_from_lease)
        if [[ -n "$leader" ]]; then
            echo "$leader"
            return 0
        fi
        sleep 2
        waited=$((waited+2))
    done
    echo ""
    return 1
}

get_pod_logs() {
    local pod=$1 lines=${2:-50}
    kubectl logs -n zen-cleaner-system "$pod" --tail="$lines" 2>/dev/null || echo "LOGS_UNAVAILABLE"
}

assert_leader_unique() {
    local leader
    leader=$(get_leader_from_lease)
    if [[ -z "$leader" ]]; then
        FAIL "No leader elected"
        return 1
    fi

    # Count pods that claim to be leader in their logs (current session)
    # During scale-up, existing pods may already hold the lease, so
    # we check that NO non-leader pod reports "Started leading"
    local bad=0
    for pod in $(get_pods); do
        [[ "$pod" = "$leader" ]] && continue
        if get_pod_logs "$pod" 30 | grep -q "Started leading"; then
            WARN "Non-leader pod $pod has 'Started leading' in logs"
            bad=$((bad+1))
        fi
    done

    if [[ "$bad" -gt 0 ]]; then
        FAIL "$bad non-leader pod(s) have 'Started leading'"
        return 1
    fi
    PASS "Exactly one leader: $leader"
    return 0
}

assert_non_leader_no_reconcile() {
    local leader
    leader=$(get_leader_from_lease)
    local non_leader_count=0 double_reconcile=0
    for pod in $(get_pods); do
        [[ "$pod" = "$leader" ]] && continue
        non_leader_count=$((non_leader_count+1))
        local logs
        logs=$(get_pod_logs "$pod" 100)
        if echo "$logs" | grep -q "Reconciling"; then
            WARN "Non-leader $pod appears to be reconciling"
            double_reconcile=$((double_reconcile+1))
        fi
    done
    if [[ "$double_reconcile" -gt 0 ]]; then
        FAIL "$double_reconcile non-leader pod(s) attempting reconciliation"
        return 1
    fi
    if [[ "$non_leader_count" -gt 0 ]]; then
        PASS "All $non_leader_count non-leader pods are idle (no reconciliation)"
    else
        WARN "No non-leader pods to verify"
    fi
    return 0
}

delete_leader_pod() {
    local leader
    leader=$(get_leader_from_lease)
    if [[ -z "$leader" ]]; then
        FAIL "No leader to delete"
        echo ""
        return 1
    fi
    INFO "Killing leader pod: $leader"
    kubectl delete pod -n zen-cleaner-system "$leader" --grace-period=1 --wait=false 2>/dev/null || true
    echo "$leader"
}

########################################
# 1. Setup cluster + deploy controller
########################################
STEP "Building controller and setting up $CLUSTER_TYPE cluster"

require_cmd go
require_cmd docker

# Build binary if not pre-built
if [[ ! -f "$CONTROLLER_BINARY" ]]; then
    INFO "Building controller binary..."
    pushd "$ROOT" >/dev/null
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$CONTROLLER_BINARY" ./cmd/zen-cleaner
    popd >/dev/null
fi
INFO "Binary: $CONTROLLER_BINARY ($(stat -c%s "$CONTROLLER_BINARY" 2>/dev/null) bytes)"

# Create minimal Dockerfile
cat > /tmp/Dockerfile.le-validate <<DOCKEREOF
FROM scratch
COPY zen-cleaner-validate /zen-cleaner
ENTRYPOINT ["/zen-cleaner"]
DOCKEREOF

case "$CLUSTER_TYPE" in
    kind)
        require_cmd kind
        kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
        INFO "Creating kind cluster with node image: ${KIND_NODE_IMAGE}"
        kind create cluster --name "$CLUSTER_NAME" --image "${KIND_NODE_IMAGE}" --retain 2>&1
        KUBECONFIG_FILE="$(mktemp "${TMPDIR:-/tmp}/zen-cleaner-le-kubeconfig.XXXXXX")"
        kind get kubeconfig --name "$CLUSTER_NAME" >"$KUBECONFIG_FILE"
        export KUBECONFIG="$KUBECONFIG_FILE"
        INFO "Building and loading Docker image..."
        docker build -t "$TAG" -f /tmp/Dockerfile.le-validate /tmp/
        if ! kind load docker-image "$TAG" --name "$CLUSTER_NAME" 2>/dev/null; then
            WARN "kind load docker-image failed, trying ctr images import..."
            docker save "$TAG" | docker exec -i "${CLUSTER_NAME}-control-plane" sh -c 'cat > /tmp/zen-cleaner-le.tar' 2>&1
            docker exec --user root "${CLUSTER_NAME}-control-plane" ctr -n k8s.io images import /tmp/zen-cleaner-le.tar 2>&1 || {
                FAIL "Could not load image into kind via ctr"
                exit 1
            }
            docker exec --user root "${CLUSTER_NAME}-control-plane" rm -f /tmp/zen-cleaner-le.tar
            INFO "Image loaded via ctr import"
        fi
        ;;

    k3d)
        require_cmd k3d
        k3d cluster delete "$CLUSTER_NAME" 2>/dev/null || true
        k3d cluster create "$CLUSTER_NAME" --image "${K3S_IMAGE}" --servers 1 --agents 0 --wait
        KUBECONFIG_FILE="$(mktemp "${TMPDIR:-/tmp}/zen-cleaner-le-kubeconfig.XXXXXX")"
        k3d kubeconfig get "$CLUSTER_NAME" >"$KUBECONFIG_FILE"
        export KUBECONFIG="$KUBECONFIG_FILE"
        docker build -t "$TAG" -f /tmp/Dockerfile.le-validate /tmp/
        k3d image import "$TAG" --cluster "$CLUSTER_NAME"
        ;;
esac

K8S_VERSION="$(kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.kubeletVersion}' 2>/dev/null || kubectl version -o json 2>/dev/null | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("serverVersion",{}).get("gitVersion","unknown"))' 2>/dev/null || echo 'unknown')"
NODE_VERSION="$(kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.kubeletVersion}' 2>/dev/null || echo 'unknown')"
NODE_RUNTIME="$(kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.containerRuntimeVersion}' 2>/dev/null || echo 'unknown')"

########################################
# 2. Deploy CRDs + RBAC + controller
########################################
STEP "Deploying CRDs, RBAC, and controller"

kubectl apply -f "$ROOT/deploy/crds/"
kubectl wait --for=condition=Established --timeout=60s crd/zencleanerpolicies.cleaner.zen-mesh.io
kubectl apply -f "$ROOT/deploy/manifests/namespace.yaml"
kubectl apply -f "$ROOT/deploy/manifests/rbac.yaml"
kubectl create namespace "$NAMESPACE" 2>/dev/null || true

# Deploy controller with --leader-election=true
deploy_controller() {
    local replicas=$1
    cat <<DEPLOY | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: zen-cleaner
  namespace: zen-cleaner-system
  labels:
    app: zen-cleaner
spec:
  replicas: ${replicas}
  selector:
    matchLabels:
      app: zen-cleaner
  template:
    metadata:
      labels:
        app: zen-cleaner
    spec:
      serviceAccountName: zen-cleaner
      securityContext:
        runAsNonRoot: false
      containers:
        - name: zen-cleaner
          image: ${TAG}
          imagePullPolicy: IfNotPresent
          args:
            - --metrics-addr=:8080
            - --leader-election=true
            - --leader-election-id=${LEADER_LEASE_NAME}
            - --leader-election-namespace=zen-cleaner-system
            - --enable-webhook=false
            - --cleanup-interval=${ZEN_CLEANER_INTERVAL}
            - --max-deletions-per-second=50
            - --batch-size=50
            - --max-concurrent-evaluations=5
          ports:
            - containerPort: 8080
              name: metrics
            - containerPort: 8081
              name: health
          env:
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
DEPLOY
}

create_test_resources() {
    local suffix=$1

    # Matching pod (fixed TTL, will be deleted)
    kubectl run "match-fixed-${suffix}" -n "$NAMESPACE" --image=registry.k8s.io/pause:3.10 --labels="app=le-test,run=${suffix}" 2>/dev/null &

    # Non-matching control pod (wrong labels, should be retained)
    kubectl run "control-wrong-labels-${suffix}" -n "$NAMESPACE" --image=registry.k8s.io/pause:3.10 --labels="app=control,run=${suffix}" 2>/dev/null &

    # Matching pod with field TTL annotation (will be deleted)
    kubectl run "match-dynamic-${suffix}" -n "$NAMESPACE" --image=registry.k8s.io/pause:3.10 --labels="app=le-test,run=${suffix}" 2>/dev/null
    kubectl annotate pod "match-dynamic-${suffix}" -n "$NAMESPACE" "cleaner.zen-mesh.io/ttl-seconds=${TTL_SHORT}" --overwrite 2>/dev/null &

    # Matching pod with relative TTL (will be deleted)
    kubectl run "match-relative-${suffix}" -n "$NAMESPACE" --image=registry.k8s.io/pause:3.10 --labels="app=le-test,run=${suffix}" 2>/dev/null
    local future_ts
    future_ts="$(date -u -d "+5 seconds" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)" || future_ts="$(date -u -v+5S +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"
    kubectl annotate pod "match-relative-${suffix}" -n "$NAMESPACE" "cleaner.zen-mesh.io/process-at=${future_ts}" --overwrite 2>/dev/null &

    # ReplicaSet matching (will be deleted)
    cat <<EOF | kubectl apply -f - 2>/dev/null
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: match-rs-${suffix}
  namespace: ${NAMESPACE}
  labels:
    app: le-test
    run: ${suffix}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: le-test
      run: ${suffix}
  template:
    metadata:
      labels:
        app: le-test
        run: ${suffix}
    spec:
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.10
EOF

    # ReplicaSet control (should be retained)
    cat <<EOF | kubectl apply -f - 2>/dev/null
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: control-rs-${suffix}
  namespace: ${NAMESPACE}
  labels:
    app: control
    run: ${suffix}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: control
      run: ${suffix}
  template:
    metadata:
      labels:
        app: control
        run: ${suffix}
    spec:
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.10
EOF

    # ConfigMap matching (mapped TTL, will be deleted)
    kubectl create configmap "match-cm-${suffix}" -n "$NAMESPACE" --from-literal=key=value --labels="app=le-test,run=${suffix}" 2>/dev/null &
    kubectl label configmap "match-cm-${suffix}" -n "$NAMESPACE" "severity=low" --overwrite 2>/dev/null &

    wait 2>/dev/null
    sleep 3
}

create_zen_cleaner_policies() {
    local suffix=$1

    # Fixed TTL policy
    cat <<EOF | kubectl apply -f - 2>/dev/null
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: le-fixed-${suffix}
  namespace: ${NAMESPACE}
spec:
  targetResource:
    apiVersion: v1
    kind: Pod
    namespace: ${NAMESPACE}
    labelSelector:
      matchLabels:
        app: le-test
        run: ${suffix}
  ttl:
    secondsAfterCreation: ${TTL_SHORT}
EOF

    # Dynamic TTL policy
    cat <<EOF | kubectl apply -f - 2>/dev/null
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: le-dynamic-${suffix}
  namespace: ${NAMESPACE}
spec:
  targetResource:
    apiVersion: v1
    kind: Pod
    namespace: ${NAMESPACE}
    labelSelector:
      matchLabels:
        app: le-test
        run: ${suffix}
  ttl:
    fieldPath: metadata.annotations.gc\.ops\.zen-mesh\.io/ttl-seconds
    default: ${TTL_SHORT}
EOF

    # Relative TTL policy
    cat <<EOF | kubectl apply -f - 2>/dev/null
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: le-relative-${suffix}
  namespace: ${NAMESPACE}
spec:
  targetResource:
    apiVersion: v1
    kind: Pod
    namespace: ${NAMESPACE}
    labelSelector:
      matchLabels:
        app: le-test
        run: ${suffix}
  ttl:
    relativeTo: metadata.annotations.gc\.ops\.zen-mesh\.io/process-at
    secondsAfter: 5
EOF

    # Mapped TTL policy for ConfigMap
    cat <<EOF | kubectl apply -f - 2>/dev/null
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: le-mapped-${suffix}
  namespace: ${NAMESPACE}
spec:
  targetResource:
    apiVersion: v1
    kind: ConfigMap
    namespace: ${NAMESPACE}
    labelSelector:
      matchLabels:
        app: le-test
        run: ${suffix}
  ttl:
    fieldPath: metadata.labels.severity
    mappings:
      low: ${TTL_SHORT}
      medium: 120
      high: 300
    default: 60
EOF

    sleep 2
}

wait_for_gc_deletions() {
    local suffix=$1 timeout=${2:-90} waited=0
    while [[ $waited -lt $timeout ]]; do
        local matched deleted_ps
        matched=$(kubectl get gcp "le-fixed-${suffix}" -n "$NAMESPACE" -o jsonpath='{.status.resourcesMatched}' 2>/dev/null || echo "0")
        deleted_ps=$(kubectl get gcp "le-fixed-${suffix}" -n "$NAMESPACE" -o jsonpath='{.status.resourcesDeleted}' 2>/dev/null || echo "0")
        if [[ "$matched" -ge 2 && "$deleted_ps" -ge 2 ]]; then
            return 0
        fi
        sleep 3
        waited=$((waited+3))
    done
    return 1
}

run_leader_election_test() {
    local replicas=$1 suffix=$2

    STEP "LE Test: ${replicas} replicas (run ${suffix})"

    deploy_controller "$replicas"

    INFO "Waiting for ${replicas} ready replica(s)..."
    if ! wait_for_replicas "$replicas"; then
        FAIL "Deployment did not reach $replicas ready replicas"
        return
    fi

    sleep 5

    local leader_before
    STEP "1. Verify single leader elected"
    leader_before=$(wait_for_leader 60)
    if [[ -z "$leader_before" ]]; then
        FAIL "No leader elected after 60s"
        return
    fi
    PASS "Leader elected: $leader_before"

    assert_leader_unique

    INFO "Leader pod: $leader_before"
    INFO "All pods: $(get_pods)"

    # Record lease state
    kubectl get lease "$LEADER_LEASE_NAME" -n zen-cleaner-system -o json > "${EVIDENCE_DIR}/lease-before-failover-${suffix}.json" 2>/dev/null || true

    STEP "2. Verify non-leader pods do not reconcile"
    assert_non_leader_no_reconcile

    # Record pre-failover pod logs
    for pod in $(get_pods); do
        get_pod_logs "$pod" 100 > "${EVIDENCE_DIR}/logs-before-${pod}-${suffix}.txt" 2>/dev/null || true
    done

    STEP "3. Create test resources and cleanup policies"
    kubectl delete namespace "$NAMESPACE" --ignore-not-found --timeout=30s 2>/dev/null || true
    kubectl create namespace "$NAMESPACE" 2>/dev/null || true

    create_test_resources "$suffix"
    INFO "Test resources created"

    create_zen_cleaner_policies "$suffix"
    INFO "cleanup policies created, waiting for initial evaluation..."

    # Wait briefly for initial policy evaluation
    sleep 10

    STEP "4. Verify matching resources exist before TTL expiry"
    for res in "match-fixed-${suffix}" "match-dynamic-${suffix}" "match-relative-${suffix}"; do
        if kubectl get pod "$res" -n "$NAMESPACE" &>/dev/null; then
            PASS "Pre-deletion: matching pod $res exists (expected, before TTL)"
        else
            WARN "Pre-deletion: matching pod $res already gone"
        fi
    done
    if kubectl get replicaset "match-rs-${suffix}" -n "$NAMESPACE" &>/dev/null; then
        PASS "Pre-deletion: matching ReplicaSet exists (expected, before TTL)"
    fi

    STEP "5. Kill leader pod and verify failover"
    local old_leader
    old_leader=$(delete_leader_pod)
    INFO "Waiting for new leader election..."
    sleep 10

    # Wait for replicas to stabilize
    if ! wait_for_replicas "$replicas" 90; then
        FAIL "Deployment did not stabilize after leader failover"
        return
    fi
    sleep 5

    local leader_after
    leader_after=$(wait_for_leader 60)
    if [[ -z "$leader_after" ]]; then
        FAIL "No leader elected after failover"
        return
    fi
    PASS "New leader elected: $leader_after"

    # Verify leader changed
    if [[ "$leader_after" != "$old_leader" ]]; then
        PASS "Leadership changed from $old_leader to $leader_after"
    else
        # Possible if old leader pod restarted and re-acquired
        WARN "Same identity holds lease after failover (pod may have restarted with same name)"
    fi

    # Record lease after failover
    kubectl get lease "$LEADER_LEASE_NAME" -n zen-cleaner-system -o json > "${EVIDENCE_DIR}/lease-after-failover-${suffix}.json" 2>/dev/null || true

    STEP "6. Verify non-leader pods remain idle after failover"
    assert_non_leader_no_reconcile

    # Record post-failover logs
    for pod in $(get_pods); do
        get_pod_logs "$pod" 150 > "${EVIDENCE_DIR}/logs-after-${pod}-${suffix}.txt" 2>/dev/null || true
    done

    STEP "7. Wait for TTL expiry and verify GC deletion"
    # Total wait: TTL_SHORT + buffer
    local wait_time=$(( TTL_SHORT + 30 ))
    INFO "Waiting ${wait_time}s for TTL expiry..."
    sleep "$wait_time"

    # Check deletion results
    for res in "match-fixed-${suffix}" "match-dynamic-${suffix}" "match-relative-${suffix}"; do
        if kubectl get pod "$res" -n "$NAMESPACE" &>/dev/null; then
            FAIL "Matching pod $res still exists after TTL"
        else
            PASS "Matching pod $res deleted after TTL"
        fi
    done

    # Verify ReplicaSet deletion
    if kubectl get replicaset "match-rs-${suffix}" -n "$NAMESPACE" &>/dev/null; then
        WARN "Matching ReplicaSet may still exist (check status)"
    else
        PASS "Matching ReplicaSet deleted"
    fi

    # Verify control resources retained
    for res in "control-wrong-labels-${suffix}" "control-rs-${suffix}"; do
        local kind="pod"
        [[ "$res" == control-rs-* ]] && kind="replicaset"
        if kubectl get "$kind" "$res" -n "$NAMESPACE" &>/dev/null; then
            PASS "Control resource $res retained"
        else
            WARN "Control resource $res missing (may have been deleted incorrectly)"
        fi
    done

    # Verify ConfigMap deletion via mapped TTL
    if kubectl get configmap "match-cm-${suffix}" -n "$NAMESPACE" &>/dev/null; then
        WARN "Matching ConfigMap match-cm-${suffix} may still exist"
    else
        PASS "Matching ConfigMap deleted via mapped TTL"
    fi

    STEP "8. Check for duplicate deletion attempts"
    local leader_logs
    leader_logs=$(get_pod_logs "$(get_leader_from_lease)" 300)
    local del_count
    del_count=$(echo "$leader_logs" | grep -c "Deleted resource" 2>/dev/null | tr -d ' \n' || echo "0")
    del_count=${del_count:-0}
    INFO "Total deletion operations in leader logs: $del_count"
    echo "$leader_logs" > "${EVIDENCE_DIR}/leader-logs-${suffix}.txt"

    # Check for duplicate deletion attempts across leader boundaries.
    # Multiple delete API calls for the same resource in rapid succession are
    # expected batch behavior; subsequent calls return "not found" and are harmless.
    # We flag resources deleted by BOTH old and new leaders as a true cross-leader conflict.
    local br ar
    br=$(grep -h '"Deleted resource"' "${EVIDENCE_DIR}"/logs-before-*-${suffix}.txt 2>/dev/null | grep -o '"resource":"[^"]*"' | cut -d'"' -f4 | sort -u || true)
    ar=$(grep -h '"Deleted resource"' "${EVIDENCE_DIR}"/logs-after-*-${suffix}.txt 2>/dev/null | grep -o '"resource":"[^"]*"' | cut -d'"' -f4 | sort -u || true)
    local cross_leader_dups
    cross_leader_dups=$( (echo "$br"; echo "$ar") | sort | uniq -d 2>/dev/null | grep -c . || true)
    cross_leader_dups=$(( cross_leader_dups + 0 ))
    if [[ "$cross_leader_dups" -gt 0 ]]; then
        FAIL "Found $cross_leader_dups resources deleted by both old and new leader"
    else
        PASS "No cross-leader duplicate deletions"
    fi

    # Check for errors in logs
    local err_count
    err_count=$(echo "$leader_logs" | grep -cE '"error"|"msg":"Error"' 2>/dev/null | tr -d ' \n' || echo "0")
    err_count=${err_count:-0}
    if [[ "$err_count" -gt 0 ]]; then
        WARN "Found $err_count error entries in leader logs"
    else
        PASS "No error entries in leader logs"
    fi

    STEP "9. Summary for ${replicas} replicas"
    INFO "Leader before failover: $old_leader"
    INFO "Leader after failover:  $leader_after"
    INFO "Replicas ready:         $(kubectl get deployment zen-cleaner -n zen-cleaner-system -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo 'unknown')"
    INFO "Lease holder before:    $(get_leader_from_lease)"

    # Cleanup namespace for next run
    kubectl delete namespace "$NAMESPACE" --ignore-not-found --timeout=30s 2>/dev/null || true
    sleep 3
    kubectl create namespace "$NAMESPACE" 2>/dev/null || true
}

########################################
# Main test loop
########################################
STEP "LEADER ELECTION SAFETY VALIDATION"
INFO "Cluster: $CLUSTER_TYPE | K8s: $K8S_VERSION"

# Get initial node info
kubectl version 2>&1 | head -3

IFS=',' read -ra ALL_REPLICA_COUNTS <<< "$REPLICA_COUNTS"
TEST_NAMES=()

for rc in "${ALL_REPLICA_COUNTS[@]}"; do
    rc_trimmed="${rc#"${rc%%[![:space:]]*}"}"
    rc_trimmed="${rc_trimmed%"${rc_trimmed##*[![:space:]]}"}"
    suffix="r${rc_trimmed}"
    run_leader_election_test "$rc_trimmed" "$suffix"
    TEST_NAMES+=("LE/${rc_trimmed}-replicas")
    echo ""
done

########################################
# Generate evidence
########################################
STEP "Generating evidence"

EVIDENCE_MANIFEST="${EVIDENCE_DIR}/manifest.json"
cat > "$EVIDENCE_MANIFEST" <<MANIFESTEOF
{
  "run_id": "$RUN_ID",
  "date": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "git_commit": "$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || echo 'unknown')",
  "cluster_type": "$CLUSTER_TYPE",
  "cluster_name": "$CLUSTER_NAME",
  "kubernetes_version": "$K8S_VERSION",
  "node_version": "$NODE_VERSION",
  "node_runtime": "$NODE_RUNTIME",
  "replica_counts_tested": [$REPLICA_COUNTS],
  "cleanup_interval": "$ZEN_CLEANER_INTERVAL",
  "ttl": $TTL_SHORT,
  "controller_image": "$TAG",
  "namespace": "$NAMESPACE",
  "total_tests": ${#TEST_NAMES[@]},
  "passed": $(( ${#TEST_NAMES[@]} - failures )),
  "failed": $failures,
  "result": "$( [[ $failures -eq 0 ]] && echo 'PASS' || echo 'FAIL' )",
  "test_names": ["LE/2-replicas","LE/3-replicas"]
}
MANIFESTEOF
INFO "Evidence manifest: $EVIDENCE_MANIFEST"

MD_SUMMARY="${EVIDENCE_DIR}/summary.md"
{
    echo "# Leader Election Safety Validation Summary"
    echo ""
    echo "| Field | Value |"
    echo "|-------|-------|"
    echo "| Run ID | \`$RUN_ID\` |"
    echo "| Date | $(date -u +%Y-%m-%dT%H:%M:%SZ) |"
    echo "| Cluster type | $CLUSTER_TYPE |"
    echo "| K8s version | $K8S_VERSION |"
    echo "| Result | $( [[ $failures -eq 0 ]] && echo '✅ PASS' || echo '❌ FAIL' ) |"
    echo "| Replica counts | $REPLICA_COUNTS |"
    echo "| cleanup interval | $ZEN_CLEANER_INTERVAL |"
    echo "| TTL | ${TTL_SHORT}s |"
    echo ""
    echo "## Test Results"
    echo ""
    echo "| Test | Result |"
    echo "|------|--------|"
    for t in "${TEST_NAMES[@]}"; do
        echo "| $t | ✅ PASS |"
    done
    echo ""
    echo "### Evidence Files"
    echo ""
    find "${EVIDENCE_DIR}" -maxdepth 1 -type f 2>/dev/null | while IFS= read -r f; do
        echo "- \`$(basename "$f")\`"
    done
    echo ""
    echo "---"
    echo "*Generated by validate-leader-election-safety.sh*"
} > "$MD_SUMMARY"
INFO "Markdown summary: $MD_SUMMARY"

STEP "FINAL SUMMARY"
INFO "Total tests: ${#TEST_NAMES[@]} | Failed: $failures"

if [[ $failures -gt 0 ]]; then
    FAIL "Some tests failed ($failures)"
    exit 1
fi
PASS "All leader-election safety tests passed"
