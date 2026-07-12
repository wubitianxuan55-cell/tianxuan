package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(webSearch{}) }

type webSearch struct{}

const (
	webSearchTimeout    = 15 * time.Second  // per-engine HTTP timeout
	webSearchMaxRetries = 1                 // retries per engine: 0, 1 (= 1 retry)
	webSearchMaxRead    = 512 << 10         // 512 KB
	webSearchTotalLimit = 20 * time.Second  // total execution deadline
)

// --- search engine interface ---

// searchEngine abstracts a single search backend.
type searchEngine interface {
	// Name returns a human-readable label for error messages.
	Name() string
	// Available reports whether this engine is configured and ready.
	Available() bool
	// Search executes a search and returns results (never nil on success).
	Search(ctx context.Context, query string, limit int) ([]searchResult, error)
}

// --- webSearch tool implementation ---

func (webSearch) Name() string { return "web_search" }

func (webSearch) Description() string {
	return "搜索公开网页（通过 SearXNG / Tavily / Brave Search）。返回结构化 JSON 数组，每项含 title/url/snippet/source 字段，支持引用追踪。当答案的正确性依赖于当前状态时使用——任何随时间变化的内容（事件、价格、发布版本、现实世界的状态）。先搜索再回答；常青问题不需要此工具。"
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

// engineError records a failed engine attempt for diagnostics.
type engineError struct {
	name    string
	err     error
	elapsed time.Duration
}

func (ws webSearch) Execute(ctx context.Context, args json.RawMessage) (string, error) {
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

	engines := ws.buildEngines()

	// Parallel execution: every engine fires in its own goroutine.
	// First success wins; failures are collected for diagnostics.
	resultCh := make(chan []searchResult, 1)
	errCh := make(chan engineError, len(engines))

	ctx, cancel := context.WithTimeout(ctx, webSearchTotalLimit)
	defer cancel()

	for _, eng := range engines {
		eng := eng
		go func() {
			defer func() {
				if r := recover(); r != nil {
					errCh <- engineError{name: eng.Name(), err: fmt.Errorf("panic: %v", r), elapsed: 0}
				}
			}()
			start := time.Now()
			results, err := eng.Search(ctx, p.Query, p.TopK)
			elapsed := time.Since(start)
			if err != nil {
				errCh <- engineError{name: eng.Name(), err: err, elapsed: elapsed}
				return
			}
			if len(results) == 0 {
				errCh <- engineError{name: eng.Name(), err: fmt.Errorf("no results"), elapsed: elapsed}
				return
			}
			select {
			case resultCh <- results:
			default:
				// another engine already won, discard
			}
		}()
	}

	// Collect: first result wins, or accumulate all failures.
	var failures []engineError
	for i := 0; i < len(engines); i++ {
		select {
		case results := <-resultCh:
			return formatResults(results), nil
		case fe := <-errCh:
			failures = append(failures, fe)
		case <-ctx.Done():
			// Timeout — drain any remaining errors that arrive quickly.
			failures = append(failures, engineError{name: "timeout", err: ctx.Err()})
			for j := i + 1; j < len(engines); j++ {
				select {
				case fe := <-errCh:
					failures = append(failures, fe)
				case <-time.After(100 * time.Millisecond):
				}
			}
			i = len(engines) // break outer loop
		}
	}

	// All engines failed — build detailed diagnostic.
	var diag strings.Builder
	diag.WriteString("所有搜索引擎失败：")
	for _, f := range failures {
		fmt.Fprintf(&diag, "\n  • %s (%v): %v", f.name, f.elapsed.Round(time.Millisecond), f.err)
	}
	if searchCfg == nil || (searchCfg.TavilyAPIKeyEnv == "" && searchCfg.BraveAPIKeyEnv == "" && searchCfg.LocalSearXNGURL == "") {
		diag.WriteString("\n\n💡 提示：配置搜索 API 可大幅提高成功率：")
		diag.WriteString("\n  1. Tavily（免费 1000次/月）：注册 tavily.com → 设环境变量 TAVILY_API_KEY")
		diag.WriteString("\n  2. Brave Search（免费 2000次/月）：注册 api.search.brave.com → 设环境变量 BRAVE_API_KEY")
		diag.WriteString("\n  3. 自建 SearXNG：docker run -d -p 8080:8080 searxng/searxng")
		diag.WriteString("\n  然后在 tianxuan.toml 中配置 [search] 节。")
	}
	return "", fmt.Errorf("%s", diag.String())
}

// buildEngines returns engines in priority order: local SearXNG → Tavily → Brave → public SearXNG.
func (webSearch) buildEngines() []searchEngine {
	var engines []searchEngine
	cfg := searchCfg // may be nil

	// 1. Local SearXNG (fastest, private)
	if cfg != nil && cfg.LocalSearXNGURL != "" {
		engines = append(engines, &localSearxNGEngine{baseURL: cfg.LocalSearXNGURL})
	}

	// 2. Tavily Search API
	if cfg != nil && cfg.TavilyKey() != "" {
		engines = append(engines, &tavilyEngine{apiKey: cfg.TavilyKey()})
	}

	// 3. Brave Search API
	if cfg != nil && cfg.BraveKey() != "" {
		engines = append(engines, &braveEngine{apiKey: cfg.BraveKey()})
	}

	// 4. Public SearXNG instances (always available as fallback)
	engines = append(engines, &publicSearxNGEngine{})

	return engines
}

// --- HTTP client ---

func searchHTTPClient() *http.Client {
	timeout := webSearchTimeout
	if searchCfg != nil {
		timeout = searchCfg.SearchTimeout()
	}
	dialer := &net.Dialer{Timeout: timeout}
	return &http.Client{
		Timeout: timeout,
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
						return nil, fmt.Errorf("refusing to connect to internal address %s", host)
					}
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
			},
			ForceAttemptHTTP2: false,
		},
	}
}

