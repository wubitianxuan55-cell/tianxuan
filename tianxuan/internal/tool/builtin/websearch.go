package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(webSearch{}) }

type webSearch struct{}

const (
	webSearchTimeout    = 15 * time.Second
	webSearchMaxRetries = 2         // exponential backoff: 1s, 2s
	webSearchMaxRead    = 512 << 10 // 512 KB
)

// searxNGInstances — 公开 SearXNG 实例，返回 JSON 格式结果.
// DuckDuckGo 已全面封杀非浏览器客户端 (CAPTCHA 202), SearXNG 作为主引擎.
var searxNGInstances = []string{
	"https://searx.be",
	"https://search.sapti.me",
	"https://searx.dresden.network",
}

func (webSearch) Name() string { return "web_search" }

func (webSearch) Description() string {
	return "搜索公开网页（通过 SearXNG / DuckDuckGo）。返回带标题、URL 和摘要的排序结果。当答案的正确性依赖于当前状态时使用——任何随时间变化的内容（事件、价格、发布版本、现实世界的状态）。基于训练数据组合此类答案会编造过时数据；先搜索，然后将答案立足于结果中。对于常青/定义性问题不需要此工具。"
}

func (webSearch) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "query":{"type":"string","description":"自然语言搜索词"},
  "topK":{"type":"integer","description":"返回结果数（默认5，最多10）","minimum":1,"maximum":10}
},
"required":["query"]
}`)
}

func (webSearch) ReadOnly() bool { return true }

func (webSearch) CompactDescription() string { return compactDesc["web_search"] }
func (webSearch) CompactSchema() json.RawMessage   { return compactSchema["web_search"] }

// searchHTTPClient returns an HTTP client with SSRF protection and HTTP/1.1
// forced — DuckDuckGo blocks HTTP/2 connections from non-browser clients with
// EOF, so we force HTTP/1.1 to at least get a proper response.
func searchHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: webSearchTimeout}
	return &http.Client{
		Timeout: webSearchTimeout,
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
						return nil, fmt.Errorf("refusing to fetch internal address %s", host)
					}
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
			},
			ForceAttemptHTTP2: false, // DuckDuckGo blocks HTTP/2 bots with EOF
		},
	}
}

func (webSearch) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Query string `json:"query"`
		TopK  int    `json:"topK"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.Query == "" {
		return "", fmt.Errorf("query is required")
	}
	if p.TopK <= 0 {
		p.TopK = 5
	}
	if p.TopK > 10 {
		p.TopK = 10
	}

	// Try SearXNG first (JSON API, no CAPTCHA), fallback to DDG.
	results, err := searchSearXNG(ctx, p.Query, p.TopK)
	if err == nil && len(results) > 0 {
		return formatResults(results), nil
	}

	// Fallback: DuckDuckGo Lite (HTTP/1.1 to avoid EOF).
	results, err = searchDuckDuckGo(ctx, p.Query, p.TopK)
	if err == nil && len(results) > 0 {
		return formatResults(results), nil
	}

	if err != nil {
		return "", fmt.Errorf("all search engines failed, last error: %w", err)
	}
	return "（未找到搜索结果）", nil
}

// --- SearXNG ---

type searxNGResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}

