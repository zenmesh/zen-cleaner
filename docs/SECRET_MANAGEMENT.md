# Secret Management Guide

This document describes how to securely manage sensitive configuration and secrets for zen-cleaner.

For **production webhook TLS** end-to-end (cert-manager vs manual CA, `caBundle`, Service wiring), read **[WEBHOOK_TLS.md](WEBHOOK_TLS.md)** first.

## Overview

zen-cleaner requires sensitive configuration for:
- **Webhook TLS Certificates**: TLS certificates and private keys for the validating webhook server
- **Kubernetes Service Account Tokens**: Automatically managed by Kubernetes
- **Future**: API keys, external service credentials (if needed)

## Webhook TLS Certificates

The validating webhook server requires TLS certificates for secure communication with the Kubernetes API server.

### Option 1: Kubernetes Secrets (Recommended)

#### Using kubectl

Create a Kubernetes Secret with TLS certificates:

```bash
# Generate self-signed certificate (for testing only)
openssl req -x509 -newkey rsa:2048 -keyout tls.key -out tls.crt -days 365 -nodes \
  -subj "/CN=zen-cleaner-webhook.zen-cleaner-system.svc"

# Create secret
kubectl create secret tls zen-cleaner-webhook-cert \
  --cert=tls.crt \
  --key=tls.key \
  -n zen-cleaner-system
```

#### Using YAML Manifest

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: zen-cleaner-webhook-cert
  namespace: zen-cleaner-system
type: kubernetes.io/tls
data:
  tls.crt: <base64-encoded-certificate>
  tls.key: <base64-encoded-private-key>
```

**To encode files:**
```bash
cat tls.crt | base64 -w 0
cat tls.key | base64 -w 0
```

#### Mount Secret in Deployment

The deployment should mount the secret:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: zen-cleaner
  namespace: zen-cleaner-system
spec:
  template:
    spec:
      containers:
      - name: zen-cleaner
        volumeMounts:
        - name: webhook-certs
          mountPath: /etc/webhook/certs
          readOnly: true
      volumes:
      - name: webhook-certs
        secret:
          secretName: zen-cleaner-webhook-cert
```

### Option 2: cert-manager (Production Recommended)

cert-manager automatically manages TLS certificates, including automatic renewal.

#### Install cert-manager

```bash
# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Verify installation
kubectl get pods -n cert-manager
```

#### Create Certificate Issuer

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: nginx
```

#### Create Certificate Resource

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: zen-cleaner-webhook-cert
  namespace: zen-cleaner-system
spec:
  secretName: zen-cleaner-webhook-cert
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
  - zen-cleaner-webhook.zen-cleaner-system.svc
  - zen-cleaner-webhook.zen-cleaner-system.svc.cluster.local
```

#### Configure ValidatingWebhookConfiguration

The webhook configuration should reference cert-manager:

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: zen-cleaner-validating-webhook
  annotations:
    cert-manager.io/inject-ca-from: zen-cleaner-system/zen-cleaner-webhook-cert
spec:
  webhooks:
  - name: validate-cleanup-policy.cleaner.zen-mesh.io
    clientConfig:
      service:
        name: zen-cleaner-webhook
        namespace: zen-cleaner-system
        path: "/validate-cleanup-policy"
    # ... rest of configuration
```

cert-manager will:
- Automatically inject the CA certificate into the webhook configuration
- Renew certificates before expiration
- Update the secret automatically

### Option 3: Self-Signed Certificates (Development Only)

For local development or testing:

```bash
# Generate self-signed certificate
openssl req -x509 -newkey rsa:2048 -keyout tls.key -out tls.crt -days 365 -nodes \
  -subj "/CN=zen-cleaner-webhook.zen-cleaner-system.svc" \
  -addext "subjectAltName=DNS:zen-cleaner-webhook.zen-cleaner-system.svc,DNS:zen-cleaner-webhook.zen-cleaner-system.svc.cluster.local"

# Create secret
kubectl create secret tls zen-cleaner-webhook-cert \
  --cert=tls.crt \
  --key=tls.key \
  -n zen-cleaner-system
