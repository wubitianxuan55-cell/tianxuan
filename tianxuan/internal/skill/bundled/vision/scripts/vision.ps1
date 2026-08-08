#!/usr/bin/env pwsh
# Vision skill: send user-provided images to an OpenCode Zen vision model and
# print the text description. Zero dependencies (PowerShell only).
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File vision.ps1 <image> [more images...] [-Prompt "question"] [-Model model] [-Api url] [-MaxTokens N]
#
# Key resolution (priority): $env:VISION_API_KEY > $env:OPENCODE_API_KEY
#   (tianxuan loads .env into the process environment, so the child inherits it).
param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string[]]$Images,
    [string]$Prompt = "请详细描述这张图片的内容。",
    [string]$Model = "mimo-v2.5-free",
    [string]$Api = "https://opencode.ai/zen/v1",
    [int]$MaxTokens = 2048
)
$ErrorActionPreference = "Stop"

# Force UTF-8 on both streams so the host (tianxuan bash tool reads UTF-8)
# decodes Chinese output correctly regardless of the system ANSI code page.
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

function Fail([string]$msg) {
    [Console]::Error.WriteLine("错误: $msg")
    exit 1
}

if (-not $Images -or $Images.Count -eq 0) {
    Fail "至少需要一个图片路径。用法: vision.ps1 <图片路径...> [-Prompt 问题] [-Model 模型]"
}

$key = $env:VISION_API_KEY
if (-not $key) { $key = $env:OPENCODE_API_KEY }
if (-not $key) {
    Fail "未找到 API Key：请设置环境变量 VISION_API_KEY 或 OPENCODE_API_KEY（tianxuan 会自动从 .env 加载）。"
}

$mime = @{
    ".png"  = "image/png"
    ".jpg"  = "image/jpeg"
    ".jpeg" = "image/jpeg"
    ".gif"  = "image/gif"
    ".webp" = "image/webp"
    ".bmp"  = "image/bmp"
}

$parts = @(@{ type = "text"; text = $Prompt })
foreach ($img in $Images) {
    $resolved = Resolve-Path -LiteralPath $img -ErrorAction SilentlyContinue
    if (-not $resolved -or -not (Test-Path -LiteralPath $resolved.Path -PathType Leaf)) {
        Fail "图片不存在: $img"
    }
    $abs = $resolved.Path
    $bytes = [System.IO.File]::ReadAllBytes($abs)
    if ($bytes.Length -gt 8MB) {
        [Console]::Error.WriteLine("提示: $abs 超过 8MB（$([math]::Round($bytes.Length / 1MB, 1))MB），发送可能较慢")
    }
    $ext = [System.IO.Path]::GetExtension($abs).ToLowerInvariant()
    $mimeType = if ($mime.ContainsKey($ext)) { $mime[$ext] } else { "image/png" }
    $b64 = [Convert]::ToBase64String($bytes)
    $parts += @{ type = "image_url"; image_url = @{ url = "data:$mimeType;base64,$b64" } }
}

$body = @{
    model      = $Model
    messages   = @(@{ role = "user"; content = $parts })
    max_tokens = $MaxTokens
} | ConvertTo-Json -Depth 10

$endpoint = $Api.TrimEnd("/") + "/chat/completions"
try {
    # HttpWebRequest + manual UTF-8 decode: PowerShell 5.1's Invoke-RestMethod
    # decodes charset-less JSON as Latin-1, mangling Chinese output.
    $req = [System.Net.HttpWebRequest]::Create($endpoint)
    $req.Method = "POST"
    $req.ContentType = "application/json"
    $req.Headers["Authorization"] = "Bearer $key"
    $reqBytes = [System.Text.Encoding]::UTF8.GetBytes($body)
    $req.ContentLength = $reqBytes.Length
    $reqStream = $req.GetRequestStream()
    $reqStream.Write($reqBytes, 0, $reqBytes.Length)
    $reqStream.Close()
    $res = $req.GetResponse()
    $reader = New-Object System.IO.StreamReader($res.GetResponseStream(), [System.Text.Encoding]::UTF8)
    $jsonText = $reader.ReadToEnd()
    $reader.Close()
    $res.Close()
    $resp = $jsonText | ConvertFrom-Json
}
catch {
    $detail = $_.ErrorDetails.Message
    if (-not $detail) { $detail = $_.Exception.Message }
    Fail "API 请求失败: $detail"
}

$content = $resp.choices[0].message.content
if ($content) {
    Write-Output $content
}
else {
    Fail "API 返回内容为空"
}
