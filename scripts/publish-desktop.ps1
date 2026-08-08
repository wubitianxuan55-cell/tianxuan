#!/usr/bin/env pwsh
# One-shot desktop release: build the Windows NSIS installer, sign it with
# minisign, generate latest.json, and (unless -SkipUpload) publish a GitHub
# release. This is the local publish path for the Windows-only channel; the CI
# workflow (.github/workflows/release-desktop.yml) is the cross-platform path.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts\publish-desktop.ps1 -Version v10.155.0
#   # ...or omit -Version to take the top entry of release/CHANGELOG.md
#
# Preconditions:
#   - NSIS (makensis) on PATH or installed at C:\Program Files (x86)\NSIS
#   - minisign signing key at %USERPROFILE%\.tianxuan-release\ (tianxuan.key +
#     minisign.password), or MINISIGN_PRIVATE_KEY / MINISIGN_PASSWORD env vars
#   - `gh` installed and authenticated for uploads (gh auth login)
param(
    [string]$Version = "",
    [switch]$SkipTests,
    [switch]$SkipUpload
)
$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent $PSScriptRoot
$Desktop = Join-Path $RepoRoot "tianxuan\desktop"
$ReleaseDirRoot = Join-Path $RepoRoot "release"
$KeyDir = Join-Path $env:USERPROFILE ".tianxuan-release"

function Fail([string]$msg) { Write-Error $msg; exit 1 }

if (-not (Test-Path (Join-Path $RepoRoot ".git"))) { Fail "RepoRoot is not a git repo: $RepoRoot" }
if (-not (Test-Path $Desktop)) { Fail "desktop dir missing: $Desktop" }

# Project-local toolchains (same as build-desktop.bat): the frontend compile
# needs node; keep the local Go ahead of the system one so both stay pinned.
$ToolsDir = Join-Path $RepoRoot "tools"
if (Test-Path (Join-Path $ToolsDir "go\bin\go.exe")) { $env:Path = "$ToolsDir\go\bin;$env:Path" }
if (Test-Path (Join-Path $ToolsDir "node\node.exe")) { $env:Path = "$ToolsDir\node;$env:Path" }

# ---- resolve version -------------------------------------------------------
if ($Version -eq "") {
    $changelog = Join-Path $ReleaseDirRoot "CHANGELOG.md"
    if (-not (Test-Path $changelog)) { Fail "release\CHANGELOG.md not found; pass -Version explicitly" }
    $m = [regex]::Match((Get-Content $changelog -Raw -Encoding UTF8), "##\s+(v\d+\.\d+\.\d+)")
    if (-not $m.Success) { Fail "no version entry found in release\CHANGELOG.md" }
    $Version = $m.Groups[1].Value
}
if ($Version -notmatch "^v\d+\.\d+\.\d+$") { Fail "version must look like v10.X.Y, got: $Version" }
$NumericVersion = $Version.TrimStart("v")
$Tag = "desktop-$Version"
$ReleaseDir = Join-Path $ReleaseDirRoot $Version
$InstallerName = "tianxuan-windows-amd64-installer.exe"
$Installer = Join-Path $ReleaseDir $InstallerName
$GitHubRepo = "wubitianxuan55-cell/tianxuan"

Write-Host "==> Publish $Version (tag $Tag) to $GitHubRepo"

# ---- NSIS / toolchain on PATH ---------------------------------------------
$nsisDir = "C:\Program Files (x86)\NSIS"
if (-not (Get-Command makensis -ErrorAction SilentlyContinue) -and (Test-Path (Join-Path $nsisDir "makensis.exe"))) {
    $env:Path = "$nsisDir;$env:Path"
}
if (-not (Get-Command makensis -ErrorAction SilentlyContinue)) { Fail "makensis not found; install NSIS (winget install NSIS.NSIS)" }
foreach ($tool in @("go", "wails")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { Fail "$tool not found on PATH" }
}

