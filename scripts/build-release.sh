#!/bin/bash
# NetBerth Release Builder — Mac → Linux
# Builds React frontend, embeds into Go binary, compiles Linux binary via Docker
set -e
cd "$(dirname "$0")/.."

echo "=== NetBerth Release Build ==="

# Step 1: Build React frontend
echo "[1/3] Building React frontend..."
cd web
npm install --silent 2>/dev/null
npm run build 2>&1 | tail -3
cd ..

# Step 2: Embed frontend into Go embed directory
echo "[2/3] Embedding frontend..."
rm -rf internal/api/handler/webroot/*
cp -r dist/web/* internal/api/handler/webroot/
echo "  Frontend embedded: $(ls internal/api/handler/webroot/ | wc -l) files"

# Step 3: Cross-compile Linux binary via Docker
echo "[3/3] Cross-compiling Linux binary..."
docker run --rm \
  -v "$(pwd)":/src \
  -w /src \
  -e GOPROXY=https://goproxy.cn,direct \
  golang:1.22-alpine \
  sh -c 'apk add --no-cache gcc musl-dev sqlite-dev && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -mod=vendor -ldflags="-s -w" -o netberth-linux ./cmd/netberth'

ls -lh netberth-linux
echo ""
echo "=== Build complete ==="
echo "Binary: ./netberth-linux"
echo "Deploy: scp netberth-linux root@unraid:/mnt/user/appdata/netberth/"
