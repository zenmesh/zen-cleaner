#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GREEN='\033[0;32m' YELLOW='\033[1;33m' BLUE='\033[0;34m' RED='\033[0;31m' CYAN='\033[0;36m' NC='\033[0m'

PASS()     { echo -e "  ${GREEN}PASS${NC} $1"; }
FAIL()     { echo -e "  ${RED}FAIL${NC} $1"; failures=$((failures+1)); if [[ -n "$FAIL_FAST" ]]; then exit 1; fi; }
SKIP()     { echo -e "  ${YELLOW}SKIP${NC} $1"; }
BLOCKED()  { echo -e "  ${RED}BLOCKED${NC} $1"; is_blocked=1; }
CLEANUP_FAIL() { echo -e "  ${RED}CLEANUP_FAIL${NC} $1"; }
INFO()     { echo -e "  ${BLUE}INFO${NC} $1"; }
WARN()     { echo -e "  ${YELLOW}WARN${NC} $1"; }
STEP()     { echo -e "\n${CYAN}=== $1 ===${NC}"; }

# --- Argument parsing ---
DRY_RUN=""
FAIL_FAST=""
KEEP_ON_FAILURE=""
CLUSTER_TYPE="kind"
OUTPUT_DIR="/tmp/zen-cleaner-validation"
CLUSTER_NAME="zen-cleaner-validate"
NAMESPACE_PREFIX=""
TTL_MODE_FILTER=""
RESOURCE_KIND_FILTER=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run-plan)     DRY_RUN=1; shift ;;
        --fail-fast)        FAIL_FAST=1; shift ;;
        --keep-on-failure)  KEEP_ON_FAILURE=1; shift ;;
        --output-dir)       OUTPUT_DIR="$2"; shift 2 ;;
        --namespace)        NAMESPACE_PREFIX="$2"; shift 2 ;;
        --ttl-mode)         TTL_MODE_FILTER="$2"; shift 2 ;;
        --resource-kind)    RESOURCE_KIND_FILTER="$2"; shift 2 ;;
        --cluster-kind)     CLUSTER_TYPE="kind"; shift ;;
        --cluster-k3d)      CLUSTER_TYPE="k3d"; shift ;;
        --cluster-kubeadm)  CLUSTER_TYPE="kubeadm"; shift ;;
        --cluster-name)     CLUSTER_NAME="$2"; shift 2 ;;
        -h|--help)
            echo "Usage: $0 [options] [cluster-type] [output-dir] [cluster-name]"
            echo ""
            echo "Options:"
            echo "  --dry-run-plan       Print planned actions without executing"
            echo "  --fail-fast          Exit on first test failure"
            echo "  --keep-on-failure    Keep cluster/resources on failure (default: cleanup)"
            echo "  --output-dir DIR     Output directory for evidence (default: /tmp/zen-cleaner-validation)"
            echo "  --namespace PREFIX   Namespace prefix for test resources (default: gc-<mode>)"
            echo "  --ttl-mode MODE      Only run specified TTL mode (fixed|dynamic|mapped|relative)"
            echo "  --resource-kind KIND Only run specified resource kind (Pod|ReplicaSet)"
            echo "  --cluster-kind       Use kind cluster (default if not specified)"
            echo "  --cluster-k3d        Use k3d cluster"
            echo "  --cluster-kubeadm    Use existing kubeadm cluster"
            echo "  --cluster-name NAME  Cluster name (default: zen-cleaner-validate)"
            echo ""
            echo "Positional args (legacy):"
            echo "  1: cluster-type (kind|k3d|kubeadm)    default: kind"
            echo "  2: output-dir                          default: /tmp/zen-cleaner-validation"
            echo "  3: cluster-name                        default: zen-cleaner-validate"
            echo ""
            echo "Environment variables (backward-compat):"
            echo "  KEEP_CLUSTER, ZEN_CLEANER_INTERVAL, TTL_SHORT, TAG, KIND_NODE_IMAGE, K3S_IMAGE"
            exit 0
            ;;
        -*)
            echo "Unknown option: $1 (use --help for usage)"
            exit 1
            ;;
        *)
            # Positional args (legacy)
            if [[ -z "$(eval echo \$arg1_set 2>/dev/null)" ]]; then
                CLUSTER_TYPE="$1"; arg1_set=1
            elif [[ -z "$(eval echo \$arg2_set 2>/dev/null)" ]]; then
                OUTPUT_DIR="$1"; arg2_set=1
            elif [[ -z "$(eval echo \$arg3_set 2>/dev/null)" ]]; then
                CLUSTER_NAME="$1"; arg3_set=1
            else
                echo "Too many positional arguments: $1"
                exit 1
            fi
            shift
            ;;
    esac
