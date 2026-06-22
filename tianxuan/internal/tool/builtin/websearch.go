package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(webSearch{}) }

type webSearch struct{}

const webSearchTimeout = 10 * time.Second

func (webSearch) Name() string { return "web_search" }

func (webSearch) Description() string {
	return "搜索公开网页。返回带标题、URL 和摘要的排序结果。当答案的正确性依赖于当前状态时使用——任何随时间变化的内容（事件、价格、发布版本、现实世界的状态）。基于训练数据组合此类答案会编造过时数据；先搜索，然后将答案立足于结果中。对于常青/定义性问题不需要此工具。"
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

	// V9.2: AnySearch 主路径（重试+指数退避），DDG 降级
	result, err := anysearchRetry(ctx, "search", map[string]any{
		"query":       p.Query,
		"max_results": float64(p.TopK),
	})
	if err == nil {
		return result, nil
	}

	// 降级到 DuckDuckGo Lite
	return ddgSearch(ctx, p.Query, p.TopK)
}

// ─── DDG 降级路径（保留原有逻辑）────────────────────────────────────

func ddgSearch(ctx context.Context, query string, topK int) (string, error) {
	searchURL := "https://lite.duckduckgo.com/lite/?"
	searchURL += "q=" + url.QueryEscape(query)

	reqCtx, cancel := context.WithTimeout(ctx, webSearchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, searchURL, nil)
	if err != nil {
		return "", fmt.Errorf("ddg request: %w", err)
	}
	req.Header.Set("User-Agent", "tianxuan-web-search/1.0")
	req.Header.Set("Accept", "text/html")

	client := &http.Client{Timeout: webSearchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ddg search %q: %w", query, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return "", fmt.Errorf("ddg read: %w", err)
	}

	results := parseDuckDuckGoLite(string(body), topK)
	if len(results) == 0 {
		return "（未找到搜索结果）", nil
	}

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
	return out.String(), nil
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

var (
	ddgLinkRe    = regexp.MustCompile(`<a[^>]*href="([^"]*)"[^>]*>([^<]*)</a>`)
	ddgSnippetRe = regexp.MustCompile(`<td[^>]*>\s*(?:<span[^>]*>)?([^<]{10,200})\s*(?:</span>)?\s*</td>`)
	ddgTagRe     = regexp.MustCompile(`<[^>]+>`)
	ddgBlankRe   = regexp.MustCompile(`\n\s*\n`)
)

func parseDuckDuckGoLite(html string, limit int) []searchResult {
	results := make([]searchResult, 0, limit)
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
		if strings.HasPrefix(href, "/") ||
			strings.Contains(href, "duckduckgo.com") ||
			strings.Contains(href, "ad.") ||
			strings.HasPrefix(href, "javascript:") {
			continue
		}
		title = stripTags(title)
		results = append(results, searchResult{Title: title, URL: href})
	}

	snippets := ddgSnippetRe.FindAllStringSubmatch(html, -1)
	si := 0
	for i := range results {
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
