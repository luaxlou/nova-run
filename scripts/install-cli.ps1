[CmdletBinding()]
param(
    [string]$Version = $(if ($env:NOVA_VERSION) { $env:NOVA_VERSION } else { "latest" }),
    [string]$InstallDir = $(if ($env:NOVA_INSTALL_DIR) { $env:NOVA_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\Nova" }),
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$Repository = if ($env:NOVA_REPO) { $env:NOVA_REPO } else { "luaxlou/nova-run" }

switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { $Architecture = "amd64" }
    "ARM64" { $Architecture = "arm64" }
    default { throw "Unsupported Windows architecture: $($env:PROCESSOR_ARCHITECTURE)" }
}

$ReleaseTag = $Version
if ($Version -eq "latest") {
    $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repository/releases/latest"
    $ReleaseTag = $Release.tag_name
}
if ([string]::IsNullOrWhiteSpace($ReleaseTag)) {
    throw "Unable to determine the Nova release version."
}

$TargetVersion = $ReleaseTag.TrimStart("v")
$Target = Join-Path $InstallDir "nova.exe"
if (-not $Force -and (Test-Path $Target)) {
    $LocalOutput = & $Target version 2>$null
    if ($LASTEXITCODE -eq 0 -and $LocalOutput -match '^nova\s+v?(.+)$' -and $Matches[1] -eq $TargetVersion) {
        Write-Host "nova $TargetVersion is already installed at $Target; no update needed."
        return
    }
}

$Asset = "nova-windows-$Architecture.exe"
$BaseUrl = "https://github.com/$Repository/releases/download/$ReleaseTag"
$TemporaryDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ("nova-install-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $TemporaryDirectory | Out-Null
try {
    $DownloadedBinary = Join-Path $TemporaryDirectory $Asset
    $DownloadedChecksums = Join-Path $TemporaryDirectory "SHA256SUMS.txt"
    Write-Host "Downloading $BaseUrl/$Asset"
    Invoke-WebRequest -Uri "$BaseUrl/$Asset" -OutFile $DownloadedBinary
    Invoke-WebRequest -Uri "$BaseUrl/SHA256SUMS.txt" -OutFile $DownloadedChecksums

    $ChecksumLine = Get-Content $DownloadedChecksums | Where-Object { $_ -match "\s+$([regex]::Escape($Asset))$" } | Select-Object -First 1
    if (-not $ChecksumLine) {
        throw "SHA256 checksum for $Asset is missing from the release."
    }
    $ExpectedHash = ($ChecksumLine -split '\s+')[0].ToUpperInvariant()
    $ActualHash = (Get-FileHash -Path $DownloadedBinary -Algorithm SHA256).Hash
    if ($ActualHash -ne $ExpectedHash) {
        throw "SHA256 checksum verification failed for $Asset."
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Move-Item -Force -Path $DownloadedBinary -Destination $Target
}
finally {
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $TemporaryDirectory
}

$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
$PathEntries = @($UserPath -split ';' | Where-Object { $_ })
if ($PathEntries -notcontains $InstallDir) {
    $UpdatedPath = (($PathEntries + $InstallDir) -join ';')
    [Environment]::SetEnvironmentVariable("Path", $UpdatedPath, "User")
    $env:Path = "$env:Path;$InstallDir"
    Write-Host "Added $InstallDir to the user PATH. Open a new terminal to use it everywhere."
}

Write-Host "nova $TargetVersion installed to $Target"
