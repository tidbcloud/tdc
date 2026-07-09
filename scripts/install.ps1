param(
    [string]$Version = "latest",
    [string]$InstallDir = "",
    [switch]$DryRun,
    [switch]$Yes
)

$ErrorActionPreference = "Stop"
$Repo = "tidbcloud/tdc"
$DefaultInstallDir = Join-Path $HOME "bin"

function Fail($Message) {
    Write-Error "tdc install [ERROR]: $Message"
    exit 1
}

function Info($Message) {
    Write-Output "  $Message"
}

function Warn($Message) {
    Write-Warning $Message
}

function Resolve-InstallDir {
    if (-not [string]::IsNullOrWhiteSpace($InstallDir)) {
        Info "Install dir: $InstallDir (from -InstallDir)"
        return $InstallDir
    }
    if (-not [string]::IsNullOrWhiteSpace($env:TDC_INSTALL_DIR)) {
        Info "Install dir: $env:TDC_INSTALL_DIR (from TDC_INSTALL_DIR)"
        return $env:TDC_INSTALL_DIR
    }

    $existing = Get-Command tdc -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($existing -and $existing.Source) {
        $existingDir = Split-Path -Parent $existing.Source
        Info "Upgrading active tdc in $existingDir"
        return $existingDir
    } else {
        Info "Install dir: $DefaultInstallDir"
    }
    return $DefaultInstallDir
}

function Bootstrap-Config {
    if ([string]::IsNullOrWhiteSpace($HOME)) {
        return
    }
    $ConfigDir = Join-Path $HOME ".tdc"
    $ConfigFile = Join-Path $ConfigDir "config"
    New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null
    if (-not (Test-Path $ConfigFile)) {
        @"
[default]
cloud_provider = 'aws'
region_code = 'us-east-1'
"@ | Set-Content -Path $ConfigFile -NoNewline
        Info "Bootstrapped $ConfigFile with default aws/us-east-1 placement"
    }
}

function Report-PathStatus {
    $active = Get-Command tdc -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $active -or -not $active.Source) {
        Warn "tdc is installed at $Target, but $InstallDir is not on your PATH"
        Warn "Run $Target directly or add $InstallDir to PATH"
        Warn "PowerShell: `$env:Path = `"$InstallDir;`$env:Path`""
        return
    }
    if ($active.Source -ne $Target) {
        Warn "PATH shadowing detected: tdc resolves to $($active.Source)"
        Warn "Installed binary: $Target"
        Warn "Re-run with TDC_INSTALL_DIR=$(Split-Path -Parent $active.Source) to replace the active binary"
    }
}

function Print-Regions {
    Write-Output ""
    Write-Output "  Config regions:"
    Write-Output "    aws: us-east-1, us-west-2, eu-central-1, ap-northeast-1, ap-southeast-1"
    Write-Output "    alibaba_cloud: ap-southeast-1"
    Write-Output ""
    Write-Output "  tdc fs regions:"
    try {
        $manifest = Invoke-RestMethod -Uri "https://drive9.ai/manifest/regions/drive9-regions.json"
        $regions = @($manifest.regions | Where-Object { $_.mode -eq "tidb_cloud_native" } | ForEach-Object { "    $($_.cloud_provider): $($_.tidb_region)" } | Sort-Object -Unique)
        if ($regions.Count -gt 0) {
            $regions | ForEach-Object { Write-Output $_ }
            return
        }
    } catch {
    }
    Write-Output "    aws: us-east-1, ap-southeast-1"
    Warn "Could not fetch the latest tdc fs region manifest; run tdc fs check-file-system after configure"
}

function Print-NextSteps {
    Write-Output ""
    Write-Output "  Get started:"
    Write-Output ""
    Write-Output "    1. Configure credentials"
    Write-Output "       tdc configure"
    Write-Output ""
    Write-Output "    2. List projects"
    Write-Output "       tdc organization list-projects --output human"
    Write-Output ""
    Write-Output "    3. Create or check tdc fs"
    Write-Output "       tdc fs create-file-system --file-system-name workspace"
    Write-Output "       tdc fs check-file-system --output human"
    Write-Output ""
    Write-Output "    4. Mount tdc fs when FUSE is available"
    Write-Output "       tdc fs mount-file-system --file-system-name workspace --mount-path ./workspace"
    Write-Output ""
    Write-Output "  Docs: https://github.com/tidbcloud/tdc"
}

$arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
if ($arch -ne [System.Runtime.InteropServices.Architecture]::X64) {
    Fail "unsupported Windows architecture: $arch"
}

$InstallDir = Resolve-InstallDir

if ($Version -eq "latest") {
    $ReleaseBase = "https://github.com/$Repo/releases/latest/download"
} else {
    if ($Version.StartsWith("v")) {
        $Tag = $Version
    } else {
        $Tag = "v$Version"
    }
    $ReleaseBase = "https://github.com/$Repo/releases/download/$Tag"
}

$Artifact = "tdc_windows_amd64.zip"
$ArchiveUrl = "$ReleaseBase/$Artifact"
$ChecksumsUrl = "$ReleaseBase/tdc_checksums.txt"
$Target = Join-Path $InstallDir "tdc.exe"

if ($DryRun) {
    Write-Output "tdc install dry-run"
    Write-Output "version: $Version"
    Write-Output "artifact: $Artifact"
    Write-Output "archive_url: $ArchiveUrl"
    Write-Output "checksums_url: $ChecksumsUrl"
    Write-Output "target: $Target"
    exit 0
}

if ((Test-Path $Target) -and -not $Yes) {
    $answer = Read-Host "Replace existing $Target? [y/N]"
    if ($answer -notin @("y", "Y", "yes", "YES")) {
        Write-Error "tdc install [ERROR]: cancelled"
        exit 130
    }
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$TempDir = New-Item -ItemType Directory -Path (Join-Path ([System.IO.Path]::GetTempPath()) ("tdc-install-" + [System.Guid]::NewGuid().ToString()))

try {
    $ArchivePath = Join-Path $TempDir.FullName $Artifact
    $ChecksumsPath = Join-Path $TempDir.FullName "tdc_checksums.txt"
    Invoke-WebRequest -Uri $ArchiveUrl -OutFile $ArchivePath
    Invoke-WebRequest -Uri $ChecksumsUrl -OutFile $ChecksumsPath

    $checksumLine = Get-Content $ChecksumsPath | Where-Object { $_ -match "\s+$([regex]::Escape($Artifact))$" } | Select-Object -First 1
    if (-not $checksumLine) {
        Fail "checksum for $Artifact not found"
    }
    $Expected = ($checksumLine -split "\s+")[0].ToLowerInvariant()
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $ArchivePath).Hash.ToLowerInvariant()
    if ($Expected -ne $Actual) {
        Fail "checksum mismatch for $Artifact"
    }

    Expand-Archive -Path $ArchivePath -DestinationPath $TempDir.FullName -Force
    $Extracted = Get-ChildItem -Path $TempDir.FullName -Recurse -Filter "tdc.exe" | Select-Object -First 1
    if (-not $Extracted) {
        Fail "archive did not contain tdc.exe"
    }

    Move-Item -Force -Path $Extracted.FullName -Destination $Target
    & $Target --version
    Write-Output "tdc installed to $Target"
    Bootstrap-Config
    Report-PathStatus
    Print-Regions
    Print-NextSteps
} finally {
    Remove-Item -Recurse -Force $TempDir.FullName -ErrorAction SilentlyContinue
}
