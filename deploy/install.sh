#!/usr/bin/env bash
# Canonical zen-cleaner install (SUPPORT2-032R §4; mirrors the zen-gc
# S025-GC §2 canonical installer).
# Required env:
#   CLEANER_IMAGE        immutable image ref (refuses :latest)
#   WEBHOOK_NAMESPACE    namespace (default zen-cleaner-system)
#
# The pre-existing root ./install.sh targets released charts; this script is
# the cluster-install source of truth and closes the prior packaging gap:
# without it nothing provisioned the zen-cleaner-webhook-cert Secret the
# Deployment mounts, so pods could never start from a clean install.
set -euo pipefail
cd "$(dirname "$0")/.."
NS="${WEBHOOK_NAMESPACE:-zen-cleaner-system}"
export WEBHOOK_NAMESPACE="$NS"
fail(){ echo "FAIL: $*" >&2; exit 1; }
command -v kubectl >/dev/null || fail "kubectl required"
command -v openssl >/dev/null || fail "openssl required (webhook cert generation)"
command -v envsubst >/dev/null || fail "envsubst required (manifest rendering)"
[ -n "${CLEANER_IMAGE:-}" ] || fail "CLEANER_IMAGE required (immutable tag)"
case "$CLEANER_IMAGE" in *:latest|*:latest@*) fail "refusing :latest";; *:*) ;; *) fail "CLEANER_IMAGE must carry explicit tag"; esac
if grep -rq "REPLACE_WITH\|SET_BY_INSTALLER" deploy/manifests/deployment.yaml; then fail "deployment.yaml still contains placeholders"; fi

kubectl create ns "$NS" --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f deploy/crds/

bash deploy/gen-webhook-cert.sh

# The generated serving cert doubles as the admission CA bundle (base64 PEM
# is exactly what the secret's tls.crt data field holds).
WEBHOOK_CA_BUNDLE="$(kubectl -n "$NS" get secret zen-cleaner-webhook-cert -o jsonpath='{.data.tls\.crt}')"
export WEBHOOK_CA_BUNDLE

TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
for src in deploy/manifests/rbac.yaml deploy/manifests/service.yaml deploy/manifests/pdb.yaml deploy/manifests/deployment.yaml deploy/webhook/mutating-webhook.yaml deploy/webhook/validating-webhook.yaml; do
  envsubst < "$src" > "$TMP/$(basename "$src")"
done
if grep -rI '\${' "$TMP"; then fail "unsubstituted placeholder in rendered manifests"; fi

kubectl apply -n "$NS" -f "$TMP/rbac.yaml"
kubectl apply -n "$NS" -f "$TMP/service.yaml"
kubectl apply -n "$NS" -f "$TMP/deployment.yaml"
kubectl apply -n "$NS" -f "$TMP/pdb.yaml"
kubectl apply -f "$TMP/mutating-webhook.yaml" -f "$TMP/validating-webhook.yaml"

echo "installed zen-cleaner; verify: kubectl -n $NS rollout status deployment/zen-cleaner"
