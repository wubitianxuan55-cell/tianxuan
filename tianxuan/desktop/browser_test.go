package main

import (
	"strings"
	"testing"
)

// TestBrowserFetchTextRequiresURL: 浏览器文本模式必须拒绝空 URL（大声失败，
// 不静默抓取空地址）。
func TestBrowserFetchTextRequiresURL(t *testing.T) {
	a := NewApp()
	for _, raw := range []string{"", "   ", "\n\t"} {
		if _, err := a.BrowserFetchText(raw); err == nil {
			t.Errorf("BrowserFetchText(%q) must error on empty input", raw)
		}
	}
}

// TestBrowserFetchTextRejectsNonHTTP: 只允许 http(s) 绝对地址（file:// 等
// 本地协议必须拒绝，与 web_fetch 的 SSRF 边界一致）。
func TestBrowserFetchTextRejectsNonHTTP(t *testing.T) {
	a := NewApp()
	for _, raw := range []string{"file:///etc/passwd", "javascript:alert(1)", "ftp://example.com/x"} {
		if _, err := a.BrowserFetchText(raw); err == nil {
			t.Errorf("BrowserFetchText(%q) must reject non-http(s) scheme", raw)
		}
	}
}

// TestBrowserFetchTextNetwork: 文本模式抓取真实返回体（网络可用时）。
// 失败则跳过而非硬失败——CI 无网环境不阻塞。
func TestBrowserFetchTextNetwork(t *testing.T) {
	a := NewApp()
	out, err := a.BrowserFetchText("https://example.com")
	if err != nil {
		t.Skipf("network unavailable: %v", err)
	}
	if !strings.Contains(out, "Example Domain") && !strings.Contains(out, "example") {
		t.Errorf("expected example.com content, got %q", out)
	}
}
