#!/usr/bin/env bash
# Deterministic self-contained webhook TLS bootstrap for zen-cleaner
# (SUPPORT2-032R §4; mirrors the zen-gc S025-GC §1 design).
# Generates a self-signed serving cert bound to the in-cluster webhook
# service DNS names, installs it as the zen-cleaner-webhook-cert Secret,
# and patches the CA bundle into the validating/mutating webhook configs.
# No cert-manager prerequisite; renewal = re-run this script + restart pods.
set -euo pipefail
NS="${WEBHOOK_NAMESPACE:-zen-cleaner-system}"
SVC="zen-cleaner-webhook"
SECRET="zen-cleaner-webhook-cert"
TMP=$(mktemp -d); trap 'rm -rf "$TMP"' EXIT
openssl req -x509 -newkey rsa:2048 -days 365 -nodes \
  -keyout "$TMP/tls.key" -out "$TMP/tls.crt" \
  -subj "/CN=${SVC}.${NS}.svc" \
  -addext "subjectAltName=DNS:${SVC},DNS:${SVC}.${NS},DNS:${SVC}.${NS}.svc" >/dev/null 2>&1
kubectl -n "$NS" create secret generic "$SECRET" \
  --from-file=tls.crt="$TMP/tls.crt" --from-file=tls.key="$TMP/tls.key" \
  --dry-run=client -o yaml | kubectl apply -f -
CA_BUNDLE=$(base64 -w0 "$TMP/tls.crt")
for WH in deploy/webhook/mutating-webhook.yaml deploy/webhook/validating-webhook.yaml; do
  [ -f "$WH" ] && sed -i "s|caBundle:.*|caBundle: ${CA_BUNDLE}|" "$WH"
done
echo "webhook cert secret ${SECRET} installed; CA bundle patched into webhook yaml (re-apply deploy/webhook/*.yaml to activate)"
