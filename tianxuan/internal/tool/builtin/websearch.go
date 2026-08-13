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
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	nethtml "golang.org/x/net/html"

	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(webSearch{}) }

type webSearch struct{}

const (
	webSearchTimeout    = 15 * time.Second // per-engine HTTP timeout
	webSearchMaxRetries = 1                // retries per engine: 0, 1 (= 1 retry)
	webSearchMaxRead    = 512 << 10        // 512 KB
	webSearchTotalLimit = 20 * time.Second // total execution deadline
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
	return "Search public web pages (SearXNG / Tavily / Brave Search) and return a structured JSON array with title/url/snippet/source per item, with citation tracking. Use when correctness depends on current state — anything that changes over time (events, prices, release versions, real-world state). Search first; evergreen questions don’t need this tool."
}

func (webSearch) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "query":{"type":"string","description":"Natural-language search query"},
  "topK":{"type":"integer","description":"Number of results (default 5, max 10)","minimum":1,"maximum":10}
},
"required":["query"]
}`)
}

func (webSearch) ReadOnly() bool      { return true }
func (webSearch) Kind() tool.ToolKind { return tool.KindFetch }

func (webSearch) CompactDescription() string     { return compactDesc["web_search"] }
func (webSearch) CompactSchema() json.RawMessage { return compactSchema["web_search"] }

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

	// 4. Bing HTML — free fallback that works without an API key.
	engines = append(engines, &bingEngine{})

	// 5. Public SearXNG instances (last fallback)
	engines = append(engines, &publicSearxNGEngine{})

	return engines
}

// --- Bing HTML Engine (free fallback) ---

type bingEngine struct{}

func (e *bingEngine) Name() string    { return "bing" }
func (e *bingEngine) Available() bool { return true }

func (e *bingEngine) Search(ctx context.Context, query string, limit int) ([]searchResult, error) {
	searchURL := fmt.Sprintf("https://www.bing.com/search?q=%s&count=%d&setlang=zh-CN",
		url.QueryEscape(query), limit)
	body, err := doSearchRequest(ctx, searchHTTPClient(), func(reqCtx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, searchURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		return req, nil
	}, webSearchMaxRetries)
	if err != nil {
		return nil, err
	}
	return parseBingResults(body, limit)
}

// parseBingResults extracts organic results (li.b_algo) from Bing HTML.
func parseBingResults(body []byte, limit int) ([]searchResult, error) {
	doc, err := nethtml.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse Bing response: %w", err)
	}
	var results []searchResult
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		if len(results) >= limit {
			return
		}
		if n.Type == nethtml.ElementNode && n.Data == "li" && hasHTMLClass(n, "b_algo") {
			if r, ok := extractBingResult(n); ok {
				results = append(results, r)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if len(results) == 0 {
		return nil, fmt.Errorf("no organic results in Bing response")
	}
	return results, nil
}

// extractBingResult pulls title/url from the first h2>a and snippet from the
// first <p> inside an organic result block.
func extractBingResult(li *nethtml.Node) (searchResult, bool) {
	var r searchResult
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n.Type == nethtml.ElementNode && n.Data == "a" && r.URL == "" {
			if href := htmlAttrValue(n, "href"); strings.HasPrefix(href, "http") {
				r.URL = href
				r.Title = nodeText(n)
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(li)
	if r.URL == "" || r.Title == "" {
		return r, false
	}

	r.Source = "bing"
	r.Title = strings.TrimSpace(r.Title)
	for c := li.FirstChild; c != nil; c = c.NextSibling {
		if txt := findSnippet(c); txt != "" {
			r.Snippet = truncate(txt, 300)
			break
		}
	}
	return r, true
}

// findSnippet returns the first <p> text under a node, or "".
func findSnippet(n *nethtml.Node) string {
	if n.Type == nethtml.ElementNode && n.Data == "p" {
		return strings.TrimSpace(nodeText(n))
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if txt := findSnippet(c); txt != "" {
			return txt
		}
	}
	return ""
}

// nodeText concatenates all text under a node.
func nodeText(n *nethtml.Node) string {
	var b strings.Builder
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n.Type == nethtml.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}

func hasHTMLClass(n *nethtml.Node, cls string) bool {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, "class") {
			for _, c := range strings.Fields(a.Val) {
				if c == cls {
					return true
				}
			}
		}
	}
	return false
}

func htmlAttrValue(n *nethtml.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}

// --- HTTP client ---

// sharedSearchTransport keeps a connection pool across search calls so DNS
// lookups and TLS handshakes are reused instead of rebuilt per request.
var (
	searchTransportOnce   sync.Once
	sharedSearchTransport *http.Transport
)

func searchHTTPClient() *http.Client {
	timeout := webSearchTimeout
	if searchCfg != nil {
		timeout = searchCfg.SearchTimeout()
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: searchTransport(),
	}
}

func searchTransport() *http.Transport {
	searchTransportOnce.Do(func() {
		sharedSearchTransport = buildSearchTransport()
	})
	return sharedSearchTransport
}

func buildSearchTransport() *http.Transport {
	timeout := webSearchTimeout
	if searchCfg != nil {
		timeout = searchCfg.SearchTimeout()
	}
	dialer := &net.Dialer{Timeout: timeout}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := resolveAllowedSearchIPs(ctx, host)
			if err != nil {
				return nil, err
			}
			var lastErr error
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("no reachable address for %s", host)
			}
			return nil, lastErr
		},
		ForceAttemptHTTP2: false,
	}
}

// lookupSearchIPs resolves a host to IP addresses. A variable so tests can
// substitute a deterministic resolver.
var lookupSearchIPs = func(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

// resolveAllowedSearchIPs resolves host and returns only IPs web_search may
// connect to. Private, link-local, unspecified and CGNAT addresses are refused
// (SSRF guard); empty resolutions are an error, never an empty success.
func resolveAllowedSearchIPs(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := lookupSearchIPs(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no IP addresses found for %s", host)
	}
	out := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		if blockedFetchIP(a.IP) {
			return nil, fmt.Errorf("refusing to connect to internal address %s", host)
		}
		out = append(out, a.IP)
	}
	return out, nil
}

// --- Local SearXNG Engine ---

type localSearxNGEngine struct{ baseURL string }

func (e *localSearxNGEngine) Name() string    { return "local-searxng" }
func (e *localSearxNGEngine) Available() bool { return e.baseURL != "" }
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

func (e *publicSearxNGEngine) Name() string    { return "public-searxng" }
func (e *publicSearxNGEngine) Available() bool { return true }
func (e *publicSearxNGEngine) Search(ctx context.Context, query string, limit int) ([]searchResult, error) {
	// Fire every public instance in parallel: the first instance with results
	// wins, and the overall call stays within one engine timeout instead of
	// serializing N timeouts (which would exceed the total execution limit).
	type outcome struct {
		results []searchResult
		err     error
	}
	ch := make(chan outcome, len(publicSearxNGInstances))
	ctx, cancel := context.WithTimeout(ctx, webSearchTimeout)
	defer cancel()

	for _, baseURL := range publicSearxNGInstances {
		baseURL := baseURL
		go func() {
			results, err := trySearXNG(ctx, baseURL, query, limit)
			ch <- outcome{results: results, err: err}
		}()
	}

	var lastErr error
	for i := 0; i < len(publicSearxNGInstances); i++ {
		select {
		case o := <-ch:
			if o.err == nil && len(o.results) > 0 {
				return o.results, nil
			}
			if o.err != nil {
				lastErr = o.err
			} else {
				lastErr = fmt.Errorf("no results")
			}
		case <-ctx.Done():
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			return nil, lastErr
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all public SearXNG instances returned no results")
	}
	return nil, lastErr
}

// --- Tavily Search API Engine ---

type tavilyEngine struct {
	apiKey  string
	baseURL string // test override; empty = official API
}

func (e *tavilyEngine) Name() string    { return "tavily" }
func (e *tavilyEngine) Available() bool { return e.apiKey != "" }

func (e *tavilyEngine) endpoint() string {
	if e.baseURL != "" {
		return e.baseURL
	}
	return "https://api.tavily.com/search"
}

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
	payload, err := json.Marshal(tavilyRequest{
		Query:      query,
		MaxResults: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	body, err := doSearchRequest(ctx, searchHTTPClient(), func(reqCtx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, e.endpoint(), strings.NewReader(string(payload)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
		req.Header.Set("User-Agent", "tianxuan/1.0")
		return req, nil
	}, webSearchMaxRetries)
	if err != nil {
		return nil, err
	}

	var tr tavilyResponse
	if err := json.Unmarshal(body, &tr); err != nil {
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

type braveEngine struct {
	apiKey  string
	baseURL string // test override; empty = official API
}

func (e *braveEngine) Name() string    { return "brave" }
func (e *braveEngine) Available() bool { return e.apiKey != "" }

func (e *braveEngine) endpoint() string {
	if e.baseURL != "" {
		return e.baseURL
	}
	return "https://api.search.brave.com/res/v1/web/search"
}

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
	// Accept-Encoding is intentionally left unset: Go's http.Transport then
	// advertises gzip itself and transparently decompresses the response.
	body, err := doSearchRequest(ctx, searchHTTPClient(), func(reqCtx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet,
			fmt.Sprintf("%s?q=%s&count=%d", e.endpoint(), url.QueryEscape(query), limit), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Subscription-Token", e.apiKey)
		req.Header.Set("User-Agent", "tianxuan/1.0")
		return req, nil
	}, webSearchMaxRetries)
	if err != nil {
		return nil, err
	}

	var br braveResponse
	if err := json.Unmarshal(body, &br); err != nil {
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
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")
	params.Set("safesearch", "1")
	if lang := searchLanguageParam(query); lang != "" {
		params.Set("language", lang)
	}
	searchURL := fmt.Sprintf("%s/search?%s", strings.TrimRight(baseURL, "/"), params.Encode())

	client := searchHTTPClient()
	body, err := doSearchRequest(ctx, client, func(reqCtx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, searchURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; tianxuan/1.0)")
		req.Header.Set("Accept", "text/html,application/json")
		return req, nil
	}, webSearchMaxRetries)
	if err != nil {
		return nil, err
	}

	var resp searxNGResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		if looksLikeHTML(string(body)) {
			return nil, fmt.Errorf("instance blocked automated access (bot-protection page returned)")
		}
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

// doSearchRequest runs an HTTP request with retries on transient failures.
// buildReq is called per attempt so request bodies are fresh on retry.
func doSearchRequest(ctx context.Context, client *http.Client, buildReq func(context.Context) (*http.Request, error), maxRetries int) ([]byte, error) {
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
		req, err := buildReq(reqCtx)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("build request: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			// Rate limits do not clear within one retry window; retrying only
			// burns time that other engines could use.
			cancel()
			resp.Body.Close()
			return nil, fmt.Errorf("search engine rate-limited (HTTP 429)")
		}
		if resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode >= http.StatusInternalServerError {
			cancel()
			lastErr = fmt.Errorf("search engine returned %d", resp.StatusCode)
			resp.Body.Close()
			continue
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, webSearchMaxRead))
		// Cancel only after the body is fully read: cancelling earlier aborts
		// large responses mid-transfer.
		cancel()
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read body: %w", err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("search engine returned HTTP %d: %s", resp.StatusCode, truncate(string(body), 300))
		}
		return body, nil
	}
	return nil, fmt.Errorf("search failed after %d attempts: %w", maxRetries+1, lastErr)
}

// searchLanguageParam picks a SearXNG language hint from the query. CJK
// queries search zh-CN; Latin-only queries leave language unset so SearXNG
// picks the best locale itself instead of forcing Chinese results.
func searchLanguageParam(query string) string {
	for _, r := range query {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			return "zh-CN"
		}
	}
	return ""
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
	if maxLen <= 0 || utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen])
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Source  string `json:"source"`
}