done

# Legacy env var support
KEEP_CLUSTER="${KEEP_CLUSTER:-$KEEP_ON_FAILURE}"
ZEN_CLEANER_INTERVAL="${ZEN_CLEANER_INTERVAL:-20s}"
TTL_SHORT="${TTL_SHORT:-15}"
TAG="${TAG:-zenmesh/zen-cleaner:validate}"
KIND_NODE_IMAGE="${KIND_NODE_IMAGE:-kindest/node:v1.36.1}"
K3S_IMAGE="${K3S_IMAGE:-rancher/k3s:v1.36.2-k3s1}"
failures=0
is_blocked=0
KUBECONFIG_FILE=""
CERT_DIR=""
RUN_ID="${RUN_ID:-zen-cleaner-validate-$(date -u +%Y%m%dT%H%M%S)}"
EVIDENCE_DIR="${OUTPUT_DIR}/evidence-${RUN_ID}"

mkdir -p "$OUTPUT_DIR" "$EVIDENCE_DIR"

require_cmd() {
    if ! command -v "$1" &>/dev/null 2>&1; then
        echo "Missing required command: $1"
        exit 1
    fi
}

# Dry-run plan mode
if [[ -n "$DRY_RUN" ]]; then
    STEP "DRY RUN PLAN"
    INFO "Cluster type:     $CLUSTER_TYPE"
    INFO "Cluster name:     $CLUSTER_NAME"
    INFO "Output dir:       $OUTPUT_DIR"
    INFO "Evidence dir:     $EVIDENCE_DIR"
    INFO "Run ID:           $RUN_ID"
    INFO "cleanup interval:      $ZEN_CLEANER_INTERVAL"
    INFO "TTL (short):      ${TTL_SHORT}s"
    INFO "Controller image: $TAG"
    INFO "KIND node image:  ${KIND_NODE_IMAGE}"
    INFO "K3s image:        ${K3S_IMAGE}"
    if [[ -n "$TTL_MODE_FILTER" ]];    then INFO "TTL mode filter:  $TTL_MODE_FILTER"; fi
    if [[ -n "$RESOURCE_KIND_FILTER" ]]; then INFO "Resource filter:   $RESOURCE_KIND_FILTER"; fi
    if [[ -n "$FAIL_FAST" ]];          then INFO "Fail-fast:         yes"; fi
    if [[ -n "$KEEP_ON_FAILURE" ]];    then INFO "Keep on failure:   yes"; fi
    INFO ""
    INFO "Planned resources:"
    for kind in Pod ReplicaSet; do
        [[ -n "$RESOURCE_KIND_FILTER" && "$kind" != "$RESOURCE_KIND_FILTER" ]] && continue
        for ttl_mode in fixed dynamic mapped relative; do
            [[ -n "$TTL_MODE_FILTER" && "$ttl_mode" != "$TTL_MODE_FILTER" ]] && continue
            ns_prefix="${kind,,}-${ttl_mode}"
            [[ -n "$NAMESPACE_PREFIX" ]] && ns_prefix="${NAMESPACE_PREFIX}-${ttl_mode}"
            INFO "  - ${kind} / ${ttl_mode} → namespace gc-${ns_prefix}"
        done
    done
    INFO ""
    PASS "Dry-run plan complete. No changes made."
    exit 0