# ---- signing key -----------------------------------------------------------
if ([string]::IsNullOrEmpty($env:MINISIGN_PRIVATE_KEY)) {
    $keyFile = Join-Path $KeyDir "tianxuan.key"
    if (-not (Test-Path $keyFile)) { Fail "MINISIGN_PRIVATE_KEY not set and $keyFile missing" }
    $env:MINISIGN_PRIVATE_KEY = Get-Content $keyFile -Raw
}
if ([string]::IsNullOrEmpty($env:MINISIGN_PASSWORD)) {
    $pwFile = Join-Path $KeyDir "minisign.password"
    if (-not (Test-Path $pwFile)) { Fail "MINISIGN_PASSWORD not set and $pwFile missing" }
    $env:MINISIGN_PASSWORD = Get-Content $pwFile -Raw
}

# ---- tests -----------------------------------------------------------------
if (-not $SkipTests) {
    Write-Host "==> go test ./... (desktop module)"
    Push-Location $Desktop
    go test ./...
    if ($LASTEXITCODE -ne 0) { Pop-Location; Fail "go test failed" }
    Pop-Location
}

# ---- sync bundled skills (same as build-desktop.bat) -----------------------
# The kernel embeds internal/skill/bundled from the ROOT module; desktop imports
# the kernel, so the sync target is tianxuan/internal/skill/bundled.
Write-Host "==> sync .tianxuan/skills -> tianxuan/internal/skill/bundled"
robocopy (Join-Path $RepoRoot ".tianxuan\skills") (Join-Path $RepoRoot "tianxuan\internal\skill\bundled") /E /NJH /NJS /NFL | Out-Null
if ($LASTEXITCODE -ge 8) { Fail "skills sync failed (robocopy exit $LASTEXITCODE)" }

# ---- build installer -------------------------------------------------------
$WailsJson = Join-Path $Desktop "wails.json"
$OriginalBytes = [System.IO.File]::ReadAllBytes($WailsJson)
$Latin1 = [System.Text.Encoding]::GetEncoding(28591)
try {
    $text = $Latin1.GetString($OriginalBytes)
    if ($text -notmatch '"productName"\s*:\s*"tianxuan"') { Fail "unexpected wails.json: productName not found" }
    $needle = '"productName": "tianxuan",'
    $injectLine = '    "productVersion": "' + $NumericVersion + '",'
    $injected = $text.Replace($needle, $needle + [Environment]::NewLine + $injectLine)
    [System.IO.File]::WriteAllBytes($WailsJson, $Latin1.GetBytes($injected))
    Write-Host "==> wails build -nsis (productVersion $NumericVersion, version $Version)"
    Push-Location $Desktop
    wails build -clean -nsis -ldflags "-X main.version=$Version" -o tianxuan-desktop.exe
    $buildCode = $LASTEXITCODE
    Pop-Location
    if ($buildCode -ne 0) { Fail "wails build failed (exit $buildCode)" }
}
finally {
    [System.IO.File]::WriteAllBytes($WailsJson, $OriginalBytes)
}

$Built = Get-ChildItem (Join-Path $Desktop "build\bin") -Filter "*installer*.exe" -ErrorAction SilentlyContinue |
    Sort-Object LastWriteTime -Descending | Select-Object -First 1
if (-not $Built) { Fail "no NSIS installer produced in desktop\build\bin" }

# ---- stage into release/<version>/ -----------------------------------------
New-Item -ItemType Directory -Path $ReleaseDir -Force | Out-Null
Copy-Item -LiteralPath $Built.FullName -Destination $Installer -Force
$Hash = (Get-FileHash -LiteralPath $Installer -Algorithm SHA256).Hash.ToLower()
$Size = (Get-Item -LiteralPath $Installer).Length
Set-Content -LiteralPath (Join-Path $ReleaseDir "SHA256SUMS") -Value "$Hash  $InstallerName" -Encoding ascii
Write-Host "==> staged $InstallerName ($([math]::Round($Size/1MB,1)) MB, sha256 $Hash)"

# ---- sign + manifest -------------------------------------------------------
Push-Location $Desktop
Write-Host "==> minisign sign"
go run ./cmd/sign sign "$Installer"
if ($LASTEXITCODE -ne 0) { Pop-Location; Fail "sign failed" }

Write-Host "==> generate latest.json"
$env:GITHUB_REPOSITORY = $GitHubRepo
go run ./cmd/sign manifest "$ReleaseDir" $Version $Tag
if ($LASTEXITCODE -ne 0) { Pop-Location; Fail "manifest generation failed" }
Pop-Location

