@echo off
REM Sync bundled skills before build
REM /E (not /MIR!): mirror mode deletes bundled skills absent from the runtime
REM skills dir (tdd/systematic-debugging/... are embed-only and were wiped by
REM /MIR, silently shipping a build without them). /E copies without deleting.
echo [sync] Copying .tianxuan/skills -> internal/skill/bundled (no-delete) ...
robocopy "%~dp0.tianxuan\skills" "%~dp0tianxuan\internal\skill\bundled" /E /NJH /NJS /NFL >nul 2>&1
if %ERRORLEVEL% GEQ 8 (echo [sync] FAILED & exit /b 1) else (echo [sync] OK)

cd /d "%~dp0tianxuan\desktop"
wails build -ldflags "-s -w -H windowsgui" -o tianxuan-desktop.exe
