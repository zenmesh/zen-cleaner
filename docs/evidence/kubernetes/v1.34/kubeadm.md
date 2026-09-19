# kubeadm — Validation Evidence (K8s v1.34.9)

## Status: PASS (Full Runtime + Cleanup)

zen-cleaner CRD (`ZenCleanerPolicy`) has been fully validated against Kubernetes
v1.34.9 provisioned via kubeadm on a Debian 13 VM, including controller runtime
reconciliation and cleanup deletion behavior.

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
| **RAM** | 12 GB |
| **vCPUs** | 4 |
| **Containerd** | 2.2.5 (upgraded from Debian's 1.7.24 via Docker apt repo) |
| **CNI** | Flannel v0.28.5 |
| **Kubeadm/Kubelet/Kubectl** | v1.34.9 |
| **Controller image** | Static CGO_ENABLED=0 build, scratch base |

## Validation Results

### CRD Registration & API Discovery
```
$ kubectl get crds zencleanerpolicies.cleaner.zen-mesh.io
NAME                                           CREATED AT
zencleanerpolicies.cleaner.zen-mesh.io   2026-07-01T18:04:40Z

$ kubectl api-resources --api-group=cleaner.zen-mesh.io
NAME                        SHORTNAMES     APIVERSION                    NAMESPACED   KIND
zencleanerpolicies   zcp,zencleanerpolicy   cleaner.zen-mesh.io/v1alpha1   true         ZenCleanerPolicy
```

### CRUD Lifecycle
| Operation | Result |
|-----------|--------|
| Create minimal GCP | ✅ |
| Create full-schema cleanupP | ✅ |
| List GCPs | ✅ |
| Read GCP YAML | ✅ |
| Re-apply CRD (idempotent) | ✅ |
| Delete GCP | ✅ |

### Negative Schema Validation
| Test | Result |
|------|--------|
| Wrong type (string for integer) | ✅ Rejected |
| Unknown field | ✅ Rejected (strict decoding) |
| Empty spec | ✅ Rejected (required fields) |
| Missing required `targetResource` | ✅ Rejected |

### Controller Deployment
```
$ kubectl get deployment zen-cleaner
NAME            READY   UP-TO-DATE   AVAILABLE   AGE
zen-cleaner   2/2     2            2           3m

$ kubectl logs zen-cleaner-... | tail
... "Starting workers" worker count=1
... "Deleted resource disposable-pod (reason: ttl_expired)"
... "Evaluated policy: matched=1, deleted=1, pending=0"
```

### Cleanup Behavior
- **GCP**: `disposable-pod-cleanup` — matches pods with label `gc-disposable=true`, TTL 10s
- **Disposable pod** (`gc-disposable=true`): detected and **deleted** within 1 cleanup interval
- **Control pod** (`gc-control=true`): **not matched** — remains running
- **After cleanup**: second cycle confirmed `matched=0, deleted=0, pending=0`

### Control-Plane Stability
All CP components running with **0 restarts** (frozen since init), 9+ min uptime.

```
NAMESPACE      NAME                               READY   STATUS    RESTARTS   AGE
kube-system    etcd-debian13                      1/1     Running   0          9m
kube-system    kube-apiserver-debian13            1/1     Running   0          9m
kube-system    kube-controller-manager-debian13   1/1     Running   0          9m
kube-system    kube-scheduler-debian13            1/1     Running   0          9m
kube-system    coredns-66bc5c9577-9bs8p           1/1     Running   0          9m
```

## Known Issues
- Events RBAC missing — controller cannot create/patch events. cleanup operations complete successfully regardless.

## Limitations
- Single-node control-plane only (no HA)
- Validated with containerd 2.2.5; Debian default containerd 1.7.24 is not claimed for this workload
- flannel CNI only
- Webhook running in insecure mode (no TLS certs)
- No cloud Kubernetes testing