```

**⚠️ Warning**: Self-signed certificates should **never** be used in production.

## Secret Rotation

### Manual Rotation

#### Step 1: Generate New Certificate

```bash
# Generate new certificate
openssl req -x509 -newkey rsa:2048 -keyout tls-new.key -out tls-new.crt -days 365 -nodes \
  -subj "/CN=zen-cleaner-webhook.zen-cleaner-system.svc"
```

#### Step 2: Update Secret

```bash
# Update secret with new certificate
kubectl create secret tls zen-cleaner-webhook-cert \
  --cert=tls-new.crt \
  --key=tls-new.key \
  -n zen-cleaner-system \
  --dry-run=client -o yaml | kubectl apply -f -
```

#### Step 3: Restart Pods

```bash
# Restart pods to pick up new certificate
kubectl rollout restart deployment/zen-cleaner -n zen-cleaner-system

# Verify pods are running
kubectl get pods -n zen-cleaner-system -l app=zen-cleaner
```

### Automatic Rotation (cert-manager)

With cert-manager, certificates are automatically renewed before expiration. No manual intervention is required.

**Monitor certificate expiration:**
```bash
# Check certificate expiration
kubectl get certificate zen-cleaner-webhook-cert -n zen-cleaner-system -o yaml

# Check cert-manager logs
kubectl logs -n cert-manager -l app.kubernetes.io/instance=cert-manager
```

## External Secret Managers

For advanced secret management, consider integrating with external secret managers:

### HashiCorp Vault

#### Install Vault CSI Driver

```bash
# Install Vault CSI driver
kubectl apply -f https://raw.githubusercontent.com/hashicorp/vault-csi-provider/main/deployment/install.yaml
```

#### Create SecretProviderClass

```yaml
apiVersion: secrets-store.csi.x-k8s.io/v1
kind: SecretProviderClass
metadata:
  name: zen-cleaner-webhook-certs
  namespace: zen-cleaner-system
spec:
  provider: vault
  parameters:
    vaultAddress: "https://vault.example.com:8200"
    roleName: "zen-cleaner"
    objects: |
      - objectName: "tls.crt"
        secretPath: "secret/data/zen-cleaner/webhook"
        secretKey: "cert"
      - objectName: "tls.key"
        secretPath: "secret/data/zen-cleaner/webhook"
        secretKey: "key"
  secretObjects:
  - secretName: zen-cleaner-webhook-cert
    type: kubernetes.io/tls
    data:
    - objectName: tls.crt
      key: tls.crt
    - objectName: tls.key
      key: tls.key
```

#### Mount in Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: zen-cleaner
spec:
  template:
    spec:
      containers:
      - name: zen-cleaner
        volumeMounts:
        - name: webhook-certs
          mountPath: /etc/webhook/certs
          readOnly: true
      volumes:
      - name: webhook-certs
        csi:
          driver: secrets-store.csi.k8s.io
          readOnly: true
          volumeAttributes:
            secretProviderClass: zen-cleaner-webhook-certs
```

### AWS Secrets Manager