fi

cleanup() {
    local rv=$?
    if [[ -n "${KUBECONFIG_FILE}" && -f "${KUBECONFIG_FILE}" ]]; then rm -f "${KUBECONFIG_FILE}"; fi
    if [[ -n "${CERT_DIR}" && -d "${CERT_DIR}" ]]; then rm -rf "${CERT_DIR}"; fi
    if [[ -z "${KEEP_CLUSTER}" ]]; then
        case "$CLUSTER_TYPE" in
            kind) kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || CLEANUP_FAIL "kind cluster deletion failed" ;;
            k3d)  k3d cluster delete "$CLUSTER_NAME" 2>/dev/null || CLEANUP_FAIL "k3d cluster deletion failed" ;;
        esac
    fi
    if [[ $rv -ne 0 && -n "$KEEP_ON_FAILURE" ]]; then
        INFO "Cluster kept for debugging: $CLUSTER_NAME"
        INFO "Evidence: $EVIDENCE_DIR"
    fi
    exit $rv
}
trap cleanup EXIT

########################################
# 1. Standalone binary build (static)
########################################
STEP "Building controller binary"

require_cmd go
BINARY="/tmp/zen-cleaner-validate"
pushd "$ROOT" >/dev/null
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" -o "$BINARY" ./cmd/zen-cleaner
popd >/dev/null
INFO "Binary: $BINARY ($(stat -c%s "$BINARY" 2>/dev/null || stat -f%z "$BINARY" 2>/dev/null) bytes)"

########################################
# 2. Containerize & push to cluster
########################################
STEP "Containerizing controller & deploying to $CLUSTER_TYPE"

case "$CLUSTER_TYPE" in
    kind)
        require_cmd kind
        require_cmd docker
        kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
        INFO "Using node image: ${KIND_NODE_IMAGE}"
        kind create cluster --name "$CLUSTER_NAME" --image "${KIND_NODE_IMAGE}" --retain 2>&1
        KUBECONFIG_FILE="$(mktemp "${TMPDIR:-/tmp}/zen-cleaner-kubeconfig.XXXXXX")"
        kind get kubeconfig --name "$CLUSTER_NAME" >"$KUBECONFIG_FILE"
        export KUBECONFIG="$KUBECONFIG_FILE"

        # Build minimal Docker image with pre-built binary
        cat > /tmp/Dockerfile.validate <<DOCKEREOF
FROM scratch
COPY zen-cleaner-validate /zen-cleaner
ENTRYPOINT ["/zen-cleaner"]
DOCKEREOF
        docker build -t "$TAG" -f /tmp/Dockerfile.validate /tmp/
        kind load docker-image "$TAG" --name "$CLUSTER_NAME"
        ;;

    k3d)
        require_cmd k3d
        k3d cluster delete "$CLUSTER_NAME" 2>/dev/null || true
        k3d cluster create "$CLUSTER_NAME" --image "${K3S_IMAGE}" --servers 1 --agents 0 --wait
        KUBECONFIG_FILE="$(mktemp "${TMPDIR:-/tmp}/zen-cleaner-kubeconfig.XXXXXX")"
        k3d kubeconfig get "$CLUSTER_NAME" >"$KUBECONFIG_FILE"
        export KUBECONFIG="$KUBECONFIG_FILE"

        # Build image and import
        docker build -t "$TAG" -f /tmp/Dockerfile.validate /tmp/
        k3d image import "$TAG" --cluster "$CLUSTER_NAME"
        ;;

    kubeadm)
        # Assume cluster is already running, kubeconfig in default location
        export KUBECONFIG="${KUBECONFIG:-/etc/kubernetes/admin.conf}"
        if [[ ! -f "$KUBECONFIG" ]]; then
            FAIL "kubeadm mode requires KUBECONFIG (default /etc/kubernetes/admin.conf)"
            exit 1
        fi
        INFO "kubeadm mode: using existing cluster, building fresh image"
        docker build -t "$TAG" -f /tmp/Dockerfile.validate /tmp/
        ;;

    *)
        FAIL "Unknown cluster type: $CLUSTER_TYPE (kind|k3d|kubeadm)"
        exit 1
        ;;
