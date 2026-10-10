#!/usr/bin/env bash
# Summarises a `go test -json` stream: prints the output of every failing
# test, per-package and total passed/failed/skipped counts (also written to
# the GitHub job summary when $GITHUB_STEP_SUMMARY is set), and exits 1 when
# go test failed, any test failed, or fewer than MIN_PASS tests passed.
#
#   ci/test-summary.sh <results.json> <go-test-exit-code> <label> [MIN_PASS]
set -euo pipefail
JSON=$1 EXIT=$2 LABEL=$3 MIN_PASS=${4:-0}

FAILED=$(jq -r 'select(.Action=="fail" and .Test!=null) | "\(.Package)\t\(.Test)"' "$JSON")
if [ -n "$FAILED" ]; then
  echo "::group::Failing tests output"
  echo "$FAILED" | while IFS=$'\t' read -r pkg test; do
    jq -j --arg p "$pkg" --arg t "$test" 'select(.Action=="output" and .Package==$p and .Test==$t) | .Output' "$JSON"
  done
  echo "::endgroup::"
fi
# package-level errors (panics, TestMain failures) when no test is to blame
if [ "$EXIT" -ne 0 ] && [ -z "$FAILED" ]; then
  jq -r 'select(.Action=="output" and .Test==null) | .Output' "$JSON" | grep -v '^$' || true
fi
RACES=$(jq -r 'select(.Action=="output") | .Output' "$JSON" | grep -c '^WARNING: DATA RACE' || true)

echo
echo "$LABEL, per package (top-level tests and subtests):"
jq -r 'select(.Test!=null and (.Action=="pass" or .Action=="fail" or .Action=="skip")) | "\(.Package|sub(".*/backend/?";"./"))\t\(.Action)"' "$JSON" \
  | sort | uniq -c | awk '{printf "  %-16s %-5s %6d\n", $2, $3, $1}'
PASS=$(jq -s '[.[] | select(.Test!=null and .Action=="pass")] | length' "$JSON")
FAIL=$(jq -s '[.[] | select(.Test!=null and .Action=="fail")] | length' "$JSON")
SKIP=$(jq -s '[.[] | select(.Test!=null and .Action=="skip")] | length' "$JSON")
echo
echo "TEST SUMMARY ($LABEL): passed=$PASS failed=$FAIL skipped=$SKIP data_races=$RACES (go test exit $EXIT)"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "### $LABEL"
    echo "| passed | failed | skipped | data races |"
    echo "|---|---|---|---|"
    echo "| $PASS | $FAIL | $SKIP | $RACES |"
  } >> "$GITHUB_STEP_SUMMARY"
fi
if [ "$EXIT" -ne 0 ] || [ "$FAIL" -ne 0 ]; then
  echo "$LABEL FAILED."
  exit 1
fi
if [ "$PASS" -lt "$MIN_PASS" ]; then
  echo "Only $PASS tests passed (expected $MIN_PASS+); tests may have been skipped. Failing."
  exit 1
fi
