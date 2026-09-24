#!/bin/bash
# SUPPORT2-047: promtool rule validation + firing fixture for zen-cleaner.
set -e
IMAGE="${PROMTOOL_IMAGE:-prom/prometheus:v2.53.0}"
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"   # deploy/prometheus

echo "== promtool check: rule files =="
fail=0
for f in "$ROOT"/prometheus-rules.yaml; do
  [ -f "$f" ] || continue
  python3 - "$f" <<'PY'
import yaml, sys
d = yaml.safe_load(open(sys.argv[1]))
g = d.get('groups') or d.get('spec', {}).get('groups', [])
yaml.safe_dump({'groups': g}, open('/tmp/zen-check.yml', 'w'))
PY
  if docker run --rm -v /tmp/zen-check.yml:/rules.yml:ro --entrypoint /bin/promtool "$IMAGE" check rules /rules.yml >/dev/null; then
    echo "OK   $(basename "$f")"
  else
    echo "FAIL $(basename "$f")"; fail=1
  fi
done
[ $fail -eq 0 ] || { echo "RULE VALIDATION FAILED"; exit 1; }

echo "== firing fixture: CleanerControllerDown =="
docker run --rm -v "$HERE:/t" --entrypoint /bin/promtool "$IMAGE" test rules /t/controller-down-firing.yml
echo "FIRING FIXTURE PASS"