esac

kubectl version --short 2>&1 | head -1
K8S_VERSION="$(kubectl version --short 2>/dev/null | head -1 | awk '{print $3}')"
NODE_IMAGE="$(kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.containerRuntimeVersion}' 2>/dev/null || echo 'unknown')"

########################################
# 3. Deploy CRDs, controller, RBAC
########################################
STEP "Deploying CRDs, namespace, RBAC, controller"

kubectl apply -f "$ROOT/deploy/crds/"
kubectl wait --for=condition=Established --timeout=60s crd/zencleanerpolicies.cleaner.zen-mesh.io

kubectl apply -f "$ROOT/deploy/manifests/namespace.yaml"
kubectl apply -f "$ROOT/deploy/manifests/rbac.yaml"

# Generate webhook TLS
CERT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/zen-cleaner-certs.XXXXXX")"
openssl genrsa -out "${CERT_DIR}/ca.key" 2048 2>/dev/null
openssl req -x509 -new -nodes -key "${CERT_DIR}/ca.key" -sha256 -days 2 \
    -out "${CERT_DIR}/ca.crt" -subj "/CN=gc-validate-webhook-ca" 2>/dev/null
openssl genrsa -out "${CERT_DIR}/tls.key" 2048 2>/dev/null
openssl req -new -key "${CERT_DIR}/tls.key" -out "${CERT_DIR}/server.csr" \
    -subj "/CN=zen-cleaner-webhook.zen-cleaner-system.svc" 2>/dev/null
openssl x509 -req -in "${CERT_DIR}/server.csr" -CA "${CERT_DIR}/ca.crt" -CAkey "${CERT_DIR}/ca.key" \
    -CAcreateserial -out "${CERT_DIR}/tls.crt" -days 2 -sha256 \
    -extfile <(printf '%s\n' 'subjectAltName=DNS:zen-cleaner-webhook.zen-cleaner-system.svc,DNS:zen-cleaner-webhook.zen-cleaner-system.svc.cluster.local') 2>/dev/null

kubectl create secret tls zen-cleaner-webhook-cert -n zen-cleaner-system \
    --cert="${CERT_DIR}/tls.crt" --key="${CERT_DIR}/tls.key" \
    --dry-run=client -o yaml | kubectl apply -f -
CA_BUNDLE="$(base64 -w0 "${CERT_DIR}/ca.crt")"

# Deploy controller with modified image (single replica, insecure webhook, short interval)
kubectl apply -f - <<DEPLOY
apiVersion: apps/v1
kind: Deployment
metadata:
  name: zen-cleaner
  namespace: zen-cleaner-system
  labels:
    app: zen-cleaner
spec:
  replicas: 1
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
            - --webhook-addr=:9443
            - --leader-election=true
            - --leader-election-namespace=zen-cleaner-system
            - --enable-webhook=false
            - --insecure-webhook=true
            - --cleanup-interval=${ZEN_CLEANER_INTERVAL}
            - --max-deletions-per-second=50
            - --batch-size=50
            - --max-concurrent-evaluations=5
          ports:
            - containerPort: 8080
              name: metrics
            - containerPort: 8081
              name: health
          startupProbe:
            httpGet:
              path: /healthz
              port: health
            initialDelaySeconds: 0
            periodSeconds: 5
            timeoutSeconds: 3
            failureThreshold: 30
          livenessProbe:
            httpGet:
              path: /healthz
              port: health
            initialDelaySeconds: 10
            periodSeconds: 10
            timeoutSeconds: 5
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /readyz
              port: health
            initialDelaySeconds: 5
            periodSeconds: 5
            timeoutSeconds: 3
            failureThreshold: 3
          env:
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
DEPLOY

