# Validating webhook TLS (production)

The controller serves an HTTPS validating webhook (`--webhook-addr`, default `:9443`). The Kubernetes API server only trusts that endpoint if:

1. **TLS** terminates inside the controller pod using a **certificate signed by a CA** the apiserver trusts.
2. **ValidatingWebhookConfiguration** includes that CA as **`clientConfig.caBundle`** (or uses **cert-manager** injection).

Manifests under `deploy/` assume a TLS Secret **`zen-cleaner-webhook-cert`** in **`zen-cleaner-system`** mounted at `/etc/webhook/certs`. The Service **`zen-cleaner-webhook`** exposes port **443** → pod **9443**.

## Recommended: cert-manager

For production clusters, use **[cert-manager](https://cert-manager.io/)** to issue and rotate certificates and to inject the CA into the webhook configuration.

1. Install cert-manager (follow upstream docs for your environment).
2. Create an **`Issuer`** / **`ClusterIssuer`** appropriate for your PKI (internal CA, ACME, etc.).
3. Model a **`Certificate`** resource that writes **`tls.crt`** / **`tls.key`** into Secret **`zen-cleaner-webhook-cert`** in **`zen-cleaner-system`** (see also `docs/SECRET_MANAGEMENT.md`).
4. Apply **`deploy/webhook/validating-webhook.yaml`**. The annotation  
   `cert-manager.io/inject-ca-from: zen-cleaner-system/zen-cleaner-webhook-cert`  
   lets cert-manager populate **`caBundle`** on the **`ValidatingWebhookConfiguration`**.

Keep **`failurePolicy: Fail`** only after you have verified webhook availability (misconfigured CA will block policy creates/updates).

## Alternative: manual CA (labs, air-gapped, bootstrap)

Useful when cert-manager is not installed:

1. Generate a CA key + certificate and a server certificate whose **SANs** include:
   - `zen-cleaner-webhook.zen-cleaner-system.svc`
   - `zen-cleaner-webhook.zen-cleaner-system.svc.cluster.local`
2. Create the Kubernetes TLS Secret:

   ```bash
   kubectl create secret tls zen-cleaner-webhook-cert -n zen-cleaner-system \
     --cert=tls.crt --key=tls.key
   ```

3. Base64-encode the **CA certificate** (PEM) **without** headers line breaks for **`caBundle`**:

   ```bash
   CA_BUNDLE=$(base64 -w0 ca.crt)
   ```

4. Apply a **`ValidatingWebhookConfiguration`** whose **`clientConfig.caBundle`** is set to that value (same **`service`**, **`namespace`**, and **`path: /validate-cleanup-policy`** as in `deploy/webhook/validating-webhook.yaml`). Remove reliance on cert-manager annotations if you hand-edit the webhook YAML.

The **`scripts/comprehensive_e2e.sh`** flow builds this path on **kind**: self-signed CA + Secret + webhook with explicit **`caBundle`**, proving TLS end-to-end without cert-manager.

## Operations checklist

- [ ] Secret **`zen-cleaner-webhook-cert`** present and mounted in the Deployment.
- [ ] Service **`zen-cleaner-webhook`** selects the controller pods and maps **443 → 9443**.
- [ ] **`ValidatingWebhookConfiguration`** **`caBundle`** matches the CA that signed the serving cert.
- [ ] Optional: network policies allow apiserver → Service port **443** (control-plane dependent).

For secret handling patterns (rotation, CSI), see **`docs/SECRET_MANAGEMENT.md`**.
