<#
.SYNOPSIS
    Cross-compile picoclaw for all supported platforms.

.DESCRIPTION
    Builds picoclaw binaries for Linux (amd64, arm64), Windows (amd64),
    and macOS (amd64, arm64). Outputs to the build/ directory.

.PARAMETER Platforms
    Comma-separated list of platforms to build. Default: all.
    Example: -Platforms "linux-amd64,windows-amd64"
#>

param(
    [string]$Platforms = "all"
)

$ErrorActionPreference = "Stop"

$BinaryName = "picoclaw"
$BuildDir = Join-Path (Join-Path $PSScriptRoot "..") "build"
$CmdDir = "cmd/$BinaryName"

# Version from git
$Version = & git describe --tags --always --dirty 2>$null
if (-not $Version) { $Version = "dev" }
$BuildTime = Get-Date -Format "yyyy-MM-ddTHH:mm:sszzz"

$LdFlags = "-X main.version=$Version -X main.buildTime=$BuildTime"

# Platform definitions: GOOS, GOARCH, suffix
$AllPlatforms = @(
    @{ GOOS = "linux"; GOARCH = "amd64"; Suffix = "" },
    @{ GOOS = "linux"; GOARCH = "arm64"; Suffix = "" },
    @{ GOOS = "windows"; GOARCH = "amd64"; Suffix = ".exe" },
    @{ GOOS = "darwin"; GOARCH = "amd64"; Suffix = "" },
    @{ GOOS = "darwin"; GOARCH = "arm64"; Suffix = "" }
)

# Filter platforms if specified
if ($Platforms -ne "all") {
    $requested = $Platforms -split ","
    $AllPlatforms = $AllPlatforms | Where-Object {
        "$($_.GOOS)-$($_.GOARCH)" -in $requested
    }
    if ($AllPlatforms.Count -eq 0) {
        Write-Error "No matching platforms found for: $Platforms"
        exit 1
    }
}

# Ensure build dir exists
if (-not (Test-Path $BuildDir)) {
    New-Item -ItemType Directory -Path $BuildDir -Force | Out-Null
}

Write-Host "Building $BinaryName $Version" -ForegroundColor Cyan
Write-Host ""

$failed = @()

foreach ($p in $AllPlatforms) {
    $outName = "$BinaryName-$($p.GOOS)-$($p.GOARCH)$($p.Suffix)"
    $outPath = Join-Path $BuildDir $outName

    Write-Host "  [$($p.GOOS)/$($p.GOARCH)] " -NoNewline -ForegroundColor Yellow
    Write-Host "$outName ... " -NoNewline

    $env:GOOS = $p.GOOS
    $env:GOARCH = $p.GOARCH
    $env:CGO_ENABLED = "0"

    try {
        & go build -ldflags $LdFlags -o $outPath "./$CmdDir" 2>&1
        if ($LASTEXITCODE -ne 0) { throw "go build exited with code $LASTEXITCODE" }
        $size = [math]::Round((Get-Item $outPath).Length / 1MB, 1)
        Write-Host "OK (${size} MB)" -ForegroundColor Green
    }
    catch {
        Write-Host "FAILED" -ForegroundColor Red
        Write-Host "    $_" -ForegroundColor Red
        $failed += "$($p.GOOS)/$($p.GOARCH)"
    }
    finally {
        Remove-Item Env:\GOOS   -ErrorAction SilentlyContinue
        Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
        Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue
    }
}

Write-Host ""

if ($failed.Count -gt 0) {
    Write-Host "Failed platforms: $($failed -join ', ')" -ForegroundColor Red
    exit 1
}
else {
    Write-Host "All builds complete! Binaries in: $BuildDir" -ForegroundColor Green
    Get-ChildItem $BuildDir -Filter "$BinaryName-*" | ForEach-Object {
        $size = [math]::Round($_.Length / 1MB, 1)
        Write-Host "  $($_.Name)  (${size} MB)" -ForegroundColor Gray
    }
}
