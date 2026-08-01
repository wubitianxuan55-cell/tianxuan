@echo off
REM V10.138: auto-extend PATH with project-local Go/Node and the Wails CLI,
REM so the build works without manually setting the environment (the frontend
REM compile previously failed with "'node' is not recognized").
set "PROJ_TOOLS=%~dp0tools"
if exist "%PROJ_TOOLS%\go\bin\go.exe"    set "PATH=%PROJ_TOOLS%\go\bin;%PATH%"
if exist "%PROJ_TOOLS%\node\node.exe"    set "PATH=%PROJ_TOOLS%\node;%PATH%"
if exist "%PROJ_TOOLS%\node\pnpm.cmd"    set "PATH=%PROJ_TOOLS%\node;%PATH%"
where wails >nul 2>&1 || (
  if exist "%USERPROFILE%\go\bin\wails.exe" set "PATH=%USERPROFILE%\go\bin;%PATH%"
)
REM Sync bundled skills before build
REM /E (not /MIR!): mirror mode deletes bundled skills absent from the runtime
REM skills dir (tdd/systematic-debugging/... are embed-only and were wiped by
REM /MIR, silently shipping a build without them). /E copies without deleting.
echo [sync] Copying .tianxuan/skills -> internal/skill/bundled (no-delete) ...
robocopy "%~dp0.tianxuan\skills" "%~dp0tianxuan\internal\skill\bundled" /E /NJH /NJS /NFL >nul 2>&1
if %ERRORLEVEL% GEQ 8 (echo [sync] FAILED & exit /b 1) else (echo [sync] OK)

cd /d "%~dp0tianxuan\desktop"
wails build -ldflags "-s -w -H windowsgui" -o tianxuan-desktop.exe
