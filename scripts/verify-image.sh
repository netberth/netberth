#!/bin/sh
# Verify the NetBerth GHCR image signature and SLSA provenance.
#
# Usage:
#   ./scripts/verify-image.sh [IMAGE]
#
# Defaults to the latest release tag. Requires cosign:
#   brew install cosign   (or https://docs.sigstore.dev/cosign/installation/)
set -eu

IMAGE="${1:-ghcr.io/netberth/netberth:latest}"
IDENTITY='^https://github\.com/netberth/netberth/\.github/workflows/ci\.yml@refs/(heads/(main|v[0-9.]+)|tags/v[0-9.]+)$'
ISSUER='https://token.actions.githubusercontent.com'

echo "Verifying $IMAGE"
cosign verify \
  --certificate-identity-regexp "$IDENTITY" \
  --certificate-oidc-issuer "$ISSUER" \
  "$IMAGE"

cosign verify-attestation \
  --type slsaprovenance \
  --certificate-identity-regexp "$IDENTITY" \
  --certificate-oidc-issuer "$ISSUER" \
  "$IMAGE"

echo "OK: signature and SLSA provenance verified for $IMAGE"