// --- Local SearXNG Engine ---

type localSearxNGEngine struct{ baseURL string }

func (e *localSearxNGEngine) Name() string    { return "local-searxng" }
func (e *localSearxNGEngine) Available() bool  { return e.baseURL != "" }
func (e *localSearxNGEngine) Search(ctx context.Context, query string, limit int) ([]searchResult, error) {
	return trySearXNG(ctx, e.baseURL, query, limit)
}

// --- Public SearXNG Engine ---

// publicSearxNGInstances — publicly accessible SearXNG instances returning JSON.
var publicSearxNGInstances = []string{
	"https://searx.be",
	"https://search.sapti.me",
	"https://searx.dresden.network",
	"https://search.bus-hit.me",
	"https://searx.tuxcloud.net",
	"https://search.ipv6s.net",
}

type publicSearxNGEngine struct{}

func (e *publicSearxNGEngine) Name() string   { return "public-searxng" }
func (e *publicSearxNGEngine) Available() bool { return true }
func (e *publicSearxNGEngine) Search(ctx context.Context, query string, limit int) ([]searchResult, error) {
	var lastErr error
	for _, baseURL := range publicSearxNGInstances {
		results, err := trySearXNG(ctx, baseURL, query, limit)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// --- Tavily Search API Engine ---

type tavilyEngine struct{ apiKey string }

func (e *tavilyEngine) Name() string   { return "tavily" }
func (e *tavilyEngine) Available() bool { return e.apiKey != "" }

type tavilyRequest struct {
	Query         string `json:"query"`
	SearchDepth   string `json:"search_depth,omitempty"`
	MaxResults    int    `json:"max_results,omitempty"`
	IncludeAnswer bool   `json:"include_answer,omitempty"`
}

type tavilyResponse struct {
	Results []struct {
		Title   string  `json:"title"`
		URL     string  `json:"url"`
		Content string  `json:"content"`
		Score   float64 `json:"score"`
	} `json:"results"`
	Answer string `json:"answer,omitempty"`
}

func (e *tavilyEngine) Search(ctx context.Context, query string, limit int) ([]searchResult, error) {
	body, err := json.Marshal(tavilyRequest{
		Query:      query,
		MaxResults: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+e.apiKey)
	httpReq.Header.Set("User-Agent", "tianxuan/1.0")

	resp, err := searchHTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, webSearchMaxRead))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var tr tavilyResponse
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

// Tavily
	results := make([]searchResult, 0, limit)
	for _, r := range tr.Results {
		if len(results) >= limit {
			break
		}
		results = append(results, searchResult{
			Title:   strings.TrimSpace(r.Title),
			URL:     strings.TrimSpace(r.URL),
			Snippet: truncate(r.Content, 300),
			Source:  "tavily",
		})
	}
	return results, nil
}

// --- Brave Search API Engine ---

type braveEngine struct{ apiKey string }

func (e *braveEngine) Name() string   { return "brave" }
func (e *braveEngine) Available() bool { return e.apiKey != "" }

type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

func (e *braveEngine) Search(ctx context.Context, query string, limit int) ([]searchResult, error) {
	searchURL := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query), limit)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Accept-Encoding", "gzip")
	httpReq.Header.Set("X-Subscription-Token", e.apiKey)
	httpReq.Header.Set("User-Agent", "tianxuan/1.0")

	resp, err := searchHTTPClient().Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, webSearchMaxRead))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var br braveResponse
	if err := json.Unmarshal(respBody, &br); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

// Brave
	results := make([]searchResult, 0, limit)
	for _, r := range br.Web.Results {
		if len(results) >= limit {
			break
		}
		results = append(results, searchResult{
			Title:   strings.TrimSpace(r.Title),
			URL:     strings.TrimSpace(r.URL),
			Snippet: truncate(r.Description, 300),
			Source:  "brave",
		})
	}
	return results, nil
}

// --- shared SearXNG implementation ---

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

// SearXNG
	results := make([]searchResult, 0, limit)
	for _, r := range resp.Results {
		if len(results) >= limit {
			break
		}
		results = append(results, searchResult{
			Title:   strings.TrimSpace(r.Title),
			URL:     strings.TrimSpace(r.URL),
			Snippet: truncate(r.Content, 300),
			Source:  "searxng",
		})
	}
	return results, nil
}

type searxNGResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
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
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(results)
	return strings.TrimSpace(out.String())
}

// --- helpers ---

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Source  string `json:"source"`
}
