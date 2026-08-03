param(
  [Parameter(Mandatory = $true)][string]$Version,
  [string]$RepoRoot = "",
  [string]$ExePath = ""
)

$ErrorActionPreference = 'Stop'

# ---- 内置排除清单 ----
# 复制目录时才生效;单 exe 发布天然无冗余,保留清单以防未来扩展到目录打包。
$ExcludeDirs = @('.git', 'node_modules', 'build', '.vite', 'dist', '.codegraph', '__pycache__')
$ExcludeFiles = @('*.log', '*.tmp', '*.bak')

if ($Version -notmatch '^v\d+\.\d+\.\d+$') {
  throw "版本号格式应为 v10.X.Y, 收到: $Version"
}

# 解析仓库根:脚本位于 <root>/.tianxuan/skills/release/scripts/package.ps1
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
if ($RepoRoot -eq '') {
  $RepoRoot = $ScriptDir
  1..4 | ForEach-Object { $RepoRoot = Split-Path -Parent $RepoRoot }
}
if (-not (Test-Path (Join-Path $RepoRoot '.git'))) {
  throw "RepoRoot 不是 git 仓库根: $RepoRoot"
}

# ---- 定位桌面端产物 ----
$Candidates = @()
if ($ExePath -ne '') { $Candidates += $ExePath }
$Candidates += @(
  (Join-Path $RepoRoot 'tianxuan\desktop\build\bin\tianxuan-desktop.exe'),
  (Join-Path $RepoRoot 'tianxuan\tianxuan-desktop.exe'),
  (Join-Path $RepoRoot 'tianxuan\tianxuan.exe')
)
$Exe = $Candidates | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $Exe) {
  # 兜底:release/ 下最近版本的产物
  $Exe = Get-ChildItem (Join-Path $RepoRoot 'release') -Recurse -Filter 'tianxuan-desktop.exe' -ErrorAction SilentlyContinue |
    Sort-Object LastWriteTime -Descending | Select-Object -First 1 -ExpandProperty FullName
}
if (-not $Exe) {
  throw "未找到 tianxuan-desktop.exe, 请先构建 (build-desktop.bat) 或传 -ExePath"
}

# ---- 复制到版本目录 + SHA256 ----
$TargetDir = Join-Path $RepoRoot ("release\" + $Version)
New-Item -ItemType Directory -Path $TargetDir -Force | Out-Null

$TargetExe = Join-Path $TargetDir 'tianxuan-desktop.exe'
Copy-Item -LiteralPath $Exe -Destination $TargetExe -Force

$Hash = (Get-FileHash -LiteralPath $TargetExe -Algorithm SHA256).Hash.ToLower()
$Size = (Get-Item -LiteralPath $TargetExe).Length
$SumsLine = "$Hash  tianxuan-desktop.exe"
Set-Content -LiteralPath (Join-Path $TargetDir 'SHA256SUMS') -Value $SumsLine -Encoding ascii

# ---- 摘要(供 agent 直接写入 CHANGELOG / release notes) ----
$SizeMB = [Math]::Round($Size / 1MB, 1)
Write-Output "VERSION=$Version"
Write-Output "EXE=$TargetExe"
Write-Output "BYTES=$Size"
Write-Output "SIZE_MB=$SizeMB"
Write-Output "SHA256=$Hash"
Write-Output "SOURCE=$Exe"
