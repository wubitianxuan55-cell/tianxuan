@echo off
REM Sync bundled skills before build
echo [sync] Copying .tianxuan/skills -> internal/skill/bundled ...
robocopy "%~dp0.tianxuan\skills" "%~dp0tianxuan\internal\skill\bundled" /MIR /NJH /NJS /NFL >nul 2>&1
if %ERRORLEVEL% GEQ 8 (echo [sync] FAILED & exit /b 1) else (echo [sync] OK)

set PATH=C:\Program Files\Go\bin;%PATH%
D:
cd D:\AI\tianxuanX\tianxuan\desktop
C:\Users\吴比\go\bin\wails.exe build -o tianxuan-desktop.exe
