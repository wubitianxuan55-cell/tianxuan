$env:PATH = "C:\Program Files\Go\bin;C:\Users\吴比\go\bin;" + $env:PATH
$env:GOPROXY = "https://goproxy.io,direct"
Set-Location D:\AI\tianxuanX\tianxuan\desktop
Copy-Item ..\tianxuan.toml . -Force
# Wails runs from build/bin — copy config there too
New-Item -ItemType Directory -Force -Path build\bin | Out-Null
Copy-Item ..\tianxuan.toml build\bin\ -Force
# .env no longer copied — API key lives in ~/.env (loaded by loadDotEnv as fallback)
wails dev