Use [External Secrets Operator](https://external-secrets.io/) to sync secrets from AWS Secrets Manager:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: zen-cleaner-webhook-cert
  namespace: zen-cleaner-system
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secrets-manager
    kind: SecretStore
  target:
    name: zen-cleaner-webhook-cert
    creationPolicy: Owner
  data:
  - secretKey: tls.crt
    remoteRef:
      key: zen-cleaner/webhook
      property: cert
  - secretKey: tls.key
    remoteRef:
      key: zen-cleaner/webhook
      property: key
```

### Google Secret Manager

Similar to AWS, use External Secrets Operator:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: zen-cleaner-webhook-cert
  namespace: zen-cleaner-system
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: gcp-secret-manager
    kind: SecretStore
  target:
    name: zen-cleaner-webhook-cert
  data:
  - secretKey: tls.crt
    remoteRef:
      key: zen-cleaner-webhook-cert
      property: cert
  - secretKey: tls.key
    remoteRef:
      key: zen-cleaner-webhook-cert
      property: key
```

## Security Best Practices

### 1. Use Kubernetes Secrets

- ✅ Store secrets in Kubernetes Secrets, not in ConfigMaps
- ✅ Use `type: kubernetes.io/tls` for TLS certificates
- ✅ Mount secrets as read-only volumes
- ✅ Never commit secrets to version control

### 2. Restrict Access

```yaml
# Use RBAC to restrict who can access secrets
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: secret-reader
  namespace: zen-cleaner-system
rules:
- apiGroups: [""]
  resources: ["secrets"]
  resourceNames: ["zen-cleaner-webhook-cert"]
  verbs: ["get"]
```

### 3. Encrypt Secrets at Rest

Enable encryption at rest for etcd:

```yaml
# In kube-apiserver configuration
--encryption-provider-config=/etc/kubernetes/encryption-config.yaml
```

### 4. Use cert-manager for Production

- ✅ Automatic certificate renewal
- ✅ Integration with Let's Encrypt
- ✅ No manual certificate management
- ✅ Automatic CA injection

### 5. Monitor Certificate Expiration

Set up alerts for certificate expiration:

```yaml
# Prometheus alert rule
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: zen-cleaner-cert-expiry
spec:
  groups:
  - name: zen-cleaner
    rules:
    - alert: WebhookCertificateExpiringSoon
      expr: cert_manager_certificate_expiration_timestamp_seconds{name="zen-cleaner-webhook-cert"} - time() < 86400 * 7
      for: 1h
      annotations:
        summary: "Webhook certificate expiring in 7 days"
```

### 6. Rotate Secrets Regularly

- **TLS Certificates**: Rotate before expiration (cert-manager handles this automatically)
- **Service Account Tokens**: Kubernetes rotates these automatically
- **API Keys**: Rotate every 90 days or as per your security policy

### 7. Use Separate Secrets per Environment

- **Development**: Use self-signed certificates
- **Staging**: Use cert-manager with staging issuer
- **Production**: Use cert-manager with production issuer

### 8. Audit Secret Access

Enable Kubernetes audit logging:

```yaml
# Audit policy
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
- level: Metadata
  resources:
  - group: ""
    resources: ["secrets"]
```

## Troubleshooting

### Issue: Webhook Certificate Errors

**Symptoms:**
- Webhook requests failing with certificate errors
- Logs show "x509: certificate signed by unknown authority"

**Solutions:**
```bash
# Check certificate in secret
kubectl get secret zen-cleaner-webhook-cert -n zen-cleaner-system -o yaml

# Verify certificate
kubectl get secret zen-cleaner-webhook-cert -n zen-cleaner-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -text -noout

# Check webhook configuration CA bundle
kubectl get validatingwebhookconfiguration zen-cleaner-validating-webhook -o yaml

# Restart pods
kubectl rollout restart deployment/zen-cleaner -n zen-cleaner-system
```

### Issue: cert-manager Not Renewing Certificates

**Symptoms:**
- Certificates expiring soon
- cert-manager logs show errors

**Solutions:**
```bash
# Check certificate status
kubectl describe certificate zen-cleaner-webhook-cert -n zen-cleaner-system

# Check cert-manager logs
kubectl logs -n cert-manager -l app.kubernetes.io/instance=cert-manager

# Check issuer status
kubectl describe clusterissuer letsencrypt-prod
```

### Issue: Secret Not Found

**Symptoms:**
- Pods failing to start
- Volume mount errors

**Solutions:**
```bash
# Verify secret exists
kubectl get secret zen-cleaner-webhook-cert -n zen-cleaner-system

# Check deployment volume configuration
kubectl get deployment zen-cleaner -n zen-cleaner-system -o yaml | grep -A 10 volumes

# Verify secret is in correct namespace
kubectl get secrets -n zen-cleaner-system | grep webhook
```

## Examples

### Complete Deployment with cert-manager

See `examples/webhook-with-cert-manager.yaml` for a complete example.

### Complete Deployment with Manual Secrets

See `examples/webhook-with-manual-secrets.yaml` for a complete example.

## References

- [Kubernetes Secrets Documentation](https://kubernetes.io/docs/concepts/configuration/secret/)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [External Secrets Operator](https://external-secrets.io/)
- [HashiCorp Vault CSI Driver](https://developer.hashicorp.com/vault/docs/platform/k8s/csi)
- [Kubernetes Encryption at Rest](https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/)

