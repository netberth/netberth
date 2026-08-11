#!/bin/bash
# NetBerth release builder — frontend embed + zig cross-compile + security checks.
# Usage: ./scripts/release.sh   (outputs to dist/release by default, override with OUT=...)
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT=$(pwd)
OUT="${OUT:-dist/release}"
VERSION=$(sed -n 's/^const Version = "\(.*\)"$/\1/p' pkg/version/version.go)
if [ -z "$VERSION" ]; then
  echo "ERROR: could not read version from pkg/version/version.go" >&2
  exit 1
fi

echo "=== NetBerth release build v${VERSION} ==="

# 1) Frontend
echo "[1/4] Building frontend..."
(cd web && npm run build >/dev/null 2>&1)
rm -rf internal/api/handler/webroot/*
cp -r dist/web/* internal/api/handler/webroot/
if grep -rni "netharbor" internal/api/handler/webroot/; then
  echo "ERROR: netharbor found in webroot" >&2
  exit 1
fi
echo "  frontend embedded"

# 2) Zig check
if ! command -v zig >/dev/null 2>&1; then
  echo "ERROR: zig is required (brew install zig)" >&2
  exit 1
fi

# 3) Cross-compile
mkdir -p "$OUT"
WRAPDIR=$(mktemp -d)
trap 'rm -rf "$WRAPDIR"' EXIT
cat > "$WRAPDIR/zigcc-amd64.sh" <<WRAP
#!/bin/sh
exec zig cc -target x86_64-linux-musl -ffile-prefix-map="\$HOME"=/BUILDER "\$@"
WRAP
cat > "$WRAPDIR/zigcc-arm64.sh" <<WRAP
#!/bin/sh
exec zig cc -target aarch64-linux-musl -ffile-prefix-map="\$HOME"=/BUILDER "\$@"
WRAP
chmod +x "$WRAPDIR/zigcc-amd64.sh" "$WRAPDIR/zigcc-arm64.sh"

echo "[2/4] Cross-compiling linux/amd64..."
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC="$WRAPDIR/zigcc-amd64.sh" \
  go build -mod=vendor -trimpath -buildvcs=false -ldflags='-s -w' \
  -o "$OUT/netberth-linux-amd64" ./cmd/netberth
echo "[3/4] Cross-compiling linux/arm64..."
CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC="$WRAPDIR/zigcc-arm64.sh" \
  go build -mod=vendor -trimpath -buildvcs=false -ldflags='-s -w' \
  -o "$OUT/netberth-linux-arm64" ./cmd/netberth

# 4) Security checks
echo "[4/4] Running strings checks..."
FAIL=0
for bin in "$OUT/netberth-linux-amd64" "$OUT/netberth-linux-arm64"; do
  name=$(basename "$bin")
  checks=(
    '/Users/' user-path
    'netharbor' old-name
    'GenerateLicense' license-generator
    '10\.5\.5\.' internal-ip
    'k3y-2024|prod-v1-secure' old-secret
    'onetwothree' other-account
  )
  for ((i = 0; i < ${#checks[@]}; i += 2)); do
    pattern="${checks[$i]}"
    label="${checks[$((i+1))]}"
    count=$(strings "$bin" | grep -cE "$pattern" || true)
    printf '  %-18s %-16s %s\n' "$name" "$label" "$count"
    if [ "$count" != "0" ]; then FAIL=1; fi
  done
done
if [ "$FAIL" != "0" ]; then
  echo "ERROR: strings checks failed" >&2
  exit 1
fi

(cd "$OUT" && shasum -a 256 netberth-linux-amd64 netberth-linux-arm64 > sha256sums.txt)
echo ""
echo "=== Release artifacts (v${VERSION}) ==="
ls -lh "$OUT"
echo ""
echo "Next: gh release create v${VERSION} --draft ... $OUT/netberth-linux-amd64 $OUT/netberth-linux-arm64 $OUT/sha256sums.txt"
