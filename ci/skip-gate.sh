#!/usr/bin/env bash
# Fails when a `go test -json` run skipped a test, printing each skipped test
# and its reason. With a second results file, a skipped test is accepted when
# it passed there (the unit run has no database; its DB tests are skipped and
# must pass in the API run instead).
#
#   ci/skip-gate.sh <label> <results.json> [<results-where-skips-must-pass.json>]
set -euo pipefail
export LC_ALL=C
LABEL=$1 JSON=$2 COVER=${3:-}

SKIPPED=$(jq -r 'select(.Action=="skip" and .Test!=null) | "\(.Package)\t\(.Test)"' "$JSON" | sort -u)
if [ -z "$SKIPPED" ]; then
  echo "SKIPS ($LABEL): 0"
  exit 0
fi
PASSED_THERE=""
[ -n "$COVER" ] && PASSED_THERE=$(jq -r 'select(.Action=="pass" and .Test!=null) | "\(.Package)\t\(.Test)"' "$COVER" | sort -u)

TOTAL=$(printf '%s\n' "$SKIPPED" | wc -l | tr -d ' ')
UNCOVERED=$(comm -23 <(printf '%s\n' "$SKIPPED") <(printf '%s\n' "$PASSED_THERE"))
if [ -z "$UNCOVERED" ]; then
  echo "SKIPS ($LABEL): $TOTAL, every one passed in $(basename "$COVER")"
  exit 0
fi
N=$(printf '%s\n' "$UNCOVERED" | wc -l | tr -d ' ')
echo "SKIPS ($LABEL): $TOTAL, $N not run anywhere:"
printf '%s\n' "$UNCOVERED" | head -100 | while IFS=$'\t' read -r pkg test; do
  why=$(jq -r --arg p "$pkg" --arg t "$test" 'select(.Action=="output" and .Package==$p and .Test==$t) | .Output' "$JSON" \
    | grep -v -e '^=== ' -e '^--- SKIP' | head -1 | sed 's/^ *//')
  echo "  ${pkg##*/} $test: ${why:-no reason given}"
done
echo "$LABEL: $N skipped test(s) did not run. Failing."
exit 1
