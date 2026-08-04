#Requires -Version 5.1
<#
.SYNOPSIS
  IA METER Collector uninstaller for Windows.
.NOTES
  Restores Claude Code's statusLine, removes the Scheduled Task, removes
  pairing credentials, and deletes the installed binary. Local config/data
  (device id, cached usage, queue) is left in place by default.
#>
param(
    [string]$InstallDir = $(if ($env:IAMETER_INSTALL_DIR) { $env:IAMETER_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "IAMeter\bin" })
)

function Write-Info($msg)  { Write-Host $msg }
function Write-WarnMsg($msg) { Write-Host "[WARN] $msg" -ForegroundColor Yellow }

$target = Join-Path $InstallDir "iameter.exe"

if (-not (Test-Path $target)) {
    Write-Info "iameter no esta instalado en $target (nada que hacer)."
    exit 0
}

Write-Info "IA METER Collector uninstaller"
Write-Info ""

Write-Info "Restaurando configuracion de Claude Code..."
& $target uninstall
if ($LASTEXITCODE -ne 0) { Write-WarnMsg "iameter uninstall reporto un error; revisa manualmente %USERPROFILE%\.claude\settings.json" }

Write-Info "Eliminando credenciales de emparejamiento..."
& $target unpair
if ($LASTEXITCODE -ne 0) { Write-WarnMsg "iameter unpair reporto un error" }

Write-Info "Eliminando binario ($target)..."
Remove-Item -Force $target -ErrorAction SilentlyContinue

Write-Info ""
Write-Info "* IA METER desinstalado."
Write-Info "  La configuracion local (device id, cola, cache de consumo) se conservo."
Write-Info "  Para borrarla completamente, elimina tambien: $env:LOCALAPPDATA\IAMeter"
