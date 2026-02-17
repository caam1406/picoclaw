#!/usr/bin/env bash
#
# Cross-compile picoclaw for all supported platforms.
#
# Usage:
#   ./scripts/build.sh                         # build all platforms
#   ./scripts/build.sh linux-amd64             # build one platform
#   ./scripts/build.sh linux-amd64 darwin-arm64  # build specific platforms
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BUILD_DIR="${SCRIPT_DIR}/../build"

# Version from git
VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo "dev")"
BUILD_TIME="$(date +%FT%T%z)"

LDFLAGS="-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}"

# All supported platforms
ALL_PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
    "darwin/amd64"
    "darwin/arm64"
)

# Filter if arguments provided
if [ $# -gt 0 ]; then
    PLATFORMS=()
    for arg in "$@"; do
        PLATFORMS+=("${arg/-/\/}")
    done
else
    PLATFORMS=("${ALL_PLATFORMS[@]}")
fi

mkdir -p "${BUILD_DIR}"

echo "Building picoclaw ${VERSION}"
echo ""

FAILED=()

for platform in "${PLATFORMS[@]}"; do
    GOOS="${platform%/*}"
    GOARCH="${platform#*/}"

    SUFFIX=""
    [ "${GOOS}" = "windows" ] && SUFFIX=".exe"

    OUT_NAME="picoclaw-${GOOS}-${GOARCH}${SUFFIX}"
    OUT_PATH="${BUILD_DIR}/${OUT_NAME}"

    printf "  [%s/%s] %s ... " "${GOOS}" "${GOARCH}" "${OUT_NAME}"

    if CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" \
       go build -ldflags "${LDFLAGS}" -o "${OUT_PATH}" "./cmd/picoclaw" 2>&1; then
        SIZE="$(du -h "${OUT_PATH}" | cut -f1)"
        echo "OK (${SIZE})"
    else
        echo "FAILED"
        FAILED+=("picoclaw ${GOOS}/${GOARCH}")
    fi
done

echo ""

if [ ${#FAILED[@]} -gt 0 ]; then
    echo "Failed: ${FAILED[*]}"
    exit 1
else
    echo "All builds complete! Binaries in: ${BUILD_DIR}"
    ls -lh "${BUILD_DIR}/picoclaw-"* 2>/dev/null || true
fi