# fill release notes from release/CHANGELOG.md top entry (line-based, so it
# works regardless of line endings / non-ASCII separators)
$changelogPath = Join-Path $ReleaseDirRoot "CHANGELOG.md"
if (Test-Path $changelogPath) {
    $lines = Get-Content $changelogPath -Encoding UTF8
    $start = -1
    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match "^##\s+v\d+\.\d+\.\d+") { $start = $i; break }
    }
    if ($start -ge 0) {
        $body = @()
        for ($i = $start + 1; $i -lt $lines.Count; $i++) {
            if ($lines[$i] -match "^---\s*$") { break }
            $body += $lines[$i]
        }
        $notes = ($body -join "`n").Trim()
    }
    if ($notes) {
        $manifestPath = Join-Path $ReleaseDir "latest.json"
        # -Encoding UTF8 is required: Windows PowerShell 5.1's default reads
        # no-BOM UTF-8 as ANSI, mangling the notes and breaking ConvertFrom-Json.
        $j = Get-Content $manifestPath -Raw -Encoding UTF8 | ConvertFrom-Json
        $j.notes = $notes
        # No-BOM UTF-8: Windows PowerShell 5.1's Set-Content -Encoding utf8 emits
        # a BOM, which Go's json.Unmarshal (the installed updater) rejects.
        $jsonText = $j | ConvertTo-Json -Depth 6
        [System.IO.File]::WriteAllText($manifestPath, $jsonText, (New-Object System.Text.UTF8Encoding($false)))
        Write-Host "==> latest.json notes filled from release/CHANGELOG.md"
    } else {
        Write-Host "==> WARN: no release/CHANGELOG.md entry found; latest.json notes left empty"
    }
}

# ---- verify ----------------------------------------------------------------
Write-Host "==> verify minisign signature against embedded public key"
Push-Location $Desktop
go run ./cmd/sign verify "$Installer"
if ($LASTEXITCODE -ne 0) { Pop-Location; Fail "signature verification failed" }
Pop-Location

$manifest = Get-Content (Join-Path $ReleaseDir "latest.json") -Raw -Encoding UTF8 | ConvertFrom-Json
$asset = $manifest.platforms."windows-amd64"
if (-not $asset -or $asset.sha256 -ne $Hash) {
    Fail "manifest sha256 mismatch: got $($asset.sha256), want $Hash"
}
Write-Host "==> manifest OK: version $($manifest.version), sha256 matches"

Write-Host ""
Write-Host "==> local artifacts ready in $ReleaseDir"
Write-Host "    $InstallerName  ($([math]::Round($Size/1MB,1)) MB)"
Write-Host "    SHA256: $Hash"

# ---- publish to GitHub -----------------------------------------------------
if (-not $SkipUpload) {
    $gh = Get-Command gh -ErrorAction SilentlyContinue
    if (-not $gh) { $gh = Get-Command "C:\Program Files\GitHub CLI\gh.exe" -ErrorAction SilentlyContinue }
    if (-not $gh) { Fail "gh not found; install GitHub CLI (winget install GitHub.cli)" }
    & $gh.Source auth status *> $null
    if ($LASTEXITCODE -ne 0) { Fail "gh not authenticated; run: gh auth login" }

    git tag $Tag 2>$null | Out-Null
    $notes = if ($manifest.notes) { $manifest.notes } else { "Tianxuan Desktop $Version" }
    $notesFile = Join-Path $env:TEMP "tianxuan-release-notes-$Version.md"
    Set-Content -LiteralPath $notesFile -Value $notes -Encoding utf8

    Write-Host "==> gh release create $Tag"
    & $gh.Source release create $Tag `
        (Join-Path $ReleaseDir $InstallerName) `
        (Join-Path $ReleaseDir "$InstallerName.minisig") `
        (Join-Path $ReleaseDir "latest.json") `
        (Join-Path $ReleaseDir "SHA256SUMS") `
        --repo $GitHubRepo `
        --title "Tianxuan Desktop $Version" `
        --notes-file $notesFile
    if ($LASTEXITCODE -ne 0) { Fail "gh release create failed" }
    Remove-Item -LiteralPath $notesFile -Force -ErrorAction SilentlyContinue
    Write-Host "==> published: https://github.com/$GitHubRepo/releases/tag/$Tag"
}

Write-Host "==> done"
