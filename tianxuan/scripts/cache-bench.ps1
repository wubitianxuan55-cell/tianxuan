# V1.4 vs V1.5 cache hit rate comparison
param(
    [string]$Model = "deepseek-v4-flash",
    [int]$Budget = 200000,
    [string]$OutDir = "benchmarks/cache-results"
)

$ErrorActionPreference = "Stop"
$root = Resolve-Path (Split-Path -Parent $PSScriptRoot)

Write-Host "tianxuan V1.4 vs V1.5 cache benchmark" -ForegroundColor Green
Write-Host "Model: $Model" -ForegroundColor Green
Write-Host "============================================" -ForegroundColor Green

# ---- Build V1.4 via git worktree (isolated) ----
$v14Worktree = "$env:TEMP/tianxuan-v14-bench"
$v14Bin = "$root/bin/tianxuan-v1.4.exe"
$v15Bin = "$root/bin/tianxuan-v1.5.exe"

Write-Host ""
Write-Host "[1/4] Building V1.4 (git worktree)..." -ForegroundColor Cyan
if (Test-Path $v14Worktree) { Remove-Item -Recurse -Force $v14Worktree }
cmd /c "git -C $root worktree add --detach $v14Worktree v1.4.0 2>nul"
try {
    Push-Location $v14Worktree
    go build -ldflags "-s -w" -o $v14Bin ./cmd/tianxuan
    if ($LASTEXITCODE -ne 0) { throw "V1.4 build failed" }
    Write-Host "  V1.4: $v14Bin" -ForegroundColor Green
} finally {
    Pop-Location
    cmd /c "git -C $root worktree remove $v14Worktree --force 2>nul"
}

Write-Host ""
Write-Host "[2/4] Building V1.5 (current workspace)..." -ForegroundColor Cyan
Push-Location $root
try {
    go build -ldflags "-s -w" -o $v15Bin ./cmd/tianxuan
    if ($LASTEXITCODE -ne 0) { throw "V1.5 build failed" }
    Write-Host "  V1.5: $v15Bin" -ForegroundColor Green
} finally {
    Pop-Location
}

# ---- Run e2ebench ----
$resultsDir = "$root/$OutDir"
New-Item -ItemType Directory -Force -Path $resultsDir | Out-Null

Write-Host ""
Write-Host "[3/4] Running V1.4 benchmark..." -ForegroundColor Cyan
Push-Location $root
$v14Out = "$resultsDir/v1.4.json"
try {
    & $v14Bin e2ebench --suite benchmarks/e2e --json $v14Out --model $Model --budget $Budget
    if ($LASTEXITCODE -ne 0) { Write-Host "  WARNING: some tasks failed" -ForegroundColor Yellow }
} finally {
    Pop-Location
}

Write-Host ""
Write-Host "[4/4] Running V1.5 benchmark..." -ForegroundColor Cyan
Push-Location $root
$v15Out = "$resultsDir/v1.5.json"
try {
    & $v15Bin e2ebench --suite benchmarks/e2e --json $v15Out --model $Model --budget $Budget
    if ($LASTEXITCODE -ne 0) { Write-Host "  WARNING: some tasks failed" -ForegroundColor Yellow }
} finally {
    Pop-Location
}

# ---- Parse and compare ----
if (-not (Test-Path $v14Out)) { throw "V1.4 results not found: $v14Out" }
if (-not (Test-Path $v15Out)) { throw "V1.5 results not found: $v15Out" }

$v14 = Get-Content $v14Out -Raw | ConvertFrom-Json
$v15 = Get-Content $v15Out -Raw | ConvertFrom-Json

Write-Host ""
Write-Host "============================================" -ForegroundColor Green
Write-Host "    V1.4 vs V1.5 Cache Hit Rate Comparison" -ForegroundColor Green
Write-Host "============================================" -ForegroundColor Green

$hit14 = 0; $miss14 = 0; $cost14 = 0.0; $tok14 = 0
$hit15 = 0; $miss15 = 0; $cost15 = 0.0; $tok15 = 0

