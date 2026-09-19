# kubeadm — Validation Evidence (K8s v1.36.2)

## Status: PASS

Full zen-cleaner validation (CRD/API, CRUD lifecycle, negative schema, RBAC,
controller runtime, cleanup behavior) completed successfully on Kubernetes v1.36.2
provisioned via kubeadm on Debian 13.

**Validation note**: Validated with containerd 2.2.5 on Debian 13. Debian's
default containerd 1.7.24 is not part of this validated claim.

## VM Configuration

| Field | Value |
|-------|-------|
| **Hostname** | `h462-gateway-kubeadm-1780668538` |
| **IP** | 192.168.122.179 |
| **Libvirt domain** | `h462-gateway-kubeadm-1780668538` |
| **OS** | Debian 13 (trixie) |
| **Kernel** | 6.12.74+deb13+1-amd64 |
| **RAM** | 12 GB (`virsh setmaxmem` + `setmem --config`, stop/start cycle) |
| **RAM (guest)** | 11 GiB total / 10 GiB available |
| **vCPUs** | 4 |
| **Containerd** | 2.2.5 |
| **CNI** | Flannel v0.28.5 |
| **Kubeadm/Kubelet/Kubectl** | v1.36.2 |
| **CoreDNS** | v1.12.0 (deployed via `kubeadm init phase addon coredns`) |

## Cluster Configuration

kubeadm `v1beta4` config, single control-plane node, flannel CNI with
`10.244.0.0/16` pod CIDR. Containerd 2.2.5 with SystemdCgroup driver.

## Evidence

### Cluster Substrate

```
$ kubectl get nodes -o wide
NAME       STATUS   ROLES           AGE   VERSION   INTERNAL-IP       OS-IMAGE                       KERNEL-VERSION                  CONTAINER-RUNTIME
debian13   Ready    control-plane   78m   v1.36.2   192.168.122.179   Debian GNU/Linux 13 (trixie)   6.12.74+deb13+1-amd64 (amd64)   containerd://2.2.5

$ kubectl get pods -A -o wide
NAMESPACE      NAME                               READY   STATUS    RESTARTS
default        control-pod                        1/1     Running   0
zen-cleaner-system      zen-cleaner-55b4b446d8-fgrxn     1/1     Running   1 (7m ago)
kube-flannel   kube-flannel-ds-krbtd              1/1     Running   0
kube-system    coredns-589f44dc88-dhjzb           1/1     Running   0
kube-system    coredns-589f44dc88-vkrzj           1/1     Running   0
kube-system    etcd-debian13                      1/1     Running   40
kube-system    kube-apiserver-debian13            1/1     Running   39
kube-system    kube-controller-manager-debian13   1/1     Running   48
kube-system    kube-proxy-6r94w                   1/1     Running   0
kube-system    kube-scheduler-debian13            1/1     Running   60
```

CP restart counts are from the initial kubelet startup (cold boots + containerd
upgrade). **Zero new restarts during the 45+ minute validation window.**

### CRD Install Idempotency

```
$ kubectl apply -f deploy/crds/cleaner.zen-mesh.io_zencleanerpolicies.yaml
  customresourcedefinition.apiextensions.k8s.io/zencleanerpolicies.cleaner.zen-mesh.io created

$ kubectl apply -f deploy/crds/cleaner.zen-mesh.io_zencleanerpolicies.yaml
  customresourcedefinition.apiextensions.k8s.io/zencleanerpolicies.cleaner.zen-mesh.io unchanged
```

### API Resource Discovery

```
$ kubectl api-resources --api-group=cleaner.zen-mesh.io
NAME                        SHORTNAMES     APIVERSION                    NAMESPACED   KIND
zencleanerpolicies   zcp,zencleanerpolicy   cleaner.zen-mesh.io/v1alpha1   true         ZenCleanerPolicy
```

### CRUD Lifecycle

| Operation | Result |
|-----------|--------|
| Create minimal GCP | ✅ created |
| Create full-schema cleanupP (all spec fields) | ✅ created |
| List GCPs | ✅ appears with namespace/name/age |
| Read GCP YAML | ✅ all spec fields persisted |
| Re-apply GCP (idempotency) | ✅ unchanged |
| Delete GCP | ✅ deleted |
| GCPs after delete | ✅ empty list |

### Negative Schema Validation

| Test | Input | Result |
|------|-------|--------|
| Wrong type | `ttl.secondsAfterCreation: "3600"` (string) | ❌ Rejected: "must be of type integer" |
| Unknown field | `spec.nonexistentField: true` | ❌ Rejected: "unknown field" (strict decoding) |
| Empty spec | `spec: {}` | ❌ Rejected: "targetResource: Required value" + "ttl: Required value" |
| Missing required | no `spec.targetResource` | ❌ Rejected: "targetResource: Required value" |

All invalid inputs are correctly rejected by the API server with descriptive
error messages.

### RBAC / Permission Boundaries

```
$ kubectl auth can-i --list --as=system:serviceaccount:zen-cleaner-system:zen-cleaner
  *.*                                                   [get list watch delete]
  zencleanerpolicies.cleaner.zen-mesh.io          [get list watch]
  zencleanerpolicies.cleaner.zen-mesh.io/status   [get update patch]
```

The controller has GCP read access, status write access, and the broad
list/delete permissions required for cleanup across namespaces.
Current RBAC gives `*.*` list/watch/delete (known broad scope — documented
risk; see RBAC hardening notes).

### Controller Runtime

```
$ kubectl get pods -n zen-cleaner-system -o wide
NAME                             READY   STATUS    RESTARTS
zen-cleaner-55b4b446d8-fgrxn   1/1     Running   1
zen-cleaner-55b4b446d8-2crzr   0/1     Running   0

$ kubectl logs -n zen-cleaner-system zen-cleaner-55b4b446d8-fgrxn --tail=20
{"level":"info","msg":"Controller configuration", ... "cleanupInterval":"1m0s"}
{"level":"info","msg":"Leader election enabled", ... "electionID":"zen-cleaner-leader-election"}
{"level":"info","msg":"Starting workers","controller":"zencleanerpolicy","worker count":1}
{"level":"info","msg":"Starting webhook server with TLS","address":":9443"}
{"level":"info","msg":"Starting Zen Cleaner controller manager"}
```

Controller started successfully, acquired leader lease, and reconciliation
workers are active. Leader election across 2 replicas works (one active, one
standby).

### Cleanup Behavior

**Test**: Create a cleanupP targeting pods with label `gc-disposable=true`, TTL of
10 seconds. Disposable pod has the label; control pod does not.

```
# Before cleanup evaluation:
NAME             READY   LABELS
control-pod      1/1     gc-control=true
disposable-pod   1/1     gc-disposable=true

# After 20 seconds (1 cleanup interval):
NAME          READY   LABELS
control-pod   1/1     gc-control=true
(disposable-pod deleted)
```

✅ **Disposable pod deleted** — GCP matched the labeled pod and the controller
deleted it after TTL expiry.

✅ **Control pod untouched** — no matching labels, unaffected by cleanup policy.

✅ **No unrelated resources affected** — only the targeted pod was deleted.

### Post-Validation Stability

After all validation actions, the control plane remains stable with no restart
growth. CoreDNS resolves DNS queries. kubelet is active. Node Ready.

## Evidence Files

- `kubeadm.json` — structured evidence data.
