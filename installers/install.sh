#!/usr/bin/env sh
# IA METER Collector installer for Linux and macOS.
#
# Usage:
#   curl -fsSL https://<your-domain>/install.sh | sh -s -- [--pair CODE] [--version X.Y.Z]
#
# Configurable via environment variables (section 21: "usa variables
# configurables, no presentes un dominio inexistente como producción"):
#   IAMETER_RELEASE_BASE_URL  Where to download release artifacts from.
#                             Defaults to this project's GitHub Releases.
#                             Point it at your own mirror/CI artifacts for
#                             local testing (see scripts/build-all.sh).
#   IAMETER_API_BASE_URL      Backend URL passed through to `iameter`.
#   IAMETER_INSTALL_DIR       Where to place the binary (default: see below).
#
# This script never claims to talk to a live production service: if
# IAMETER_RELEASE_BASE_URL is left at its default and no release has been
# published there yet, the download step fails with a clear error instead
# of silently succeeding.
set -eu

DEFAULT_RELEASE_BASE_URL="https://github.com/iameter/collector/releases"
RELEASE_BASE_URL="${IAMETER_RELEASE_BASE_URL:-$DEFAULT_RELEASE_BASE_URL}"
VERSION="${IAMETER_VERSION:-latest}"
PAIR_CODE=""

while [ $# -gt 0 ]; do
  case "$1" in
    --pair)
      PAIR_CODE="${2:-}"
      shift 2
      ;;
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    *)
      echo "iameter install: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

log()  { printf '%s\n' "$*"; }
warn() { printf '[WARN] %s\n' "$*" >&2; }
die()  { printf '[ERROR] %s\n' "$*" >&2; exit 1; }

# 1. Detect system
os_raw="$(uname -s)"
case "$os_raw" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *) die "unsupported operating system: $os_raw (this installer supports Linux and macOS only; use install.ps1 on Windows)" ;;
esac

# 2. Detect architecture
arch_raw="$(uname -m)"
case "$arch_raw" in
  x86_64|amd64)  arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) die "unsupported architecture: $arch_raw" ;;
esac

log "IA METER Collector installer"
log ""
log "✓ Sistema detectado: ${os} ${arch}"

# Default install dir: ~/.local/bin (Linux, XDG) or the same on macOS —
# a user-writable location, no sudo required (section 19).
INSTALL_DIR="${IAMETER_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$INSTALL_DIR"

TMPDIR="$(mktemp -d)"
cleanup() { rm -rf "$TMPDIR"; }
trap cleanup EXIT INT TERM

BINARY_NAME="iameter-${os}-${arch}"
DOWNLOAD_URL="${RELEASE_BASE_URL}/download/${VERSION}/${BINARY_NAME}"
CHECKSUMS_URL="${RELEASE_BASE_URL}/download/${VERSION}/checksums.txt"

download() {
  url="$1"
  out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$url" -O "$out"
  else
    die "neither curl nor wget is available"
  fi
}

# 3. Download the correct binary
log "Descargando ${BINARY_NAME} (${VERSION})..."
if ! download "$DOWNLOAD_URL" "${TMPDIR}/${BINARY_NAME}"; then
  die "no se pudo descargar ${DOWNLOAD_URL} — ¿existe ese release? Configura IAMETER_RELEASE_BASE_URL/IAMETER_VERSION si usas un mirror propio."
fi

# 4. Download checksums
log "Descargando checksums.txt..."
if ! download "$CHECKSUMS_URL" "${TMPDIR}/checksums.txt"; then
  die "no se pudo descargar checksums.txt — instalación abortada (no se verifica un binario sin checksum)."
fi

# 5. Verify SHA-256 — abort and clean up on any mismatch (section 24:
# "binarios manipulados").
log "Verificando SHA-256..."
EXPECTED_SUM="$(grep "  ${BINARY_NAME}\$" "${TMPDIR}/checksums.txt" | awk '{print $1}')"
[ -n "$EXPECTED_SUM" ] || die "checksums.txt no contiene una entrada para ${BINARY_NAME}"

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL_SUM="$(sha256sum "${TMPDIR}/${BINARY_NAME}" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL_SUM="$(shasum -a 256 "${TMPDIR}/${BINARY_NAME}" | awk '{print $1}')"
else
  die "neither sha256sum nor shasum is available to verify the download"
fi

if [ "$EXPECTED_SUM" != "$ACTUAL_SUM" ]; then
  die "checksum mismatch for ${BINARY_NAME}: expected ${EXPECTED_SUM}, got ${ACTUAL_SUM} — installation aborted, nothing was installed."
fi
log "✓ Checksum verificado"

# 6. Install to a user path
chmod +x "${TMPDIR}/${BINARY_NAME}"
TARGET="${INSTALL_DIR}/iameter"
if ! mv "${TMPDIR}/${BINARY_NAME}" "$TARGET"; then
  die "no se pudo mover el binario a ${TARGET} — instalación abortada."
fi
log "✓ Binario instalado en ${TARGET}"

case ":$PATH:" in
  *":${INSTALL_DIR}:"*) ;;
  *) warn "${INSTALL_DIR} no está en tu PATH. Añade esta línea a tu shell rc:  export PATH=\"${INSTALL_DIR}:\$PATH\"" ;;
esac

# 7-9. Run `iameter install` (statusLine + daemon service), optionally
# pairing, then `iameter doctor`.
INSTALL_ARGS=""
if [ -n "$PAIR_CODE" ]; then
  INSTALL_ARGS="--pair ${PAIR_CODE}"
fi

log ""
log "Ejecutando iameter install..."
# shellcheck disable=SC2086
if ! "$TARGET" install $INSTALL_ARGS; then
  die "iameter install falló. El binario permanece en ${TARGET}; ejecuta '${TARGET} install' manualmente tras revisar el error."
fi

log ""
log "Ejecutando iameter doctor..."
"$TARGET" doctor || true

log ""
log "Instalación completada. Abre Claude Code y envía un mensaje para obtener el primer dato de consumo,"
log "luego ejecuta: ${TARGET} status"