Write-Host ""
Write-Host "| Task | V1.4 Hit | V1.5 Hit | Delta | V1.4 Cost | V1.5 Cost |"
Write-Host "|------|----------|----------|-------|-----------|-----------|"

$count = [Math]::Max($v14.Count, $v15.Count)
for ($i = 0; $i -lt $count; $i++) {
    $t14 = if ($i -lt $v14.Count) { $v14[$i] } else { $null }
    $t15 = if ($i -lt $v15.Count) { $v15[$i] } else { $null }
    
    $name = if ($t14) { $t14.ID } else { $t15.ID }
    $skip = if ($t14 -and $t14.skipped) { $true } else { $false }
    
    if ($skip) {
        Write-Host "| $name | skipped | skipped | - | - | - |"
        continue
    }
    
    $denom14 = $t14.cache_hit_tokens + $t14.cache_miss_tokens
    $denom15 = $t15.cache_hit_tokens + $t15.cache_miss_tokens
    
    $r14 = if ($denom14 -gt 0) { [Math]::Round(100 * $t14.cache_hit_tokens / $denom14) } else { 0 }
    $r15 = if ($denom15 -gt 0) { [Math]::Round(100 * $t15.cache_hit_tokens / $denom15) } else { 0 }
    $delta = if ($denom14 -gt 0 -and $denom15 -gt 0) { "+" + ($r15 - $r14) + "%" } else { "-" }
    
    $c14 = "{0}{1:F4}" -f $t14.currency, $t14.cost
    $c15 = "{0}{1:F4}" -f $t15.currency, $t15.cost
    
    Write-Host "| $name | $r14% | $r15% | $delta | $c14 | $c15 |"
    
    $hit14 += $t14.cache_hit_tokens; $miss14 += $t14.cache_miss_tokens
    $hit15 += $t15.cache_hit_tokens; $miss15 += $t15.cache_miss_tokens
    $cost14 += $t14.cost; $cost15 += $t15.cost
    $tok14 += $t14.prompt_tokens + $t14.completion_tokens
    $tok15 += $t15.prompt_tokens + $t15.completion_tokens
}

# Summary
$agg14 = if (($hit14 + $miss14) -gt 0) { [Math]::Round(100 * $hit14 / ($hit14 + $miss14), 1) } else { 0 }
$agg15 = if (($hit15 + $miss15) -gt 0) { [Math]::Round(100 * $hit15 / ($hit15 + $miss15), 1) } else { 0 }

$missRate14 = [Math]::Round(100 - $agg14, 1)
$missRate15 = [Math]::Round(100 - $agg15, 1)
$improvement = if ($missRate15 -gt 0) { [Math]::Round($missRate14 / $missRate15, 1) } else { 999 }

$costSave = if ($cost14 -gt 0) { [Math]::Round(100 * (1 - $cost15 / $cost14), 1) } else { 0 }

Write-Host ""
Write-Host "---"
Write-Host "## Summary"
Write-Host ""
Write-Host "| Metric | V1.4 | V1.5 | Change |"
Write-Host "|--------|------|------|--------|"
Write-Host ("| Cache Hit Rate | {0}% | {1}% | +{2}% |" -f $agg14, $agg15, ([Math]::Round($agg15 - $agg14, 1)))
Write-Host ("| Miss Rate | {0}% | {1}% | {2}x better |" -f $missRate14, $missRate15, $improvement)
Write-Host ("| Total Tokens | {0:N0} | {1:N0} | {2}% |" -f $tok14, $tok15, [Math]::Round(100 * ($tok15 - $tok14) / [Math]::Max($tok14, 1), 1))
Write-Host ("| Total Cost | {0:F4} | {1:F4} | -{2}% |" -f $cost14, $cost15, $costSave)
Write-Host ("| Cache Hit Tokens | {0:N0} | {1:N0} | - |" -f $hit14, $hit15)
Write-Host ("| Cache Miss Tokens | {0:N0} | {1:N0} | - |" -f $miss14, $miss15)

Write-Host ""
Write-Host "Results saved to: $resultsDir/" -ForegroundColor Green
