#!/usr/bin/env bash
# Copyright 2026 Zen Mesh
#
# End-to-end validation on a disposable kind cluster: CRDs, controller (image),
# validating webhook with locally generated TLS + caBundle, policy workloads,
# and optional Go e2e tests. Cluster is deleted on exit unless E2E_KEEP_CLUSTER=1.
#
# Requires: kind, kubectl, docker, openssl, go (for Go e2e tests).
#
# Usage:
#   ./scripts/comprehensive_e2e.sh
#   CLUSTER_NAME=my-e2e ./scripts/comprehensive_e2e.sh
#   E2E_SKIP_GO_TESTS=1 ./scripts/comprehensive_e2e.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GREEN='\033[0;32m' YELLOW='\033[1;33m' BLUE='\033[0;34m' RED='\033[0;31m' NC='\033[0m'
log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_step() { echo -e "${BLUE}[STEP]${NC} $1"; }
log_err() { echo -e "${RED}[ERROR]${NC} $1"; }

NAMESPACE="${NAMESPACE:-gc-e2e-test}"
CLUSTER_NAME="${CLUSTER_NAME:-zen-cleaner-e2e}"
IMAGE_TAG="${IMAGE_TAG:-zenmesh/zen-cleaner:e2e-${USER:-local}}"
E2E_KEEP_CLUSTER="${E2E_KEEP_CLUSTER:-}"
E2E_SKIP_GO_TESTS="${E2E_SKIP_GO_TESTS:-}"

KUBECONFIG_FILE=""
CERT_DIR=""

cleanup() {
	local rv=$?
	if [[ -z "${E2E_KEEP_CLUSTER:-}" ]]; then
		log_info "Deleting kind cluster ${CLUSTER_NAME}..."
		kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
	else
		log_warn "E2E_KEEP_CLUSTER set — leaving cluster ${CLUSTER_NAME} running."
	fi
	if [[ -n "${KUBECONFIG_FILE}" && -f "${KUBECONFIG_FILE}" ]]; then
		rm -f "${KUBECONFIG_FILE}"
	fi
	if [[ -n "${CERT_DIR}" && -d "${CERT_DIR}" ]]; then
		rm -rf "${CERT_DIR}"
	fi
	exit "${rv}"
}
trap cleanup EXIT

require_cmd() {
	if ! command -v "$1" &>/dev/null; then
		log_err "Missing required command: $1"
		exit 1
	fi
}

log_info "=== zen-cleaner comprehensive E2E (${ROOT}) ==="

require_cmd kind
require_cmd kubectl
require_cmd docker
require_cmd openssl
if [[ -z "${E2E_SKIP_GO_TESTS:-}" ]]; then
	require_cmd go
fi

log_step "Recreating kind cluster ${CLUSTER_NAME}..."
kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true

cat <<EOF | kind create cluster --name "$CLUSTER_NAME" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
EOF

KUBECONFIG_FILE="$(mktemp "${TMPDIR:-/tmp}/zen-cleaner-e2e-kubeconfig.XXXXXX")"
kind get kubeconfig --name "$CLUSTER_NAME" >"${KUBECONFIG_FILE}"
chmod 600 "${KUBECONFIG_FILE}"
export KUBECONFIG="${KUBECONFIG_FILE}"
log_info "KUBECONFIG=${KUBECONFIG}"

log_step "Installing CRDs..."
kubectl apply -f "${ROOT}/deploy/crds/"
kubectl wait --for=condition=Established --timeout=120s crd/zencleanerpolicies.cleaner.zen-mesh.io

log_step "Namespaces & RBAC..."
kubectl apply -f "${ROOT}/deploy/manifests/namespace.yaml"
kubectl apply -f "${ROOT}/deploy/manifests/rbac.yaml"

log_step "Webhook TLS (self-signed for ${CLUSTER_NAME})..."
CERT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/zen-cleaner-e2e-certs.XXXXXX")"
openssl genrsa -out "${CERT_DIR}/ca.key" 2048
openssl req -x509 -new -nodes -key "${CERT_DIR}/ca.key" -sha256 -days 2 \
	-out "${CERT_DIR}/ca.crt" -subj "/CN=gc-e2e-webhook-ca"
openssl genrsa -out "${CERT_DIR}/tls.key" 2048
openssl req -new -key "${CERT_DIR}/tls.key" -out "${CERT_DIR}/server.csr" \
	-subj "/CN=zen-cleaner-webhook.zen-cleaner-system.svc"
openssl x509 -req -in "${CERT_DIR}/server.csr" -CA "${CERT_DIR}/ca.crt" -CAkey "${CERT_DIR}/ca.key" \
	-CAcreateserial -out "${CERT_DIR}/tls.crt" -days 2 -sha256 \
	-extfile <(printf '%s\n' 'subjectAltName=DNS:zen-cleaner-webhook.zen-cleaner-system.svc,DNS:zen-cleaner-webhook.zen-cleaner-system.svc.cluster.local')

kubectl create secret tls zen-cleaner-webhook-cert -n zen-cleaner-system \
	--cert="${CERT_DIR}/tls.crt" --key="${CERT_DIR}/tls.key" \
	--dry-run=client -o yaml | kubectl apply -f -

