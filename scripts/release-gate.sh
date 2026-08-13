#!/bin/bash
# NetBerth release gate — the one command that must be green before publishing.
#
# Stages:
#   1. Private tree clean + version consistency (Go source == web package.json)
#   2. Full Go test suite
#   3. scripts/release.sh: frontend embed + zig cross-build + strings/checksum checks
#   4. Mirror private -> public with the canonical exclude list (never rm -rf the public repo)
#   5. Audit the public tree: forbidden files + sensitive patterns
#   6. Independent build + test inside the public tree
#
# Usage:
#   ./scripts/release-gate.sh                 # full gate
#   SKIP_TESTS=1 ./scripts/release-gate.sh    # skip slow Go tests (CI only, not for release)
set -uo pipefail
cd "$(dirname "$0")/.."
ROOT=$(pwd)
PUBLIC="${NB_PUBLIC_DIR:-../netberth-public}"
GOCACHE_TMP="${GOCACHE:-/private/tmp/nb-gocache}"
PASS_N=0; FAIL_N=0
check(){ name=$1; want=$2; got=$3; note=$4
  if [ "$want" = "$got" ]; then echo "  [PASS] $name ($note)"; PASS_N=$((PASS_N+1))
  else echo "  [FAIL] $name want=$want got=$got ($note)"; FAIL_N=$((FAIL_N+1)); fi
}
version=$(sed -n 's/^const Version = "\(.*\)"$/\1/p' pkg/version/version.go)
webver=$(sed -n 's/^  "version": "\(.*\)",$/\1/p' web/package.json | head -1)

echo "=== NetBerth release gate v${version} ==="
echo "private=$ROOT public=$PUBLIC"

echo
echo "--- [1/6] private tree + version consistency ---"
DIRTY=$(git status --porcelain | wc -l | tr -d ' ')
check "private_tree_clean" 0 "$DIRTY" "no uncommitted changes"
check "version_consistency" "$version" "$webver" "pkg/version == web/package.json"
if [ "$version" = "1.2.0" ]; then
  echo "  [WARN] version still 1.2.0 — did you bump for this release?"
fi

if [ "${SKIP_TESTS:-0}" != "1" ]; then
  echo
  echo "--- [2/6] full Go test suite ---"
  if GOCACHE="$GOCACHE_TMP" go test -count=1 ./... >/tmp/nb-gate-go-test.log 2>&1; then
    echo "  [PASS] go_test_all (see /tmp/nb-gate-go-test.log)"
    PASS_N=$((PASS_N+1))
  else
    echo "  [FAIL] go_test_all (see /tmp/nb-gate-go-test.log)"
    FAIL_N=$((FAIL_N+1))
    tail -20 /tmp/nb-gate-go-test.log
  fi
else
  echo "--- [2/6] go tests skipped (SKIP_TESTS=1) ---"
fi

echo
echo "--- [3/6] release build (frontend + zig + strings + sha256) ---"
if ./scripts/release.sh >/tmp/nb-gate-release.log 2>&1; then
  echo "  [PASS] release_build (see /tmp/nb-gate-release.log)"
  PASS_N=$((PASS_N+1))
else
  echo "  [FAIL] release_build (see /tmp/nb-gate-release.log)"
  FAIL_N=$((FAIL_N+1))
  tail -25 /tmp/nb-gate-release.log
fi

echo
echo "--- [4/6] mirror private -> public (in place, no rm -rf) ---"
if [ ! -d "$PUBLIC/.git" ]; then
  echo "  [FAIL] $PUBLIC/.git missing — refusing to sync into a non-repo"
  FAIL_N=$((FAIL_N+1))
else
  rsync -a --delete \
    --exclude='.git' \
    --exclude='internal/licensing/enterprise' \
    --exclude='internal/licensing/factory_ent.go' \
    --exclude='*report*.md' \
    --exclude='project-snapshot.md' \
    --exclude='release-manifest.md' \
    --exclude='PROJECT_HERITAGE.md' --exclude='TEAM.md' \
    --exclude='bin' --exclude='data' --exclude='certs' --exclude='dist' \
    --exclude='node_modules' \
    --exclude='*.db' --exclude='*.jwt_secret' --exclude='*.bundle' \
    --exclude='HANDOVER.md' --exclude='AGENTS.md' \
    --exclude='docs/design' \
    --exclude='* 2.*' --exclude='* 3' \
    --exclude='.DS_Store' \
    --exclude='/netberth' --exclude='/netberth-linux' --exclude='/netharbor' --exclude='/netharbor-linux' \
    "$ROOT/" "$PUBLIC/"
  check "public_sync" 0 "$?" "rsync completed"
fi

echo
echo "--- [5/6] public tree audit ---"
FORBIDDEN=$(find "$PUBLIC" -not -path '*/.git/*' \( \
  -name 'HANDOVER.md' -o -name 'AGENTS.md' -o -name 'TEAM.md' \
  -o -name 'PROJECT_HERITAGE.md' -o -name '*report*.md' \
  -o -name 'project-snapshot.md' -o -name 'release-manifest.md' \
  -o -name '*.db' -o -name '*.jwt_secret' -o -name '*.bundle' \
  -o -path '*/internal/licensing/enterprise*' -o -name 'factory_ent.go' \
  -o -path '*/docs/design*' \) -print | head -20)
check "public_forbidden_files" "" "${FORBIDDEN}" "no private files leaked"
SENS=$(cd "$PUBLIC" && rg -l --hidden -g '!.git' -g '!dist' -g '!node_modules' -g '!vendor' \
  -g '!scripts/release.sh' -g '!scripts/release-gate.sh' \
  -e '/Users/' -e '~/dev' -e 'netberth-quarantine' -e 'netharbor' \
  -e 'GenerateLicense' -e '10\.5\.5\.' -e 'k3y-2024|prod-v1-secure' -e 'onetwothree' \
  . 2>/dev/null | head -20)
check "public_sensitive_scan" "" "${SENS}" "no private markers in public tree"

echo
echo "--- [6/6] independent build + test in public tree ---"
if [ "${SKIP_TESTS:-0}" != "1" ]; then
  if (cd "$PUBLIC" && GOCACHE=/private/tmp/nb-gocache-public go build ./... && \
      GOCACHE=/private/tmp/nb-gocache-public go test -count=1 ./...) >/tmp/nb-gate-public-test.log 2>&1; then
    echo "  [PASS] public_build_and_test (see /tmp/nb-gate-public-test.log)"
    PASS_N=$((PASS_N+1))
  else
    echo "  [FAIL] public_build_and_test (see /tmp/nb-gate-public-test.log)"
    FAIL_N=$((FAIL_N+1))
    tail -25 /tmp/nb-gate-public-test.log
  fi
else
  echo "  [SKIP] public tests skipped (SKIP_TESTS=1)"
fi

echo
echo "=== release gate summary: PASS=$PASS_N FAIL=$FAIL_N (v${version} @ $(git rev-parse --short HEAD)) ==="
[ "$FAIL_N" -eq 0 ] || exit 1
