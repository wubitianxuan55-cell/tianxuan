package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(webFetch{}) }

type webFetch struct{}

const (
	webFetchTimeout = 15 * time.Second
	webFetchMaxRead = 1 << 20
)

func (webFetch) Name() string { return "web_fetch" }

func (webFetch) Description() string {
	return "抓取URL纯文本(去标签,SSRF安全)"
}

func (webFetch) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "url":{"type":"string","description":"Absolute URL beginning with http:// or https://"}
},
"required":["url"]
}`)
}

func (webFetch) ReadOnly() bool { return true }

func (webFetch) CompactDescription() string { return compactDesc["web_fetch"] }
func (webFetch) CompactSchema() json.RawMessage   { return compactSchema["web_fetch"] }

func (webFetch) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.URL == "" {
		return "", fmt.Errorf("url is required")
	}

	// V9.2: scheme 检查（必须 http/https）
	parsed, err := url.Parse(p.URL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q — only http(s) allowed", parsed.Scheme)
	}

	// V9.2: 本地/link-local 地址直接 fetch，远程 URL 走 AnySearch extract
	if !isLocalOrLinkLocal(p.URL) {
		if result, err := anysearchRetry(ctx, "extract", map[string]any{
			"url": p.URL,
		}); err == nil {
			return result, nil
		}
	}

	return directFetch(ctx, p.URL)
}

func isLocalOrLinkLocal(raw string) bool {
	return strings.HasPrefix(raw, "http://localhost") ||
		strings.HasPrefix(raw, "https://localhost") ||
		strings.HasPrefix(raw, "http://127.") ||
		strings.HasPrefix(raw, "https://127.") ||
		strings.HasPrefix(raw, "http://[::1]") ||
		strings.HasPrefix(raw, "https://[::1]") ||
		strings.Contains(raw, "169.254.") ||
		strings.HasPrefix(raw, "http://10.") ||
		strings.HasPrefix(raw, "http://172.16.") ||
		strings.HasPrefix(raw, "http://192.168.")
}

// ─── 直接 HTTP fetch（SSRF 保护）───────────────────────────────────

func directFetch(ctx context.Context, rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}

	reqCtx, cancel := context.WithTimeout(ctx, webFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "tianxuan-web-fetch/1.0")
	req.Header.Set("Accept", "text/html,text/plain,application/json,*/*")

	resp, err := ssrfGuardedClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", parsed.String(), err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(webFetchMaxRead)))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		return "Content-Type: " + contentType + "\n\n" + htmlToText(string(body)), nil
	}
	return string(body), nil
}

func ssrfGuardedClient() *http.Client {
	dialer := &net.Dialer{Timeout: webFetchTimeout}
	return &http.Client{
		Timeout: webFetchTimeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
				if err != nil {
					return nil, err
				}
				for _, ip := range ips {
					if blockedFetchIP(ip.IP) {
						return nil, fmt.Errorf("refusing to fetch internal address %s (resolves to %s)", host, ip.IP)
					}
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
			},
		},
	}
}

var cgnatRange = mustCIDR("100.64.0.0/10")

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

func blockedFetchIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || cgnatRange.Contains(ip)
}

// ─── 简易 HTML 转纯文本 ──────────────────────────────────────────

var (
	htmlScriptRe = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	htmlStyleRe  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	htmlTagRe    = regexp.MustCompile(`<[^>]+>`)
	htmlBlankRe  = regexp.MustCompile(`\n\s*\n`)
)

func htmlToText(htmlStr string) string {
	// 移除 <script> 和 <style> 块
	htmlStr = htmlScriptRe.ReplaceAllString(htmlStr, "")
	htmlStr = htmlStyleRe.ReplaceAllString(htmlStr, "")
	// 去除标签
	htmlStr = htmlTagRe.ReplaceAllString(htmlStr, "\n")
	// 合并空白
	htmlStr = htmlBlankRe.ReplaceAllString(htmlStr, "\n")
	// 解码 HTML 实体
	htmlStr = html.UnescapeString(htmlStr)
	return strings.TrimSpace(htmlStr)
}