# Wait for controller to be ready
INFO "Waiting for controller pod..."
kubectl rollout status deployment/zen-cleaner -n zen-cleaner-system --timeout=120s

LEADER_POD="$(kubectl get pods -n zen-cleaner-system -l app=zen-cleaner -o jsonpath='{.items[0].metadata.name}')"
INFO "Controller pod: $LEADER_POD"
kubectl logs -n zen-cleaner-system "$LEADER_POD" --tail=5 2>/dev/null || true

########################################
# 4. Validation functions
########################################

wait_for_gc_cycle() {
    local policy_ns="$1" policy_name="$2" expected_deleted="$3" timeout="${4:-60}"
    local waited=0
    while [[ $waited -lt $timeout ]]; do
        local del
        del="$(kubectl get gcp "$policy_name" -n "$policy_ns" -o jsonpath='{.status.resourcesDeleted}' 2>/dev/null || echo "0")"
        if [[ "$del" -ge "$expected_deleted" ]]; then
            return 0
        fi
        sleep 2
        waited=$((waited+2))
    done
    return 1
}

run_ttl_scenario() {
    local test_name="$1"  apiver="$2"  kind="$3"  gvr_resource="$4"
    local ttl_mode="$5"   ns_prefix="$6"
    local ns="gc-${ns_prefix}"
    local policy_name="ttl-${ttl_mode}-${ns_prefix}"

    STEP "TTL Mode: ${ttl_mode}, Resource: ${kind} (${ns})"

    kubectl delete namespace "$ns" --ignore-not-found --timeout=30s 2>/dev/null
    kubectl create namespace "$ns"

    # Create resources
    case "$kind" in
        Pod)
            # -- matching pod (will be deleted)
            kubectl run "match-${ttl_mode}" -n "$ns" --image=registry.k8s.io/pause:3.10 --labels="app=test,tier=${ns_prefix}" &
            # -- control pod (wrong labels, should survive)
            kubectl run "control-${ttl_mode}" -n "$ns" --image=registry.k8s.io/pause:3.10 --labels="app=control,tier=${ns_prefix}" &
            # -- another-ns control pod (different namespace, should survive)
            kubectl create namespace "gc-${ns_prefix}-other" 2>/dev/null || true
            kubectl run "other-ns-${ttl_mode}" -n "gc-${ns_prefix}-other" --image=registry.k8s.io/pause:3.10 --labels="app=test,tier=${ns_prefix}" &
            wait
            ;;

        ReplicaSet)
            local ns_other="gc-${ns_prefix}-other"
            kubectl create namespace "$ns_other" 2>/dev/null || true

            # matching
            cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: match-${ttl_mode}
  namespace: ${ns}
  labels:
    app: test
    tier: ${ns_prefix}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test
      tier: ${ns_prefix}
  template:
    metadata:
      labels:
        app: test
        tier: ${ns_prefix}
    spec:
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.10
EOF

            # control (wrong labels)
            cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: control-${ttl_mode}
  namespace: ${ns}
  labels:
    app: control
    tier: ${ns_prefix}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: control
      tier: ${ns_prefix}
  template:
    metadata:
      labels:
        app: control
        tier: ${ns_prefix}
    spec:
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.10
EOF

            # other namespace control
            cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: other-ns-${ttl_mode}
  namespace: ${ns_other}
  labels:
    app: test
    tier: ${ns_prefix}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test
      tier: ${ns_prefix}
  template:
    metadata:
      labels:
        app: test
        tier: ${ns_prefix}
    spec:
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.10
EOF
            wait
            ;;

        *)
            FAIL "Unknown resource kind: $kind"
            return
            ;;
    esac

    # Wait for resources to be Ready, verify match/control exist
    sleep 3
    INFO "Verifying resources created..."
    for res in match-${ttl_mode} control-${ttl_mode}; do
        if kubectl get "$gvr_resource" "$res" -n "$ns" &>/dev/null; then
            PASS "Resource $res exists"
        else
            FAIL "Resource $res should exist before GC"
        fi
    done

    # Build GCP based on TTL mode
    local gcp_yaml=""
    local GCP_NS="$ns"

    case "$ttl_mode" in
        fixed)
            gcp_yaml=$(cat <<EOF
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: ${policy_name}
  namespace: ${ns}
spec:
  targetResource:
    apiVersion: ${apiver}
    kind: ${kind}
    namespace: ${ns}
    labelSelector:
      matchLabels:
        app: test
        tier: ${ns_prefix}
  ttl:
    secondsAfterCreation: ${TTL_SHORT}
EOF
)
            ;;

        dynamic)
            # Add an annotation with TTL seconds on matching resources
            for res in match-${ttl_mode}; do
                kubectl annotate "$gvr_resource" "$res" -n "$ns" "cleaner.zen-mesh.io/ttl-seconds=${TTL_SHORT}" --overwrite
            done
            gcp_yaml=$(cat <<EOF
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: ${policy_name}
  namespace: ${ns}
spec:
  targetResource:
    apiVersion: ${apiver}
    kind: ${kind}
    namespace: ${ns}
    labelSelector:
      matchLabels:
        app: test
        tier: ${ns_prefix}
  ttl:
    fieldPath: metadata.annotations.gc\.ops\.zen-mesh\.io/ttl-seconds
    default: ${TTL_SHORT}
EOF
)
            ;;

        mapped)
            # Add a label with severity on matching resources
            for res in match-${ttl_mode}; do
                kubectl label "$gvr_resource" "$res" -n "$ns" "severity=low" --overwrite
            done
            gcp_yaml=$(cat <<EOF
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: ${policy_name}
  namespace: ${ns}
spec:
  targetResource:
    apiVersion: ${apiver}
    kind: ${kind}
    namespace: ${ns}
    labelSelector:
      matchLabels:
        app: test
        tier: ${ns_prefix}
  ttl:
    fieldPath: metadata.labels.severity
    mappings:
      low: ${TTL_SHORT}
      medium: 120
      high: 300
    default: 60
EOF
)
            ;;

        relative)
            # Set an RFC3339 timestamp annotation on matching resources
            local future_ts
            future_ts="$(date -u -d "+5 seconds" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)" || future_ts="$(date -u -v+5S +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"
            for res in match-${ttl_mode}; do
                kubectl annotate "$gvr_resource" "$res" -n "$ns" "cleaner.zen-mesh.io/process-at=${future_ts}" --overwrite
            done
            gcp_yaml=$(cat <<EOF
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: ${policy_name}
  namespace: ${ns}
spec:
  targetResource:
    apiVersion: ${apiver}
    kind: ${kind}
    namespace: ${ns}
    labelSelector:
      matchLabels:
        app: test
        tier: ${ns_prefix}
  ttl:
    relativeTo: metadata.annotations.gc\.ops\.zen-mesh\.io/process-at
    secondsAfter: 5
EOF
)
            ;;

        *)
            FAIL "Unknown TTL mode: $ttl_mode"
            return
            ;;
    esac

    echo "$gcp_yaml" | kubectl apply -f -
    INFO "GCP ${policy_name} created"

    # Verify policy becomes Active
    sleep 2
    local phase
    phase="$(kubectl get gcp "$policy_name" -n "$ns" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
    INFO "Policy phase: $phase"

    # Wait for GC cycle
    INFO "Waiting for GC cycle (up to 60s)..."

    local deadline=$(( $(date +%s) + 60 + TTL_SHORT ))
    local matched=0 deleted=0
    while [[ $(date +%s) -lt $deadline ]]; do
        matched="$(kubectl get gcp "$policy_name" -n "$ns" -o jsonpath='{.status.resourcesMatched}' 2>/dev/null || echo "0")"
        deleted="$(kubectl get gcp "$policy_name" -n "$ns" -o jsonpath='{.status.resourcesDeleted}' 2>/dev/null || echo "0")"
        if [[ "$matched" -ge 1 && "$deleted" -ge 1 ]]; then
            break
        fi
        sleep 2
    done

    INFO "Final status: resourcesMatched=$matched  resourcesDeleted=$deleted"

    # Assertions
    if [[ "$matched" -ge 1 ]]; then
        PASS "$test_name: Resources matched ($matched)"
    else
        FAIL "$test_name: Resources matched (expected >=1, got $matched)"
    fi

    if [[ "$deleted" -ge 1 ]]; then
        PASS "$test_name: Resources deleted ($deleted)"
    else
        FAIL "$test_name: Resources deleted (expected >=1, got $deleted)"
    fi

    # Verify matching resource is GONE
    if ! kubectl get "$gvr_resource" "match-${ttl_mode}" -n "$ns" &>/dev/null; then
        PASS "$test_name: Matching resource 'match-${ttl_mode}' deleted"
    else
        FAIL "$test_name: Matching resource 'match-${ttl_mode}' still exists"
    fi

    # Verify control resource (wrong labels) still EXISTS
    if kubectl get "$gvr_resource" "control-${ttl_mode}" -n "$ns" &>/dev/null; then
        PASS "$test_name: Control resource (wrong labels) retained"
    else
        FAIL "$test_name: Control resource (wrong labels) incorrectly deleted"
    fi

    # Verify other-namespace control resource still EXISTS
    local ns_other="gc-${ns_prefix}-other"
    if kubectl get "$gvr_resource" "other-ns-${ttl_mode}" -n "$ns_other" &>/dev/null; then
        PASS "$test_name: Other-namespace control resource retained"
    else
        FAIL "$test_name: Other-namespace control resource incorrectly deleted"
    fi

    # Collect controller logs for this test
    kubectl logs -n zen-cleaner-system "$LEADER_POD" --tail=30 2>/dev/null > "${OUTPUT_DIR}/controller-logs-${ttl_mode}-${ns_prefix}.txt" || true

    # Cleanup policy (but keep resources for verification)
    kubectl delete gcp "$policy_name" -n "$ns" --timeout=10s 2>/dev/null || true
    INFO "Cleanup: GCP deleted"
}

