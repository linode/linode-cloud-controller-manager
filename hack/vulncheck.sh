#!/usr/bin/env bash
set -uo pipefail
trap 'rm -f govulncheck.out' EXIT
govulncheck ./... > govulncheck.out
RC=$?
if [[ "${RC}" == 0 ]]; then
  echo "No vulnerabilities found"
  exit 0
fi
UNFIXABLE=$(grep -c "Fixed in: N/A" govulncheck.out)
TOTAL=$(grep -oE 'affected by [0-9]+' govulncheck.out | grep -oE '[0-9]+')
TOTAL=${TOTAL:-0}
if (( UNFIXABLE != TOTAL )); then
  cat govulncheck.out
  exit ${RC}
else
  echo "Found ${TOTAL} vulnerabilities but none are fixable; ignoring..."
fi
