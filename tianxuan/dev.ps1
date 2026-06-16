Start-Process -FilePath D:\AI\tianxuanX\tianxuan\bin\tianxuan.exe -ArgumentList "serve","--addr","127.0.0.1:8090"
Set-Location D:\AI\tianxuanX\tianxuan\desktop\frontend
Write-Host "http://127.0.0.1:5174"
npx vite
