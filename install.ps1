# Install the ocr (Open Code Review) CLI from GitHub releases on Windows.
#   irm https://raw.githubusercontent.com/alibaba/open-code-review/main/install.ps1 | iex
# Prefer to inspect first:
#   irm https://raw.githubusercontent.com/alibaba/open-code-review/main/install.ps1 -OutFile install.ps1
#   notepad install.ps1   # review, then: .\install.ps1
# Env: OCR_INSTALL_DIR (default $env:LOCALAPPDATA\Programs\ocr), OCR_VERSION (default latest).
# Requires PowerShell 5.1+ or PowerShell 7+.

$ErrorActionPreference = 'Stop'

function Err([string]$Message) {
    [Console]::Error.WriteLine("error: $Message")
    exit 1
}

function Get-OcrArch {
    $arch = $env:PROCESSOR_ARCHITECTURE
    if ([string]::IsNullOrEmpty($arch)) {
        Err 'unable to detect architecture (PROCESSOR_ARCHITECTURE is empty); please set it manually'
    }
    switch -Regex ($arch) {
        '^(AMD64|X64|x86_64)$' { return 'amd64' }
        '^(ARM64|aarch64)$' { return 'arm64' }
        default { Err "unsupported architecture: $arch (only amd64 and arm64 are supported)" }
    }
}

function Resolve-OcrVersion([string]$Repo) {
    $version = $env:OCR_VERSION
    if (-not [string]::IsNullOrWhiteSpace($version)) {
        return $version.Trim()
    }
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
    } catch {
        Err "failed to fetch latest release info from github api"
    }
    if (-not $release.tag_name) {
        Err 'could not resolve latest release tag'
    }
    return [string]$release.tag_name
}

function Get-ChecksumFromFile([string]$ChecksumFile, [string]$AssetName) {
    foreach ($line in Get-Content -LiteralPath $ChecksumFile) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line.Trim() -split '\s+', 2
        if ($parts.Count -eq 2 -and $parts[1] -eq $AssetName) {
            return $parts[0].ToLowerInvariant()
        }
    }
    return $null
}

function Install-OcrBinary([string]$Source, [string]$InstallDir, [string]$BinName) {
    try {
        New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
        $dest = Join-Path $InstallDir $BinName
        Copy-Item -LiteralPath $Source -Destination $dest -Force
    } catch {
        Err "$InstallDir is not writable; set OCR_INSTALL_DIR to a writable path"
    }
}

function Show-PostInstallPathNotice([string]$BinName, [string]$InstallDir) {
    $pathEntries = $env:PATH -split ';' | ForEach-Object { $_.TrimEnd('\') }
    $normalizedInstall = $InstallDir.TrimEnd('\')
    $onPath = $false
    foreach ($entry in $pathEntries) {
        if ($entry -and [string]::Equals($entry, $normalizedInstall, [System.StringComparison]::OrdinalIgnoreCase)) {
            $onPath = $true
            break
        }
    }
    if (-not $onPath) {
        Write-Host "note: $InstallDir is not on your PATH; add it or run $InstallDir\$BinName directly"
        return
    }
    if (-not (Get-Command $BinName -ErrorAction SilentlyContinue)) {
        Write-Host "note: open a new shell so $BinName resolves on PATH"
    }
}

# Ensure TLS 1.2 for Windows PowerShell 5.1 (Invoke-WebRequest / Invoke-RestMethod).
try {
    [Net.ServicePointManager]::SecurityProtocol = `
        [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
} catch {
    # Ignore if the runtime already negotiates modern TLS.
}

$Repo = 'alibaba/open-code-review'
$Bin = 'ocr.exe'
$AssetPrefix = 'opencodereview'
$DefaultInstallDir = Join-Path $env:LOCALAPPDATA 'Programs\ocr'
$InstallDir = if (-not [string]::IsNullOrWhiteSpace($env:OCR_INSTALL_DIR)) {
    $env:OCR_INSTALL_DIR.Trim()
} else {
    $DefaultInstallDir
}

$arch = Get-OcrArch
$os = 'windows'
$Version = Resolve-OcrVersion $Repo
$asset = "$AssetPrefix-$os-$arch.exe"
$base = "https://github.com/$Repo/releases/download/$Version"

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("ocr-install-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null

try {
    $assetPath = Join-Path $tmp $asset
    $sumPath = Join-Path $tmp 'sha256sum.txt'

    Write-Host "downloading $Bin $Version ($os/$arch)..."
    try {
        Invoke-WebRequest -Uri "$base/$asset" -OutFile $assetPath -UseBasicParsing
    } catch {
        Err "download failed: $base/$asset"
    }
    try {
        Invoke-WebRequest -Uri "$base/sha256sum.txt" -OutFile $sumPath -UseBasicParsing
    } catch {
        Err 'sha256sum.txt download failed'
    }

    $want = Get-ChecksumFromFile $sumPath $asset
    if ([string]::IsNullOrEmpty($want)) {
        Err "no checksum entry for $asset in sha256sum.txt"
    }
    $got = (Get-FileHash -LiteralPath $assetPath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($got -ne $want) {
        Err "checksum mismatch for $asset (got $got, want $want)"
    }

    Install-OcrBinary $assetPath $InstallDir $Bin

    Write-Host "installed $Bin $Version -> $InstallDir\$Bin"
    Show-PostInstallPathNotice $Bin $InstallDir
} finally {
    Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
