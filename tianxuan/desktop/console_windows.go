//go:build windows

package main

import "syscall"

func init() {
	hideConsole()
}

// hideConsole 双重保险隐藏控制台窗口：
// ShowWindow(SW_HIDE) — 隐藏已存在的控制台窗口
// FreeConsole — 解除进程与控制台的关联（GUI 子系统二进制此调用无副作用）
func hideConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")

	// 先尝试隐藏窗口
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	showWindow := user32.NewProc("ShowWindow")
	if hwnd, _, _ := getConsoleWindow.Call(); hwnd != 0 {
		showWindow.Call(hwnd, 0) // SW_HIDE
	}

	// 再解除关联
	freeConsole := kernel32.NewProc("FreeConsole")
	freeConsole.Call()
}
