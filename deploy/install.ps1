<#
.SYNOPSIS
  valheim-server-ui installer for Windows Server 2019+ / Windows 10+ (x64).

.DESCRIPTION
  Installs or upgrades the manager as the Windows service "valheim-ui",
  running under the virtual service account NT SERVICE\valheim-ui, with
  its data under %ProgramData%\valheim-ui. Game instances are child
  processes of the manager (supervisor: direct); there is no systemd or
  sudo wrapper on Windows, see docs/RUNBOOK.md §13.

  Run from an elevated PowerShell:

    irm https://raw.githubusercontent.com/jonasthim/valheim-server-ui/main/deploy/install.ps1 -OutFile install.ps1
    .\install.ps1 [-BaseUrl https://valheim.example.com]

  Re-running upgrades the binary and keeps config.yaml and all data.

.PARAMETER Version
  Release tag to install (default: latest).
.PARAMETER InstallDir
  Where valheim-ui.exe lives (default: %ProgramFiles%\valheim-ui).
.PARAMETER DataDir
  Data directory (default: %ProgramData%\valheim-ui).
.PARAMETER Listen
  Listen address for the web UI (default: 127.0.0.1:8080).
.PARAMETER BaseUrl
  Public URL of the UI (required for OIDC and secure cookies behind a proxy).
.PARAMETER NoSteamCmd
  Skip downloading SteamCMD into <DataDir>\steamcmd.
.PARAMETER Check
  Report the installed version and service state, change nothing.
.PARAMETER Uninstall
  Remove the service and the binary. Data is kept.
#>
#Requires -RunAsAdministrator
[CmdletBinding()]
param(
  [string]$Version = 'latest',
  [string]$InstallDir = (Join-Path $env:ProgramFiles 'valheim-ui'),
  [string]$DataDir = (Join-Path $env:ProgramData 'valheim-ui'),
  [string]$Listen = '127.0.0.1:8080',
  [string]$BaseUrl = '',
  [switch]$NoSteamCmd,
  [switch]$Check,
  [switch]$Uninstall
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$Repo = 'jonasthim/valheim-server-ui'
$ServiceName = 'valheim-ui'
$Account = "NT SERVICE\$ServiceName"
$Exe = Join-Path $InstallDir 'valheim-ui.exe'
$ConfigPath = Join-Path $DataDir 'config.yaml'
$SteamCmdDir = Join-Path $DataDir 'steamcmd'
$SteamCmdZip = 'https://steamcdn-a.akamaihd.net/client/installer/steamcmd.zip'

function Write-Step($msg) { Write-Host "==> $msg" -ForegroundColor Cyan }
function Write-Warn($msg) { Write-Host "warning: $msg" -ForegroundColor Yellow }

# The installed version is read from a marker file, never by executing the
# binary: the service account can write valheim-ui.exe (self-upgrade), so an
# administrator must not run it.
$VersionMarker = Join-Path $InstallDir 'VERSION'
function Get-InstalledVersion {
  if (-not (Test-Path $Exe)) { return $null }
  if (Test-Path $VersionMarker) { return (Get-Content $VersionMarker -Raw).Trim() }
  return 'unknown'
}

function Get-Service-OrNull {
  Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
}

if ($Check) {
  $svc = Get-Service-OrNull
  $ver = Get-InstalledVersion
  Write-Host "binary:   $(if ($ver) { "$Exe ($ver)" } else { 'not installed' })"
  Write-Host "service:  $(if ($svc) { $svc.Status } else { 'not registered' })"
  Write-Host "config:   $(if (Test-Path $ConfigPath) { $ConfigPath } else { 'missing' })"
  Write-Host "steamcmd: $(if (Test-Path (Join-Path $SteamCmdDir 'steamcmd.exe')) { 'present' } else { 'missing' })"
  exit 0
}

if ($Uninstall) {
  $svc = Get-Service-OrNull
  if ($svc) {
    Write-Step "Stopping and removing service $ServiceName"
    if ($svc.Status -ne 'Stopped') { Stop-Service -Name $ServiceName -Force }
    & sc.exe delete $ServiceName | Out-Null
  }
  if (Test-Path $InstallDir) {
    Write-Step "Removing $InstallDir"
    Remove-Item -Recurse -Force $InstallDir
  }
  Write-Host "Data in $DataDir was kept."
  exit 0
}

if (-not [Environment]::Is64BitOperatingSystem) {
  throw 'only 64-bit Windows is supported (the Valheim dedicated server is x64 only)'
}

# ---- resolve and download the release --------------------------------------
Write-Step "Resolving release $Version"
$headers = @{ 'User-Agent' = 'valheim-ui-installer' }
if ($Version -eq 'latest') {
  $rel = Invoke-RestMethod -Headers $headers -Uri "https://api.github.com/repos/$Repo/releases/latest"
} else {
  if ($Version -notmatch '^v\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$') { throw "invalid release tag: $Version" }
  $rel = Invoke-RestMethod -Headers $headers -Uri "https://api.github.com/repos/$Repo/releases/tags/$Version"
}
$tag = $rel.tag_name
$zipAsset = $rel.assets | Where-Object { $_.name -eq 'valheim-ui_windows_amd64.zip' }
$sumsAsset = $rel.assets | Where-Object { $_.name -eq 'SHA256SUMS' }
if (-not $zipAsset) { throw "release $tag has no Windows build (valheim-ui_windows_amd64.zip); releases before v1.4.0 are Linux-only" }
if (-not $sumsAsset) { throw "release $tag has no SHA256SUMS" }

$installed = Get-InstalledVersion
if ($installed -eq $tag) {
  Write-Host "valheim-ui $tag is already installed; refreshing service registration only."
}

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("valheim-ui-install-" + [Guid]::NewGuid().ToString('n'))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
  if ($installed -ne $tag) {
    Write-Step "Downloading $tag"
    $zipPath = Join-Path $tmp 'valheim-ui_windows_amd64.zip'
    $sumsPath = Join-Path $tmp 'SHA256SUMS'
    Invoke-WebRequest -Headers $headers -Uri $zipAsset.browser_download_url -OutFile $zipPath
    Invoke-WebRequest -Headers $headers -Uri $sumsAsset.browser_download_url -OutFile $sumsPath

    Write-Step 'Verifying checksum'
    $expected = (Get-Content $sumsPath | Where-Object { $_ -match '\svalheim-ui_windows_amd64\.zip$' } | Select-Object -First 1)
    if (-not $expected) { throw 'SHA256SUMS has no entry for valheim-ui_windows_amd64.zip' }
    $expected = ($expected -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 $zipPath).Hash.ToLowerInvariant()
    if ($expected -ne $actual) { throw "checksum mismatch for valheim-ui_windows_amd64.zip: expected $expected, got $actual" }

    Expand-Archive -Path $zipPath -DestinationPath (Join-Path $tmp 'unpacked') -Force
    $newExe = Join-Path $tmp 'unpacked\valheim-ui.exe'
    if (-not (Test-Path $newExe)) { throw 'zip does not contain valheim-ui.exe' }
  }

  # ---- directories and permissions -------------------------------------------
  Write-Step "Preparing $InstallDir and $DataDir"
  foreach ($d in @($InstallDir, $DataDir, (Join-Path $DataDir 'instances'), (Join-Path $DataDir 'jobs'), (Join-Path $DataDir 'cache'))) {
    if (-not (Test-Path $d)) { New-Item -ItemType Directory -Path $d | Out-Null }
  }

  # ---- service registration ---------------------------------------------------
  $svc = Get-Service-OrNull
  if ($svc -and $svc.Status -ne 'Stopped') {
    Write-Step "Stopping $ServiceName"
    Stop-Service -Name $ServiceName -Force
    $svc.WaitForStatus('Stopped', [TimeSpan]::FromMinutes(3))
  }

  if ($installed -ne $tag) {
    Write-Step "Installing valheim-ui.exe"
    if (Test-Path $Exe) { Move-Item -Force $Exe "$Exe.prev" }
    Copy-Item -Force $newExe $Exe
    Set-Content -Path $VersionMarker -Value $tag -NoNewline
  }

  if (-not (Test-Path $ConfigPath)) {
    Write-Step "Writing $ConfigPath"
    @"
# valheim-ui configuration (Windows). Every key can be overridden with
# VALHEIM_UI_<UPPERCASE_KEY> in the service environment.

# Address the web UI listens on. Keep it on loopback and put a TLS reverse
# proxy (Caddy, IIS ARR, nginx) in front for remote access.
listen: "$Listen"

# Public URL of the UI. Required for OIDC (redirect URI) and used for the
# CSRF origin check. Example: https://valheim.example.com
base_url: "$BaseUrl"

data_dir: "$($DataDir -replace '\\', '\\')"

# Windows has no systemd: the manager service supervises the game processes.
supervisor: "direct"
steamcmd_path: "$(($SteamCmdDir -replace '\\', '\\') + '\\steamcmd.exe')"

# Set true only when serving plain http on a trusted LAN.
insecure_cookies: false

log_level: "info"
"@ | Set-Content -Encoding UTF8 -Path $ConfigPath
  } elseif ($BaseUrl) {
    Write-Warn "config.yaml exists; -BaseUrl was not applied (edit base_url in $ConfigPath)"
  }

  if (-not $svc) {
    Write-Step "Registering service $ServiceName"
    $binPath = "`"$Exe`" serve --config `"$ConfigPath`""
    New-Service -Name $ServiceName -BinaryPathName $binPath -DisplayName 'Valheim Server UI' `
      -Description 'Web UI managing Valheim dedicated servers' -StartupType Automatic | Out-Null
  } else {
    & sc.exe config $ServiceName binPath= "`"$Exe`" serve --config `"$ConfigPath`"" | Out-Null
  }
  # Virtual service account: no password, its own SID, least privilege.
  & sc.exe sidtype $ServiceName unrestricted | Out-Null
  & sc.exe config $ServiceName obj= $Account | Out-Null
  # A self-upgrade ends the process without reporting SERVICE_STOPPED; the SCM
  # treats that as a failure and these recovery actions restart the manager
  # on the new binary (RUNBOOK.md §11).
  & sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/5000/restart/5000 | Out-Null
  & sc.exe failureflag $ServiceName 1 | Out-Null

  Write-Step "Granting $Account access to $InstallDir and $DataDir"
  # Modify on the install dir lets the manager swap its own binary on upgrade.
  & icacls $InstallDir /grant "${Account}:(OI)(CI)M" /T /Q | Out-Null
  & icacls $DataDir /grant "${Account}:(OI)(CI)M" /T /Q | Out-Null

  # ---- SteamCMD -------------------------------------------------------------
  if (-not $NoSteamCmd -and -not (Test-Path (Join-Path $SteamCmdDir 'steamcmd.exe'))) {
    Write-Step "Downloading SteamCMD into $SteamCmdDir"
    New-Item -ItemType Directory -Path $SteamCmdDir -Force | Out-Null
    $scZip = Join-Path $tmp 'steamcmd.zip'
    Invoke-WebRequest -Headers $headers -Uri $SteamCmdZip -OutFile $scZip
    Write-Host ("steamcmd.zip sha256: " + (Get-FileHash -Algorithm SHA256 $scZip).Hash.ToLowerInvariant())
    Expand-Archive -Path $scZip -DestinationPath $SteamCmdDir -Force
    & icacls $SteamCmdDir /grant "${Account}:(OI)(CI)M" /T /Q | Out-Null
  }

  Write-Step "Starting $ServiceName"
  Start-Service -Name $ServiceName
  (Get-Service -Name $ServiceName).WaitForStatus('Running', [TimeSpan]::FromSeconds(30))
} finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

Write-Host ''
Write-Host "valheim-ui $tag is running as service $ServiceName." -ForegroundColor Green
Write-Host "  UI:      http://$Listen  (first visit creates the admin account)"
Write-Host "  Config:  $ConfigPath"
Write-Host "  Data:    $DataDir"
Write-Host "  Logs:    $(Join-Path $DataDir 'manager.log')"
Write-Host ''
Write-Host 'Open the game ports of each instance (UDP <port> and <port>+1) in Windows Firewall.'
if (-not $BaseUrl -and -not (Test-Path $ConfigPath)) {
  Write-Warn 'base_url is empty: set it in config.yaml before enabling OIDC or serving behind a proxy'
}