########################################
# 5. Run validation matrix
########################################
declare -a RESULTS
results_json() {
    local f="$1"
    echo "{" > "$f"
    echo '  "clusterType": "'"$CLUSTER_TYPE"'",' >> "$f"
    echo '  "kubernetesVersion": "'"$K8S_VERSION"'",' >> "$f"
    echo '  "nodeImage": "'"$NODE_IMAGE"'",' >> "$f"
    echo '  "validationDate": "'"$(date -u +%Y-%m-%dT%H:%M:%SZ)"'",' >> "$f"
    echo '  "totalTests": '"$total"',' >> "$f"
    echo '  "passed": '"$((total - failures))"',' >> "$f"
    echo '  "failed": '"$failures"',' >> "$f"
    echo '  "tests": [' >> "$f"
    # Append test results
    local sep=""
    for r in "${RESULTS[@]}"; do
        echo "    ${sep}{\"name\":\"${r}\"}" >> "$f"
        sep=","
    done
    echo '  ]' >> "$f"
    echo "}" >> "$f"
}

STEP "Running validation matrix"

total=0

# Pod scenarios
for ttl_mode in fixed dynamic mapped relative; do
    [[ -n "$TTL_MODE_FILTER" && "$ttl_mode" != "$TTL_MODE_FILTER" ]] && continue
    [[ -n "$RESOURCE_KIND_FILTER" && "Pod" != "$RESOURCE_KIND_FILTER" ]] && continue
    total=$((total+1))
    run_ttl_scenario \
        "Pod/${ttl_mode}" \
        "v1" "Pod" "pod" \
        "$ttl_mode" "pod-${ttl_mode}"
    RESULTS+=("Pod/${ttl_mode}")
    echo ""
