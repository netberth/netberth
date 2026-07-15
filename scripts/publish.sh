#!/bin/bash
# NetBerth Docker Hub publish script
set -e

VERSION="${1:-$(date +%Y.%m.%d)}"
REGISTRY="${DOCKER_REGISTRY:-docker.io}"
IMAGE="${REGISTRY}/netberth/netberth"

echo "=== Publishing NetBerth v${VERSION} ==="

# Build multi-arch
docker buildx build --platform linux/amd64,linux/arm64 \
  -t "${IMAGE}:${VERSION}" \
  -t "${IMAGE}:latest" \
  --push .

echo "Published: ${IMAGE}:${VERSION}"
echo "Published: ${IMAGE}:latest"

echo ""
echo "Users can now deploy with:"
echo "  docker run -d --name netberth --network host \\"
echo "    -v netberth-data:/app/data \\"
echo "    -e NB_JWT_SECRET=\$(openssl rand -base64 48) \\"
echo "    ${IMAGE}:latest"
