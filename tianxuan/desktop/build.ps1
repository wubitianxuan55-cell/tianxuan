#!/usr/bin/env pwsh
# tianxuan 桌面端构建脚本 — 固定输出到项目根 bin/ 目录
param(
    [string]$Version = "dev"
)
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$bin = Resolve-Path "build/bin"
New-Item -ItemType Directory -Force -Path $bin | Out-Null

$out = Join-Path $bin "tianxuan-desktop.exe"
# wails build -o is relative to desktop/build/bin/, so use bare filename
Write-Host "Building tianxuan-desktop $Version → $out"

Push-Location $PSScriptRoot
wails build -ldflags "-X main.version=$Version" -o tianxuan-desktop.exe
Pop-Location
if ($LASTEXITCODE -ne 0) { throw "wails build failed" }

$hash = (Get-FileHash $out -Algorithm SHA256).Hash.ToLower()
$size = (Get-Item $out).Length / 1MB
Write-Host "Done: $([math]::Round($size,0))MB · SHA256: $hash"
