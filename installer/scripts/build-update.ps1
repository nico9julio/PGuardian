param(
    [string]$Version = "0.1.0"
)

$ErrorActionPreference = "Stop"
$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$DistDir = Join-Path $RepoRoot "dist"
$BinDir = Join-Path $DistDir "bin"

New-Item -ItemType Directory -Force $BinDir | Out-Null

$Commit = "dev"
try {
    $gitHash = (git -C $RepoRoot rev-parse --short HEAD 2>$null)
    if ($gitHash) { $Commit = $gitHash.Trim() }
} catch {}
$BuildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

$LdFlagsCommon = "-s -w -X github.com/nico9julio/PGuardian/internal/config.Version=$Version -X github.com/nico9julio/PGuardian/internal/config.Commit=$Commit -X github.com/nico9julio/PGuardian/internal/config.BuildDate=$BuildDate"
$LdFlagsGui = "$LdFlagsCommon -H=windowsgui"

Write-Host "==> [1/4] Building Frontend (React + TypeScript + Tailwind)..." -ForegroundColor Cyan
Push-Location (Join-Path $RepoRoot "frontend")
npm run build
if ($LASTEXITCODE -ne 0) { throw "Frontend build failed" }
Pop-Location

Write-Host "==> [2/4] Building PGuardianService.exe, PGuardianTray.exe, PGuardianUpdater.exe..." -ForegroundColor Cyan
Push-Location $RepoRoot
go build -trimpath -ldflags="$LdFlagsCommon" -o (Join-Path $BinDir "PGuardianService.exe") .\cmd\pguardian-service
if ($LASTEXITCODE -ne 0) { throw "PGuardianService build failed" }

go build -trimpath -ldflags="$LdFlagsGui" -o (Join-Path $BinDir "PGuardianTray.exe") .\cmd\pguardian-tray
if ($LASTEXITCODE -ne 0) { throw "PGuardianTray build failed" }

go build -trimpath -ldflags="$LdFlagsGui" -o (Join-Path $BinDir "PGuardianUpdater.exe") .\cmd\pguardian-updater
if ($LASTEXITCODE -ne 0) { throw "PGuardianUpdater build failed" }

Write-Host "==> [3/4] Building Native Wails App (PGuardian.exe)..." -ForegroundColor Cyan
wails build -clean -ldflags "$LdFlagsCommon" -o PGuardian.exe
if ($LASTEXITCODE -ne 0) { throw "Wails build failed" }
Copy-Item (Join-Path $RepoRoot "build\bin\PGuardian.exe") (Join-Path $BinDir "PGuardian.exe") -Force
Pop-Location

Write-Host "==> [4/4] Creating Signed Update Bundle (PGuardianUpdate_v$Version.zip)..." -ForegroundColor Cyan
$Files = @("PGuardian.exe", "PGuardianService.exe", "PGuardianTray.exe", "PGuardianUpdater.exe")
$FileHashes = [ordered]@{}
foreach ($f in $Files) {
    $p = Join-Path $BinDir $f
    $h = (Get-FileHash -Algorithm SHA256 $p).Hash.ToLower()
    $FileHashes[$f] = $h
}

$ManifestObj = [ordered]@{
    version   = $Version
    commit    = $Commit
    buildDate = $BuildDate
    files     = $FileHashes
}
$ManifestPath = Join-Path $BinDir "manifest.json"
$ManifestJson = $ManifestObj | ConvertTo-Json -Depth 5
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($ManifestPath, $ManifestJson, $Utf8NoBom)

$ZipPath = Join-Path $DistDir "PGuardianUpdate_v$Version.zip"
if (Test-Path $ZipPath) { Remove-Item $ZipPath -Force }

$ToPack = ($Files | ForEach-Object { Join-Path $BinDir $_ }) + @($ManifestPath)
Compress-Archive -Path $ToPack -DestinationPath $ZipPath -CompressionLevel Optimal

Write-Host "Update bundle ready: $ZipPath" -ForegroundColor Green
