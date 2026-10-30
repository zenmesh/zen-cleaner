#!/usr/bin/env python3
"""Probe GHCR blob-upload session semantics for the AH metadata push.

Runs the same token exchange as push-artifacthub-metadata.py, then exercises
three upload shapes against the REAL registry and prints only non-secret
observations: statuses, Location scheme/host/path, query PARAMETER NAMES
(values redacted), and registry error bodies. Never prints the token, the
Authorization header, or Location query values.

Shapes:
  A) POST -> PUT(body, digest)                    [monolithic, spec §2.2]
  B) POST(Content-Length: 0) -> PUT(body, digest) [explicit empty CL]
  C) POST -> PATCH(body) -> PUT(digest only)      [oras/docker chunked]

Usage: debug-ghcr-upload-session.py <registry/repo> <metadata-file> <user> <token>
"""
import base64, hashlib, json, sys, urllib.request, urllib.error
from urllib.parse import urlsplit

repo, file = sys.argv[1], sys.argv[2]
user, token = sys.argv[3], sys.argv[4]
host = repo.split("/")[0]
base = f"https://{host}/v2/{repo.split('/', 1)[1]}"

def redact_loc(loc):
    if not loc:
        return "(none)"
    sp = urlsplit(loc)
    keys = [kv.split("=", 1)[0] for kv in sp.query.split("&") if kv]
    q = ("?" + "&".join(k + "=<redacted>" for k in keys)) if keys else ""
    return f"{sp.scheme}://{sp.netloc}{sp.path}{q}"

def call(url, method, data=None, headers=None):
    r = urllib.request.Request(url, data=data, method=method,
                               headers=dict(headers or {}))
    try:
        resp = urllib.request.urlopen(r)
        return resp.status, dict(resp.headers), resp.read()
    except urllib.error.HTTPError as e:
        return e.code, dict(e.headers), e.read()

basic = "Basic " + base64.b64encode(f"{user}:{token}".encode()).decode()
st, _, body = call(f"https://{host}/token?scope=repository:{repo}:push,pull"
                   f"&service={host}", headers={"Authorization": basic})
print(f"token exchange: {st}")
if st != 200:
    sys.exit(1)
auth = {"Authorization": "Bearer " + json.loads(body)["token"]}

content = open(file, "rb").read()
digest = "sha256:" + hashlib.sha256(content).hexdigest()
print(f"content: {len(content)} bytes, digest {digest}")

def new_session(extra=None):
    h = dict(auth)
    h.update(extra or {})
    st, hh, _ = call(f"{base}/blobs/uploads/", "POST", headers=h)
    loc = hh.get("Location") or hh.get("location") or ""
    print(f"  POST: {st} | Location: {redact_loc(loc)}")
    return st, loc

def finalize(loc, data, digest_param=True):
    if loc.startswith("/"):
        loc = f"https://{host}" + loc
    url = loc + (("&" if "?" in loc else "?") + "digest=" + digest) \
        if digest_param else loc
    st, _, body = call(url, "PUT", data,
                       {**auth, "Content-Type": "application/octet-stream"})
    print(f"  PUT : {st} | {body[:200]!r}")
    return st

print("shape A: POST -> PUT(body, digest)")
st, loc = new_session()
if st in (200, 202) and loc and finalize(loc, content) in (200, 201):
    print("RESULT: shape A works"); sys.exit(0)

print("shape B: POST(Content-Length: 0) -> PUT(body, digest)")
st, loc = new_session({"Content-Length": "0"})
if st in (200, 202) and loc and finalize(loc, content) in (200, 201):
    print("RESULT: shape B works"); sys.exit(0)

print("shape C: POST -> PATCH(body) -> PUT(digest only)")
st, loc = new_session()
if st in (200, 202) and loc:
    if loc.startswith("/"):
        loc = f"https://{host}" + loc
    st2, hh2, b2 = call(loc, "PATCH", content,
                        {**auth, "Content-Type": "application/octet-stream"})
    print(f"  PATCH: {st2} | {b2[:120]!r}")
    fin = hh2.get("Location") or hh2.get("location") or loc
    if st2 in (200, 202) and finalize(fin, None) in (200, 201):
        print("RESULT: shape C works"); sys.exit(0)

print("RESULT: all shapes failed — inspect statuses above")