done

# ReplicaSet scenarios
for ttl_mode in fixed dynamic mapped relative; do
    [[ -n "$TTL_MODE_FILTER" && "$ttl_mode" != "$TTL_MODE_FILTER" ]] && continue
    [[ -n "$RESOURCE_KIND_FILTER" && "ReplicaSet" != "$RESOURCE_KIND_FILTER" ]] && continue
    total=$((total+1))
    run_ttl_scenario \
        "ReplicaSet/${ttl_mode}" \
        "apps/v1" "ReplicaSet" "replicaset" \
        "$ttl_mode" "rs-${ttl_mode}"
    RESULTS+=("ReplicaSet/${ttl_mode}")
    echo ""
done

########################################
# 6. Summary
########################################
STEP "SUMMARY"
INFO "Total: $total | Passed: $((total - failures)) | Failed: $failures"

results_json "${OUTPUT_DIR}/validation-results.json"

# Generate evidence manifest
EVIDENCE_MANIFEST="${EVIDENCE_DIR}/manifest.json"
cat > "$EVIDENCE_MANIFEST" <<MANIFESTEOF
{
  "run_id": "$RUN_ID",
  "date": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "git_commit": "$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || echo 'unknown')",
  "cluster_type": "$CLUSTER_TYPE",
  "cluster_name": "$CLUSTER_NAME",
  "kubernetes_version": "$K8S_VERSION",
  "node_image": "$NODE_IMAGE",
  "cleanup_interval": "$ZEN_CLEANER_INTERVAL",
  "ttl_short": $TTL_SHORT,
  "controller_image": "$TAG",
  "total_tests": $total,
  "passed": $((total - failures)),
  "failed": $failures,
  "blocked": $is_blocked,
  "result": "$( [[ $failures -eq 0 && $is_blocked -eq 0 ]] && echo 'PASS' || echo 'FAIL' )"
}
MANIFESTEOF
INFO "Evidence manifest: $EVIDENCE_MANIFEST"

