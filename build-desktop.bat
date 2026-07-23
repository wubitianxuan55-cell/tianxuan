@echo off
REM Sync bundled skills before build
echo [sync] Copying .tianxuan/skills -> internal/skill/bundled ...
robocopy "%~dp0.tianxuan\skills" "%~dp0tianxuan\internal\skill\bundled" /MIR /NJH /NJS /NFL >nul 2>&1
if %ERRORLEVEL% GEQ 8 (echo [sync] FAILED & exit /b 1) else (echo [sync] OK)

cd /d "%~dp0tianxuan\desktop"
wails build -ldflags "-s -w -H windowsgui" -o tianxuan-desktop.exe
