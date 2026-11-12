param(
    [Parameter(Mandatory=$true)]
    [string]$Version,
    [string]$Repo = "nico9julio/PGuardian",
    [string]$ReleaseNotes = ""
)

$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$DistDir = Join-Path $RepoRoot "dist"

# Resolve GitHub Token from environment or local Git Credential Manager (never stored in repository)
$Token = $env:GITHUB_TOKEN
if (-not $Token) {
    $credLines = ("protocol=https`nhost=github.com`n" | git credential fill)
    $Token = (($credLines | Where-Object { $_ -match "^password=" }) -replace "^password=", "").Trim()
}
if (-not $Token) {
    throw "No GitHub credentials found. Set `$env:GITHUB_TOKEN or sign in with Git Credential Manager."
}

& (Join-Path $PSScriptRoot "build-all.ps1") -Version $Version

$Tag = "v$Version"
Push-Location $RepoRoot
git tag -f $Tag
git push origin $Tag --force
Pop-Location

$Headers = @{
    Authorization = "Bearer $Token"
    Accept        = "application/vnd.github+json"
    "User-Agent"  = "PGuardian-Release-Script"
}

if (-not $ReleaseNotes) {
    $ReleaseNotes = "PGuardian $Tag — Automated release bundle and Windows Server installer."
}

$CreateBody = @{
    tag_name   = $Tag
    name       = "PGuardian $Tag"
    body       = $ReleaseNotes
    draft      = $false
    prerelease = $false
} | ConvertTo-Json

$Release = Invoke-RestMethod -Method Post -Uri "https://api.github.com/repos/$Repo/releases" -Headers $Headers -Body $CreateBody -ContentType "application/json; charset=utf-8"
$UploadBase = "https://uploads.github.com/repos/$Repo/releases/$($Release.id)/assets"

$Assets = @(
    "PGuardianSetup_v$Version.exe",
    "PGuardianUpdate_v$Version.zip",
    "checksums_v$Version.txt"
)

foreach ($asset in $Assets) {
    $filePath = Join-Path $DistDir $asset
    if (-not (Test-Path $filePath)) { continue }
    Write-Host "Uploading $asset to GitHub Release $Tag..." -ForegroundColor Cyan
    $UploadHeaders = @{
        Authorization  = "Bearer $Token"
        Accept         = "application/vnd.github+json"
        "User-Agent"   = "PGuardian-Release-Script"
        "Content-Type" = "application/octet-stream"
    }
    Invoke-RestMethod -Method Post -Uri "$UploadBase?name=$asset" -Headers $UploadHeaders -InFile $filePath | Out-Null
}

Write-Host "Published GitHub Release: $($Release.html_url)" -ForegroundColor Green