# Generate markdown summary
MD_SUMMARY="${EVIDENCE_DIR}/summary.md"
{
    echo "# Validation Summary"
    echo ""
    echo "| Field | Value |"
    echo "|-------|-------|"
    echo "| Run ID | \`$RUN_ID\` |"
    echo "| Date | $(date -u +%Y-%m-%dT%H:%M:%SZ) |"
    echo "| Cluster type | $CLUSTER_TYPE |"
    echo "| K8s version | $K8S_VERSION |"
    echo "| Result | $( [[ $failures -eq 0 ]] && echo '✅ PASS' || echo '❌ FAIL' ) |"
    echo "| Total tests | $total |"
    echo "| Passed | $((total - failures)) |"
    echo "| Failed | $failures |"
    echo ""
    echo "## Test Results"
    echo ""
    echo "| Test | Result |"
    echo "|------|--------|"
    for r in "${RESULTS[@]}"; do
        echo "| $r | ✅ PASS |"
    done
    if [[ $failures -gt 0 ]]; then
        echo ""
        echo "⚠️  ${failures} test(s) failed."
    fi
    echo ""
    echo "---"
    echo "*Generated by validate-real-gc-deletion.sh*"
} > "$MD_SUMMARY"
INFO "Markdown summary: $MD_SUMMARY"

if [[ $failures -gt 0 ]]; then
    FAIL "Some tests failed ($failures)"
    exit 1
fi
if [[ $is_blocked -gt 0 ]]; then
    BLOCKED "Substrate blocked — cannot complete validation"
    exit 1
fi
PASS "All tests passed"
