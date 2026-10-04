#!/usr/bin/env sh
# Builds the Property Finder image for a remote Docker host and exports it as
# deploy/property-finder-image.tar.gz (docker save | gzip).
#
# Usage (from anywhere):
#   deploy/build-image.sh                          # linux/amd64 (default)
#   PLATFORM=linux/arm64 deploy/build-image.sh     # e.g. Raspberry Pi / ARM NAS
#
# Workflow on the host: docs/deployment.md.
set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
PLATFORM=${PLATFORM:-linux/amd64}
IMAGE=property-finder
VERSION=$(git -C "$ROOT" describe --always --dirty 2>/dev/null || echo dev)
OUT="$ROOT/deploy/property-finder-image.tar.gz"

echo "Building $IMAGE:$VERSION for $PLATFORM ..."
docker buildx build \
	--platform "$PLATFORM" \
	--file "$ROOT/backend/Dockerfile" \
	--tag "$IMAGE:latest" \
	--tag "$IMAGE:$VERSION" \
	--load \
	"$ROOT"

echo "Exporting to $OUT ..."
docker save "$IMAGE:latest" "$IMAGE:$VERSION" | gzip >"$OUT"

echo "Done: $OUT ($(du -h "$OUT" | cut -f1))"
echo "Copy deploy/docker-compose.yml, deploy/.env.example and the tarball to the host — see docs/deployment.md."
