#!/usr/bin/env sh
# IA METER Collector uninstaller for Linux and macOS.
#
# Usage:
#   curl -fsSL https://<your-domain>/uninstall.sh | sh
#
# Restores Claude Code's statusLine to its pre-IAMETER state, removes the
# background service registration, removes local pairing credentials, and
# deletes the installed binary. Local config/data (device id, cached
# usage, queue) are left in place by default — nothing the user typed or
# that represents their consumption history is deleted without being asked.
set -eu

INSTALL_DIR="${IAMETER_INSTALL_DIR:-$HOME/.local/bin}"
TARGET="${INSTALL_DIR}/iameter"

log()  { printf '%s\n' "$*"; }
warn() { printf '[WARN] %s\n' "$*" >&2; }

if [ ! -x "$TARGET" ]; then
  log "iameter no está instalado en ${TARGET} (nada que hacer)."
  exit 0
fi

log "IA METER Collector uninstaller"
log ""

log "Restaurando configuración de Claude Code..."
"$TARGET" uninstall || warn "iameter uninstall reportó un error; revisa manualmente ~/.claude/settings.json"

log "Eliminando credenciales de emparejamiento..."
"$TARGET" unpair || warn "iameter unpair reportó un error"

log "Eliminando binario (${TARGET})..."
rm -f "$TARGET"

log ""
log "✓ IA METER desinstalado."
log "  La configuración local (device id, cola, caché de consumo) se conservó."
log "  Para borrarla completamente, elimina también:"
log "    ~/.config/iameter (o \$XDG_CONFIG_HOME/iameter)"
log "    ~/.local/share/iameter (o \$XDG_DATA_HOME/iameter)"
log "    ~/Library/Application Support/IAMeter (macOS)"
