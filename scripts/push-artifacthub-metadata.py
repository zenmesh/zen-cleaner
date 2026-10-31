#!/usr/bin/env python3
"""Push the Artifact Hub repository-metadata OCI artifact (tag artifacthub.io).

Usage: push-artifacthub-metadata.py <registry/repo> <metadata-file> [user] [token]
Pure urllib; no HEAD-before-push (GHCR answers DENIED for nonexistent tags,
which breaks oras's existence check for the first metadata artifact)."""
import base64, hashlib, json, sys, urllib.request

repo, file = sys.argv[1], sys.argv[2]
user = sys.argv[3] if len(sys.argv) > 3 else None
token = sys.argv[4] if len(sys.argv) > 4 else None
proto = "http" if repo.startswith(("localhost:", "127.0.0.1:")) else "https"
base = f"{proto}://{repo.split('/')[0]}/v2/{repo.split('/', 1)[1]}"

def req(url, method="GET", data=None, headers=None):
    # Headers are passed through EXACTLY as given: the caller decides the
    # authorization (Basic for the token exchange, Bearer for registry
    # operations). Injecting Basic here overwrote the exchanged Bearer on
    # blob/manifest calls (the P0 defect).
    h = dict(headers or {})
    r = urllib.request.Request(url, data=data, method=method, headers=h)
    try:
        resp = urllib.request.urlopen(r)
        return resp.status, resp.headers, resp.read()
    except urllib.error.HTTPError as e:
        return e.code, e.headers, e.read()

def sha256(b): return "sha256:" + hashlib.sha256(b).hexdigest()

def ghcr_headers():
    # Exchange the Basic actor:token credential at the REGISTRY's own /token
    # endpoint (Docker registry token-auth spec; for ghcr.io this is
    # ghcr.io/token) and use the returned repository-scoped Bearer on every
    # API call. Basic alone is rejected on blob uploads with 401.
    if not (user and token):
        return {}
    host = repo.split("/")[0]
    # The scope's repository field is the repo PATH ONLY. Passing the full
    # <registry>/<namespace>/<name> minted a Bearer for a resource that does
    # not exist: GHCR tolerates it on the session POST but the finalizing
    # blob PUT answers 404 BLOB_UPLOAD_UNKNOWN — every chart job since 0.1.16
    # — while the identical shape with a host-free scope PUT 201 in the same
    # runner seconds later (dispatch probes 41144637095 / 41148521050).
    repo_path = repo.split("/", 1)[1]
    token_url = f"{proto}://{host}/token?scope=repository:{repo_path}:push,pull&service={host}"
    basic = "Basic " + base64.b64encode(f"{user}:{token}".encode()).decode()
    st, _, body = req(token_url, headers={"Authorization": basic})
    if st != 200:
        sys.exit(f"FATAL: registry token exchange failed ({st})")
    tok = json.loads(body).get("token")
    if not tok:
        sys.exit("FATAL: registry token exchange returned no token")
    return {"Authorization": f"Bearer {tok}"}

auth = ghcr_headers()
content = open(file, "rb").read()
digest = sha256(content)
size = len(content)

# ONE upload session, ONE finalizing PUT: the manifest below legally
# references the same blob for both config and layer (identical digests),
# so there is exactly one blob to upload. No second session, no retries.
st, headers, _ = req(f"{base}/blobs/uploads/", "POST", headers=dict(auth))
if st not in (200, 202):
    sys.exit(f"FATAL: blob upload POST {st}")
loc = headers.get("Location") or headers.get("location")
if not loc:
    sys.exit("FATAL: blob upload POST returned no Location")
if loc.startswith("/"):
    loc = base + loc
dl = loc + ("&" if "?" in loc else "?") + "digest=" + digest
put_headers = {"Content-Type": "application/octet-stream", "Content-Length": str(size)}
put_headers.update(auth)
st, headers, body = req(dl, "PUT", content, put_headers)
if st not in (200, 201):
    sys.exit(f"FATAL: blob PUT {st}: {body[:200]}")

manifest = {
    "schemaVersion": 2,
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "config": {"mediaType": "application/vnd.cncf.artifacthub.config.v1+yaml", "digest": digest, "size": size},
    "layers": [{"mediaType": "application/yaml", "digest": digest, "size": size}],
}
mbytes = json.dumps(manifest).encode()
put_headers = {"Content-Type": "application/vnd.oci.image.manifest.v1+json", "Content-Length": str(len(mbytes))}
put_headers.update(auth)
st, headers, body = req(f"{base}/manifests/artifacthub.io", "PUT", mbytes, put_headers)
if st not in (200, 201):
    sys.exit(f"FATAL: manifest PUT {st}: {body[:200]}")
print(f"PUSHED: {repo}:artifacthub.io (config {digest}, {size} bytes)")
