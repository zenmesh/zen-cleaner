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
    h = dict(headers or {})
    if user and token:
        h["Authorization"] = "Basic " + base64.b64encode(f"{user}:{token}".encode()).decode()
    r = urllib.request.Request(url, data=data, method=method, headers=h)
    try:
        resp = urllib.request.urlopen(r)
        return resp.status, resp.headers, resp.read()
    except urllib.error.HTTPError as e:
        return e.code, e.headers, e.read()

def sha256(b): return "sha256:" + hashlib.sha256(b).hexdigest()

def ghcr_headers():
    # GHCR: exchange the Basic actor:token credential for a repository-scoped
    # bearer, then use it on every API call (Basic alone is rejected on blob
    # uploads with 401).
    if user and token:
        basic = "Basic " + base64.b64encode(f"{user}:{token}".encode()).decode()
        st, _, body = req(f"https://ghcr.io/token?scope=repository:{repo}:push,pull&service=ghcr.io",
                          headers={"Authorization": basic})
        if st == 200:
            tok = json.loads(body).get("token")
            if tok:
                return {"Authorization": f"Bearer {tok}"}
    return {}

hh = ghcr_headers()
content = open(file, "rb").read()
digest = sha256(content)
size = len(content)

uploads = []
for _ in range(2):
    st, headers, _ = req(f"{base}/blobs/uploads/", "POST", headers=hh)
    if st not in (200, 202):
        sys.exit(f"FATAL: blob upload POST {st}")
    loc = headers.get("Location") or headers.get("location")
    if not loc:
        sys.exit("FATAL: blob upload POST returned no Location")
    if loc.startswith("/"):
        loc = base + loc
    uploads.append(loc)

for loc in uploads:
    dl = loc + ("&" if "?" in loc else "?") + "digest=" + digest
    st, headers, _ = req(dl, "PUT", content, {"Content-Type": "application/octet-stream", "Content-Length": str(size)})
    if st not in (200, 201):
        sys.exit(f"FATAL: blob PUT {st}")

manifest = {
    "schemaVersion": 2,
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "config": {"mediaType": "application/vnd.cncf.artifacthub.config.v1+yaml", "digest": digest, "size": size},
    "layers": [{"mediaType": "application/yaml", "digest": digest, "size": size}],
}
mbytes = json.dumps(manifest).encode()
st, headers, body = req(f"{base}/manifests/artifacthub.io", "PUT", mbytes,
                        {"Content-Type": "application/vnd.oci.image.manifest.v1+json", "Content-Length": str(len(mbytes))})
if st not in (200, 201):
    sys.exit(f"FATAL: manifest PUT {st}: {body[:200]}")
print(f"PUSHED: {repo}:artifacthub.io (config {digest}, {size} bytes)")
