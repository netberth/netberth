#!/bin/bash
# NetBerth multi-round QA runner — executes the full isolated suite
# NB_QA_ROUNDS times (default 3) and reports a per-round verdict.
#
# Usage:
#   ./qa/rounds.sh                  # 3 full rounds (soak only if NB_QA_SOAK=1)
#   NB_QA_ROUNDS=2 NB_QA_SOAK=1 ./qa/rounds.sh
set -uo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
ROUNDS="${NB_QA_ROUNDS:-3}"
OVERALL_FAIL=0

echo "=== NetBerth multi-round QA: ${ROUNDS} rounds ==="
for i in $(seq 1 "$ROUNDS"); do
  echo
  echo "########## ROUND ${i}/${ROUNDS} ##########"
  if "$DIR/run-all.sh"; then
    echo ">>> round ${i}/${ROUNDS}: ALL GREEN"
  else
    echo ">>> round ${i}/${ROUNDS}: FAILED"
    OVERALL_FAIL=1
  fi
done

echo
echo "=== multi-round summary: ${ROUNDS} rounds, overall_fail=${OVERALL_FAIL} ==="
[ "$OVERALL_FAIL" -eq 0 ] || exit 1
