@echo off
REM Build the mobile web frontend and embed it into the Go binary.
REM Run from the tianxuan/ directory.

echo [1/3] Building mobile frontend...
cd /d "%~dp0web-mobile"
call npm run build
if %ERRORLEVEL% neq 0 exit /b %ERRORLEVEL%

echo [2/3] Copying dist to embed directory...
cd /d "%~dp0"
if exist "internal\serve\mobileui\dist" rmdir /s /q "internal\serve\mobileui\dist"
xcopy /e /i /q "web-mobile\dist" "internal\serve\mobileui\dist"
echo Done.

echo [3/3] Mobile frontend embedded. Run "go build" to include it in the binary.
echo Connection: http://YOUR_IP:8787/mobile?token=YOUR_TOKEN
