#!/usr/bin/env bash
# Cross-compiles iameter for the 6 targets required by section 28 and
# writes SHA-256 checksums for all of them. No CGO, matching section 8.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

OUT_DIR="${OUT_DIR:-dist}"
VERSION="${VERSION:-0.1.0-dev}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

MODULE="github.com/valleedev/iameter-collector"
LDFLAGS="-s -w \
  -X ${MODULE}/internal/version.Version=${VERSION} \
  -X ${MODULE}/internal/version.Commit=${COMMIT} \
  -X ${MODULE}/internal/version.BuildDate=${BUILD_DATE}"

TARGETS=(
  "linux amd64"
  "linux arm64"
  "windows amd64"
  "windows arm64"
  "darwin amd64"
  "darwin arm64"
)

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

echo "Building iameter ${VERSION} (${COMMIT}) for ${#TARGETS[@]} targets..."

for target in "${TARGETS[@]}"; do
  read -r os arch <<< "$target"
  ext=""
  [ "$os" = "windows" ] && ext=".exe"
  out="${OUT_DIR}/iameter-${os}-${arch}${ext}"

  echo "  -> ${os}/${arch}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build \
    -trimpath \
    -ldflags "$LDFLAGS" \
    -o "$out" \
    ./cmd/iameter
done

echo "Writing checksums.txt..."
(
  cd "$OUT_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum iameter-* > checksums.txt
  else
    shasum -a 256 iameter-* > checksums.txt
  fi
)

echo "Done. Artifacts in ${OUT_DIR}/:"
ls -la "$OUT_DIR"
