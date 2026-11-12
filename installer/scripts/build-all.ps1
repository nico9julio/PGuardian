param(
    [string]$Version = "0.1.0"
)

$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$DistDir = Join-Path $RepoRoot "dist"

& (Join-Path $PSScriptRoot "build-update.ps1") -Version $Version

Write-Host "==> [5/5] Compiling Windows Installer (Inno Setup)..." -ForegroundColor Cyan
$IsccCandidates = @(
    "iscc.exe",
    "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe",
    "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
    "$env:ProgramFiles\Inno Setup 6\ISCC.exe"
)

$IsccPath = $null
foreach ($c in $IsccCandidates) {
    $cmd = Get-Command $c -ErrorAction SilentlyContinue
    if ($cmd) {
        $IsccPath = $cmd.Source
        break
    }
    if (Test-Path $c) {
        $IsccPath = $c
        break
    }
}

if (-not $IsccPath) {
    throw "Inno Setup (ISCC.exe) not found. Install via: winget install JRSoftware.InnoSetup"
}

$IssFile = Join-Path $RepoRoot "installer\pguardian.iss"
& $IsccPath "/DAppVersion=$Version" $IssFile
if ($LASTEXITCODE -ne 0) { throw "Inno Setup compilation failed" }

$ChecksumFile = Join-Path $DistDir "checksums_v$Version.txt"
$Lines = @()
foreach ($artifact in @("PGuardianSetup_v$Version.exe", "PGuardianUpdate_v$Version.zip")) {
    $ap = Join-Path $DistDir $artifact
    if (Test-Path $ap) {
        $hash = (Get-FileHash -Algorithm SHA256 $ap).Hash.ToLower()
        $Lines += "$hash  $artifact"
    }
}
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllLines($ChecksumFile, $Lines, $Utf8NoBom)

Write-Host "`nAll artifacts built successfully in $DistDir :" -ForegroundColor Green
Get-ChildItem $DistDir -File | Select-Object Name, Length