func searchSearXNG(ctx context.Context, query string, limit int) ([]searchResult, error) {
	var lastErr error
	for _, baseURL := range searxNGInstances {
		results, err := trySearXNG(ctx, baseURL, query, limit)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func trySearXNG(ctx context.Context, baseURL, query string, limit int) ([]searchResult, error) {
	searchURL := fmt.Sprintf("%s/search?%s", strings.TrimRight(baseURL, "/"),
		"q="+url.QueryEscape(query)+"&format=json&language=zh-CN&safesearch=1")

	client := searchHTTPClient()
	body, err := doSearchRequest(ctx, client, searchURL, webSearchMaxRetries)
	if err != nil {
		return nil, err
	}

	var resp searxNGResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse SearXNG response: %w", err)
	}

	results := make([]searchResult, 0, limit)
	for _, r := range resp.Results {
		if len(results) >= limit {
			break
		}
		snippet := r.Content
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		results = append(results, searchResult{
			Title:   strings.TrimSpace(r.Title),
			URL:     strings.TrimSpace(r.URL),
			Snippet: strings.TrimSpace(snippet),
		})
	}
	return results, nil
}

// --- DuckDuckGo Lite (fallback) ---

func searchDuckDuckGo(ctx context.Context, query string, limit int) ([]searchResult, error) {
	searchURL := "https://lite.duckduckgo.com/lite/?"
	searchURL += "q=" + url.QueryEscape(query)

	client := searchHTTPClient()
	body, err := doSearchRequest(ctx, client, searchURL, webSearchMaxRetries)
	if err != nil {
		return nil, err
	}

	// Check for CAPTCHA.
	if strings.Contains(string(body), "bots use DuckDuckGo") {
		return nil, fmt.Errorf("DuckDuckGo returned CAPTCHA challenge")
	}

	return parseDuckDuckGoLite(string(body), limit), nil
}

// --- shared HTTP ---

func doSearchRequest(ctx context.Context, client *http.Client, urlStr string, maxRetries int) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		reqCtx, cancel := context.WithTimeout(ctx, webSearchTimeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, urlStr, nil)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; tianxuan/1.0)")
		req.Header.Set("Accept", "text/html,application/json")

		resp, err := client.Do(req)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			lastErr = fmt.Errorf("search engine returned %d", resp.StatusCode)
			continue
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, webSearchMaxRead))
		if err != nil {
			lastErr = fmt.Errorf("read body: %w", err)
			continue
		}
		return body, nil
	}
	return nil, fmt.Errorf("search failed after %d retries: %w", maxRetries+1, lastErr)
}

// --- formatting ---

func formatResults(results []searchResult) string {
	var out strings.Builder
	for i, r := range results {
		if i > 0 {
			out.WriteString("\n\n")
		}
		fmt.Fprintf(&out, "%d. %s\n   %s", i+1, r.Title, r.URL)
		if r.Snippet != "" {
			out.WriteString("\n   ")
			out.WriteString(r.Snippet)
		}
	}
	return out.String()
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

// DuckDuckGo Lite 结果页结构:
//
//	<table>
//	  <tr><td><a rel="nofollow" href="URL">Title</a> </td></tr>
//	  <tr><td><span class="link-text">display URL</span></td></tr>
//	  <tr><td><span class="snippet">snippet text</span></td></tr>
//	</table>
//
// 每个结果跨 3 个 <tr>：标题行→链接行→摘要行。
var (
	ddgLinkRe    = regexp.MustCompile(`<a[^>]*href="([^"]*)"[^>]*>([^<]*)</a>`)
	ddgSnippetRe = regexp.MustCompile(`<td[^>]*>\s*(?:<span[^>]*>)?([^<]{10,200})\s*(?:</span>)?\s*</td>`)
	ddgTagRe     = regexp.MustCompile(`<[^>]+>`)
	ddgBlankRe   = regexp.MustCompile(`\n\s*\n`)
)

func parseDuckDuckGoLite(html string, limit int) []searchResult {
	// 策略：找到所有结果链接（rel="nofollow" 的外部链接），
	// 每个链接对应一个结果。摘要从后续的 <td> 中提取。
	results := make([]searchResult, 0, limit)

	// 找到所有外部链接（排除 DuckDuckGo 自身的链接）
	linkMatches := ddgLinkRe.FindAllStringSubmatch(html, -1)
	for _, m := range linkMatches {
		if len(results) >= limit {
			break
		}
		href := strings.TrimSpace(m[1])
		title := strings.TrimSpace(m[2])
		if title == "" || href == "" {
			continue
		}
		// 排除 DuckDuckGo 内部链接和广告链接
		if strings.HasPrefix(href, "/") ||
			strings.Contains(href, "duckduckgo.com") ||
			strings.Contains(href, "ad.") ||
			strings.HasPrefix(href, "javascript:") {
			continue
		}
		title = stripTags(title)
		results = append(results, searchResult{Title: title, URL: href})
	}

	// 提取摘要文本（<td> 中的较长文本段）
	snippets := ddgSnippetRe.FindAllStringSubmatch(html, -1)
	si := 0
	for i := range results {
		// 跳过链接行和空白
		for si < len(snippets) {
			s := strings.TrimSpace(snippets[si][1])
			s = stripTags(s)
			if len(s) > 20 && !strings.HasPrefix(s, "http") {
				results[i].Snippet = s
				si++
				break
			}
			si++
		}
	}

	return results
}

func stripTags(s string) string {
	s = ddgTagRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	s = ddgBlankRe.ReplaceAllString(s, "\n")
	return s
}
