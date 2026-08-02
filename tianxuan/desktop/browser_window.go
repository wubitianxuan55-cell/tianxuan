//go:build windows

package main

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	webview "github.com/jchv/go-webview2"
)

var (
	browserWinMu sync.Mutex
	browserWin   webview.WebView
)

// browserDataPath 返回独立 WebView2 user data folder——浏览器窗口的 cookies/
// 会话与主应用 WebView2 完全隔离（Codex 内置浏览器独立 profile 的蒸馏）。
func browserDataPath() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "tianxuan", "browser-profile"), nil
}

// OpenBrowserWindow 打开独立 WebView2 浏览器窗口（go-webview2 高层 API）。
// 与 iframe 方案的关键差异：真实完整渲染（无 X-Frame-Options/CSP 限制）、
// 独立 profile（可登录、跨窗口共享会话）。窗口在后台 goroutine 跑消息循环，
// 关闭后清理引用；已开窗口时只导航到新 URL。
func (a *App) OpenBrowserWindow(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("url must be an absolute http(s) address")
	}

	browserWinMu.Lock()
	defer browserWinMu.Unlock()
	if browserWin != nil {
		// 已有浏览器窗口：在主线程导航到新地址。
		win := browserWin
		win.Dispatch(func() { win.Navigate(rawURL) })
		return nil
	}

	dataPath, err := browserDataPath()
	if err != nil {
		return err
	}
	w := webview.NewWithOptions(webview.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  dataPath,
		WindowOptions: webview.WindowOptions{
			Title:  "tianxuan 浏览器",
			Width:  1280,
			Height: 800,
			Center: true,
		},
	})
	if w == nil {
		return errors.New("failed to create browser window")
	}
	browserWin = w
	w.Navigate(strings.TrimSpace(rawURL))
	go func() {
		w.Run()
		w.Destroy()
		browserWinMu.Lock()
		if browserWin == w {
			browserWin = nil
		}
		browserWinMu.Unlock()
	}()
	return nil
}