CA_BUNDLE="$(base64 -w0 "${CERT_DIR}/ca.crt")"

log_step "Building and loading controller image ${IMAGE_TAG}..."
docker build -t "${IMAGE_TAG}" "${ROOT}"
kind load docker-image "${IMAGE_TAG}" --name "${CLUSTER_NAME}"

log_step "Deploying controller..."
kubectl apply -f "${ROOT}/deploy/manifests/deployment.yaml"
kubectl set image deployment/zen-cleaner zen-cleaner="${IMAGE_TAG}" -n zen-cleaner-system
kubectl scale deployment/zen-cleaner -n zen-cleaner-system --replicas=1
kubectl rollout status deployment/zen-cleaner -n zen-cleaner-system --timeout=180s

log_step "Webhook Service + ValidatingWebhookConfiguration (manual caBundle)..."
kubectl apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: zen-cleaner-webhook
  namespace: zen-cleaner-system
spec:
  ports:
    - port: 443
      targetPort: 9443
  selector:
    app: zen-cleaner
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: zen-cleaner-validating-webhook
webhooks:
  - name: validate-cleanup-policy.cleaner.zen-mesh.io
    clientConfig:
      caBundle: ${CA_BUNDLE}
      service:
        name: zen-cleaner-webhook
        namespace: zen-cleaner-system
        path: "/validate-cleanup-policy"
    rules:
      - apiGroups: ["cleaner.zen-mesh.io"]
        apiVersions: ["v1alpha1"]
        operations: ["CREATE", "UPDATE"]
        resources: ["zencleanerpolicies"]
    admissionReviewVersions: ["v1", "v1beta1"]
    sideEffects: None
    failurePolicy: Fail
EOF

kubectl create namespace "${NAMESPACE}" 2>/dev/null || true

log_step "Waiting for webhook readiness (bounded canary create/delete)..."
WEBHOOK_READY=0
for i in $(seq 1 30); do
	if kubectl apply -f - <<EOF 2>/dev/null
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: webhook-canary
  namespace: ${NAMESPACE}
spec:
  targetResource:
    apiVersion: v1
    kind: ConfigMap
  ttl:
    secondsAfterCreation: 3600
EOF
	then
		kubectl delete zencleanerpolicy webhook-canary -n "${NAMESPACE}" --wait=false 2>/dev/null || true
		WEBHOOK_READY=1
		log_info "Webhook ready after attempt ${i}"
		break
	fi
	sleep 2
done
if [[ "${WEBHOOK_READY}" != "1" ]]; then
	log_err "Webhook never became ready (30 attempts / ~60s)"
	exit 1
fi

# --- Assertions -------------------------------------------------------------

log_step "Test 1: valid ZenCleanerPolicy..."
kubectl apply -f - <<EOF
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: test-policy
  namespace: ${NAMESPACE}
spec:
  targetResource:
    apiVersion: v1
    kind: ConfigMap
  ttl:
    secondsAfterCreation: 3600
EOF
kubectl get gcp test-policy -n "${NAMESPACE}"

log_step "Test 2: CRD rejects invalid policy (empty targetResource)..."
set +e
out="$(kubectl apply -f - 2>&1 <<EOF
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: invalid-policy
  namespace: ${NAMESPACE}
spec:
  targetResource: {}
  ttl:
    secondsAfterCreation: 3600
EOF
)"
rc=$?
set -e
if [[ "${rc}" -eq 0 ]]; then
	log_err "Expected kubectl apply to fail for invalid CR (empty targetResource)"
	exit 1
fi
log_info "Rejected as expected: $(echo "${out}" | head -1)"

log_step "Test 3: Dry-run policy + ConfigMap retention..."
kubectl apply -f - <<EOF
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: dryrun-policy
  namespace: ${NAMESPACE}
spec:
  targetResource:
    apiVersion: v1
    kind: ConfigMap
  ttl:
    secondsAfterCreation: 10
  behavior:
    dryRun: true
EOF
kubectl create configmap test-dry -n "${NAMESPACE}" --from-literal=k=v
sleep 15
if ! kubectl get configmap test-dry -n "${NAMESPACE}" -o name &>/dev/null; then
	log_err "Dry-run: ConfigMap should still exist"
	exit 1
fi
log_info "Dry-run: ConfigMap still present (ok)"

log_step "Test 4: burst policy creates..."
for i in $(seq 1 15); do
	kubectl apply -f - <<EOF
apiVersion: cleaner.zen-mesh.io/v1alpha1
kind: ZenCleanerPolicy
metadata:
  name: rate-policy-${i}
  namespace: ${NAMESPACE}
spec:
  targetResource:
    apiVersion: v1
    kind: ConfigMap
  ttl:
    secondsAfterCreation: 3600
EOF
done
log_info "Created 15 policies in namespace ${NAMESPACE}"

if [[ -z "${E2E_SKIP_GO_TESTS:-}" ]]; then
	log_step "Go E2E tests (./test/e2e)..."
	(
		cd "${ROOT}"
		KUBECONFIG="${KUBECONFIG_FILE}" GOTOOLCHAIN=auto go test -tags=e2e -v -timeout=15m ./test/e2e/...
	)
fi

log_info "=== E2E validation succeeded ==="
