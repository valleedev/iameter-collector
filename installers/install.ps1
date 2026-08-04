#Requires -Version 5.1
<#
.SYNOPSIS
  IA METER Collector installer for Windows.

.EXAMPLE
  .\install.ps1 -PairCode "CM-7X4P2Q"

.NOTES
  Configurable via environment variables or parameters (section 21: "usa
  variables configurables, no presentes un dominio inexistente como
  producción"). Never presents a fabricated production endpoint: if
  -ReleaseBaseUrl is left at its default and no release has been published
  there yet, the download step fails with a clear error.
#>
param(
    [string]$PairCode = "",
    [string]$Version = $(if ($env:IAMETER_VERSION) { $env:IAMETER_VERSION } else { "latest" }),
    [string]$ReleaseBaseUrl = $(if ($env:IAMETER_RELEASE_BASE_URL) { $env:IAMETER_RELEASE_BASE_URL } else { "https://github.com/valleedev/iameter-collector/releases" }),
    [string]$InstallDir = $(if ($env:IAMETER_INSTALL_DIR) { $env:IAMETER_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "IAMeter\bin" })
)

$ErrorActionPreference = "Stop"

function Write-Info($msg)  { Write-Host $msg }
function Write-WarnMsg($msg) { Write-Host "[WARN] $msg" -ForegroundColor Yellow }
function Fail($msg) { Write-Host "[ERROR] $msg" -ForegroundColor Red; exit 1 }

Write-Info "IA METER Collector installer"
Write-Info ""

# 2. Detect architecture (OS is always windows here)
$archRaw = $env:PROCESSOR_ARCHITECTURE
switch ($archRaw) {
    "AMD64" { $arch = "amd64" }
    "ARM64" { $arch = "arm64" }
    default { Fail "unsupported architecture: $archRaw" }
}
Write-Info "* Sistema detectado: windows $arch"

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

$tmpDir = Join-Path $env:TEMP ("iameter-install-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null
try {
    $binaryName = "iameter-windows-$arch.exe"
    $downloadUrl = "$ReleaseBaseUrl/download/$Version/$binaryName"
    $checksumsUrl = "$ReleaseBaseUrl/download/$Version/checksums.txt"
    $binaryPath = Join-Path $tmpDir $binaryName
    $checksumsPath = Join-Path $tmpDir "checksums.txt"

    # 3. Download the correct binary
    Write-Info "Descargando $binaryName ($Version)..."
    try {
        Invoke-WebRequest -Uri $downloadUrl -OutFile $binaryPath -UseBasicParsing
    } catch {
        Fail "no se pudo descargar $downloadUrl -- ¿existe ese release? Configura -ReleaseBaseUrl/-Version si usas un mirror propio. ($_)"
    }

    # 4. Download checksums
    Write-Info "Descargando checksums.txt..."
    try {
        Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath -UseBasicParsing
    } catch {
        Fail "no se pudo descargar checksums.txt -- instalacion abortada (no se verifica un binario sin checksum). ($_)"
    }

    # 5. Verify SHA-256
    Write-Info "Verificando SHA-256..."
    $checksumLine = Select-String -Path $checksumsPath -Pattern ([regex]::Escape($binaryName)) | Select-Object -First 1
    if (-not $checksumLine) {
        Fail "checksums.txt no contiene una entrada para $binaryName"
    }
    $expectedSum = ($checksumLine.Line -split '\s+')[0].ToLower()
    $actualSum = (Get-FileHash -Path $binaryPath -Algorithm SHA256).Hash.ToLower()
    if ($expectedSum -ne $actualSum) {
        Fail "checksum mismatch for $binaryName -- expected $expectedSum, got $actualSum -- installation aborted, nothing was installed."
    }
    Write-Info "* Checksum verificado"

    # 6. Install to a user path (no admin required)
    $target = Join-Path $InstallDir "iameter.exe"
    Move-Item -Force -Path $binaryPath -Destination $target
    Write-Info "* Binario instalado en $target"

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
        Write-WarnMsg "Se agrego $InstallDir a tu PATH de usuario. Abre una nueva terminal para que surta efecto."
    }

    # 7-9. Run `iameter install` (statusLine + service), optionally pair,
    # then `iameter doctor`.
    Write-Info ""
    Write-Info "Ejecutando iameter install..."
    $installArgs = @("install")
    if ($PairCode -ne "") { $installArgs += @("--pair", $PairCode) }
    & $target @installArgs
    if ($LASTEXITCODE -ne 0) {
        Fail "iameter install fallo. El binario permanece en $target; ejecutalo manualmente tras revisar el error."
    }

    Write-Info ""
    Write-Info "Ejecutando iameter doctor..."
    & $target doctor

    Write-Info ""
    Write-Info "Instalacion completada. Abre Claude Code y envia un mensaje para obtener el primer dato de consumo,"
    Write-Info "luego ejecuta: $target status"
} finally {
    Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
}
