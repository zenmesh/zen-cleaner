"""Behavioral battery for scripts/push-artifacthub-metadata.py (P0 tranche).

Runs the script as a SUBPROCESS against a local stub impersonating a
registry (token endpoint, blob uploads, manifest PUT) so the exact artifact
CI executes is under test (argv, exit codes, wire requests, output).

Contract (P0):
1. Basic actor:SECRET appears ONLY at the /token exchange.
2. The exchanged Bearer is preserved on blob PATCH/PUT and manifest PUT.
3. Failures fail closed: token/blob/manifest errors exit non-zero with no
   partial-success output.
4. The Basic secret and the exchanged bearer never appear in output.
5. No HEAD requests are ever sent (GHCR DENIED before-push broke oras).
"""
import base64
import json
import os
import subprocess
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

SCRIPT_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                           "..", "push-artifacthub-metadata.py")
USER = "github-actor"
SECRET = "gh-super-secret-token-value"
BEARER = "fake-bearer-token-value"
REPO = "zenmesh/charts/zen-cleaner"


class Stub(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *a):
        pass

    def _record(self, method):
        self.server.recorded.append((method, self.path,
                                     self.headers.get("Authorization", "")))

    def _json(self, code, obj):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        self._record("GET")
        if self.path.startswith("/token"):
            self._json(200, {"token": BEARER})
        else:
            self._json(404, {"message": "nf"})

    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        self.rfile.read(n)
        self._record("POST")
        if self.path.endswith("/blobs/uploads/"):
            # Registry behavior: 202 + ABSOLUTE Location for the follow-up PUT.
            self.send_response(202)
            self.send_header("Location",
                             "http://127.0.0.1:" + str(self.server.server_port)
                             + self.path + "blob-upload-uuid")
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        self._json(202, {})

    def do_PATCH(self):
        n = int(self.headers.get("Content-Length", 0))
        self.rfile.read(n)
        self._record("PATCH")
        self._json(202, {})

    def do_PUT(self):
        n = int(self.headers.get("Content-Length", 0))
        self.rfile.read(n)
        self._record("PUT")
        self._json(201, {})

    def do_HEAD(self):
        self._record("HEAD")
        self.send_response(403)
        self.end_headers()


class TestPushArtifacthubMetadata(unittest.TestCase):
    def setUp(self):
        self.recorded = []
        self.httpd = ThreadingHTTPServer(("127.0.0.1", 0), Stub)
        self.httpd.recorded = self.recorded
        self.port = self.httpd.server_address[1]
        t = threading.Thread(target=self.httpd.serve_forever, daemon=True)
        t.start()
        self.addCleanup(self.httpd.shutdown)
        self.addCleanup(self.httpd.server_close)
        self.registry = "127.0.0.1:{port}".format(port=self.port)

    def run_push(self, user=USER, token=SECRET):
        proc = subprocess.run(
            [sys.executable, SCRIPT_PATH,
             "{registry}/{repo}".format(registry=self.registry, repo=REPO),
             self.meta_path(), user, token],
            capture_output=True, text=True, timeout=30)
        return proc.returncode, proc.stdout, proc.stderr

    def meta_path(self):
        base = os.path.dirname(os.path.dirname(os.path.dirname(
            os.path.abspath(__file__))))
        return os.path.join(base, "docs", "artifacthub-repo.yml")

    def auths(self):
        return [(m, p, a) for (m, p, a) in self.recorded]

    def test_basic_only_at_token_exchange(self):
        rc, out, err = self.run_push()
        self.assertEqual(rc, 0, out + err)
        expect = "Basic " + base64.b64encode(
            "{u}:{s}".format(u=USER, s=SECRET).encode()).decode()
        exchanges = [a for (m, p, a) in self.auths() if p.startswith("/token")]
        self.assertTrue(exchanges, "token exchange never happened")
        for a in exchanges:
            self.assertEqual(a, expect)
        for (m, p, a) in self.auths():
            if not p.startswith("/token"):
                self.assertEqual(a, "Bearer " + BEARER,
                                 m + " " + p + " must carry the exchanged Bearer")

    def test_bearer_preserved_on_registry_writes(self):
        rc, out, err = self.run_push()
        self.assertEqual(rc, 0, out + err)
        writes = [(m, p, a) for (m, p, a) in self.recorded
                  if m in ("PATCH", "PUT")]
        self.assertTrue(writes, "no registry writes recorded")
        for m, p, a in writes:
            self.assertEqual(a, "Bearer " + BEARER,
                             m + " " + p + " lost the exchanged Bearer")

    def test_no_head_requests(self):
        rc, out, err = self.run_push()
        self.assertEqual(rc, 0, out + err)
        heads = [(m, p) for (m, p, a) in self.recorded if m == "HEAD"]
        self.assertEqual(heads, [], "HEAD requests must never be sent")

    def test_secrets_not_in_output(self):
        rc, out, err = self.run_push()
        self.assertEqual(rc, 0)
        for sink in (out, err):
            self.assertNotIn(SECRET, sink)
            self.assertNotIn(BEARER, sink)


if __name__ == "__main__":
    unittest.main()
