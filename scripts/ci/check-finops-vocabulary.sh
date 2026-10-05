#!/bin/bash
# Copyright 2026 Zen-Mesh Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# CB-2 (PI-SC-001 P0, 2026-10-05): THE FINOPS-VOCABULARY GUARD.
#
# The ADV-CLEANER engine is PRIVATE; this module is PUBLIC with nothing
# around finops. The owner's history scrub removed the engine — this
# guard exists so the leak class can NEVER re-land:
#
#   mode tree    — scan the tracked tree for the private vocabulary
#   mode commits — scan NEW commit messages on push (stdin: range)
#
# Fragments law: the needles are assembled at runtime so no line of
# this script carries a complete marker (the scanner's own idiom).
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

# ── needle fragments (assembled below; never a complete literal) ──
_m1="mainte"; _m2="nanceopportunity"; _m3="mainte"; _m4="nance"
_p1="pvc"; _p2="detect"; _p3="or"
_f1="fin"; _f2="ops"
_r1="right"; _r2="sizing"
_c1="cost-"; _c2="class"
_s1="savings"; _s2="-engine"

NEEDLES=("${_m1}${_m2}" "${_m3}${_m4}opportunity" "${_p1}${_p2}${_p3}" "${_f1}${_f2}" "${_r1}${_r2}" "${_c1}${_c2}" "${_s1}${_s2}")

mode="${1:-tree}"
violations=0

scan_line() {
  local line="$1" where="$2"
  for needle in "${NEEDLES[@]}"; do
    if [[ "${line,,}" == *"${needle}"* ]]; then
      echo "FINOPS-VOCAB GUARD: $where carries private-engine vocabulary ($needle)" >&2
      violations=$((violations + 1))
    fi
  done
}

case "$mode" in
  tree)
    while IFS= read -r file; do
      # The guard's own files NAME the law to enforce it (the scanner's
      # own exception) — they are never leaks.
      case "$file" in
        test/repo/guard_v4_test.go|test/repo/guard_v5_test.go|test/repo/guard_v6_test.go|scripts/ci/check-finops-vocabulary.sh)
          continue
          ;;
      esac
      while IFS= read -r line; do
        scan_line "$line" "$file"
      done < "$file"
    done < <(git ls-files | grep -vE '^scripts/ci/check-finops-vocabulary\.sh$')
    ;;
  commits)
    range="${2:-}"
    [ -z "$range" ] && { echo "commits mode needs a range (e.g. origin/main..HEAD)" >&2; exit 2; }
    while IFS= read -r msg; do
      scan_line "$msg" "commit-message"
    done < <(git log --format=%B "$range")
    ;;
  *)
    echo "unknown mode: $mode (tree|commits)" >&2
    exit 2
    ;;
esac

if [ "$violations" -gt 0 ]; then
  echo "FINOPS-VOCAB GUARD: $violations violation(s) — the ADV-CLEANER engine is private; refuse" >&2
  exit 1
fi
echo "FINOPS-VOCAB GUARD: clean"
