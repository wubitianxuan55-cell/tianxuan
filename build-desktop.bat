@echo off
cd /d "%~dp0tianxuan\desktop"
wails build -ldflags "-s -w -H windowsgui" -o tianxuan-desktop.exe
