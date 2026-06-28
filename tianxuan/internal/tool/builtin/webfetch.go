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

func init() { tool.RegisterBuiltin(webFetch{}) }

type webFetch struct{}

const (
	webFetchTimeout       = 15 * time.Second
	webFetchMaxRead       = 1 << 20 // 1 MiB cap before extraction
	webFetchDefaultRetries = 2       // V10.5: 默认重试次数
)

func (webFetch) Name() string { return "web_fetch" }

func (webFetch) Description() string {
	return "Fetch a URL over HTTPS/HTTP and return its text content. HTML pages are reduced to readable text (scripts, styles, tags stripped, whitespace collapsed); JSON / plain text / markdown bodies come back verbatim. Use retries=N for transient network errors (default 2, exponential backoff 1s→2s→4s)."
}

func (webFetch) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "url":{"type":"string","description":"Absolute URL beginning with http:// or https://"},
  "retries":{"type":"integer","description":"Max retries on transient network errors (default 2). Exponential backoff: 1s, 2s, 4s...","minimum":0,"maximum":5}
},
"required":["url"]
}`)
}

func (webFetch) ReadOnly() bool { return true }

func (webFetch) CompactDescription() string { return compactDesc["web_fetch"] }
func (webFetch) CompactSchema() json.RawMessage   { return compactSchema["web_fetch"] }

// ssrfGuardedClient is an HTTP client whose dialer refuses to connect to private,
// link-local, or unspecified addresses — the SSRF surface a prompt-injected fetch
// would aim at (cloud metadata at 169.254.169.254, RFC1918 internal services).
// Loopback is allowed: the agent can already reach localhost via bash, so a local
// dev server stays fetchable. The check runs at dial time on the resolved IP, so a
// public host that redirects or DNS-rebinds to an internal address is caught too.
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
				// Dial the IP we just vetted, not the hostname, so the connection
				// can't re-resolve to a different (internal) address (DNS rebinding).
				return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
			},
		},
	}
}

// cgnatRange is RFC 6598 shared address space (100.64.0.0/10). Go's IsPrivate
// doesn't cover it, yet some clouds host instance metadata there (Alibaba Cloud
// at 100.100.100.200), so it's an SSRF target web_fetch must refuse too.
var cgnatRange = mustCIDR("100.64.0.0/10")

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// blockedFetchIP reports whether ip is an address web_fetch must not reach.
func blockedFetchIP(ip net.IP) bool {
	return ip.IsPrivate() || // RFC1918 + IPv6 unique-local (fc00::/7)
		ip.IsLinkLocalUnicast() || // 169.254.0.0/16 (incl. cloud metadata) + fe80::/10
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || // 0.0.0.0 / ::
		cgnatRange.Contains(ip) // 100.64.0.0/10 (incl. Alibaba Cloud metadata)
}

// isTransientError reports whether err represents a transient network issue
// that is worth retrying (DNS failures, connection refused/timeout/reset, TLS
// handshake failures), as opposed to permanent errors (invalid URL, SSRF block)
// or HTTP-level errors (4xx/5xx) which should be returned as-is.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// Permanent errors we should NOT retry
	if strings.Contains(msg, "refusing to fetch internal") {
		return false
	}
	if strings.Contains(msg, "unsupported protocol") {
		return false
	}
	// Transient errors worth retrying
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection timed out") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "TLS handshake timeout") ||
		strings.Contains(msg, "broken pipe") {
		return true
	}
	// net.Error with Timeout() or Temporary() is transient
	if ne, ok := err.(net.Error); ok {
		return ne.Timeout() || ne.Temporary()
	}
	return false
}

// doFetch performs a single HTTP fetch and returns the cleaned result.
// Extracted so the retry loop can call it without duplicating logic.
func doFetch(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("url must be an absolute http(s) address")
	}

	reqCtx, cancel := context.WithTimeout(ctx, webFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "tianxuan-web-fetch/1.0")
	req.Header.Set("Accept", "text/html,text/plain,text/markdown,application/json,*/*;q=0.5")

	resp, err := ssrfGuardedClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxRead))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	out := string(body)
	if strings.Contains(ct, "text/html") || looksLikeHTML(out) {
		out = htmlToText(out)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return fmt.Sprintf("(empty body — status %s)", resp.Status), nil
	}
	header := fmt.Sprintf("status %s · %s · %d bytes\n\n", resp.Status, contentTypeShort(ct), len(body))
	return header + out, nil
}

func (webFetch) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		URL     string `json:"url"`
		Retries *int   `json:"retries,omitempty"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.URL == "" {
		return "", fmt.Errorf("url is required")
	}

	maxRetries := webFetchDefaultRetries
	if p.Retries != nil {
		maxRetries = *p.Retries
	}

	// V10.5: 自动重试 — 指数退避处理瞬时网络错误
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<(attempt-1)) * time.Second // 1s, 2s, 4s, 8s
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}
		result, err := doFetch(ctx, p.URL)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !isTransientError(err) {
			// Non-transient error — don't retry
			return "", err
		}
	}
	return "", fmt.Errorf("fetch %s failed after %d retries: %w", p.URL, maxRetries, lastErr)
}

// looksLikeHTML lets servers that misreport Content-Type still hit the HTML
// reducer — GitHub raw pages and many docs sites lie about content type.
func looksLikeHTML(s string) bool {
	head := s
	if len(head) > 512 {
		head = head[:512]
	}
	low := strings.ToLower(head)
	return strings.Contains(low, "<!doctype html") || strings.Contains(low, "<html")
}

var (
	scriptStyle = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(?:script|style)>`)
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	anyTag      = regexp.MustCompile(`(?s)<[^>]+>`)
	multiBlank  = regexp.MustCompile(`\n[\t ]*\n([\t ]*\n)+`)
	trailingWS  = regexp.MustCompile(`[\t ]+\n`)
)

// htmlToText strips <script>/<style> blocks, HTML comments, and every other
// tag, then unescapes the common entities and collapses runs of blank lines.
// It is intentionally lossy — we want to give the model readable text rather
// than preserve structure for re-rendering.
func htmlToText(s string) string {
	s = scriptStyle.ReplaceAllString(s, "")
	s = htmlComment.ReplaceAllString(s, "")
	s = anyTag.ReplaceAllString(s, "")

	repl := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&apos;", "'",
		"&nbsp;", " ",
	)
	s = repl.Replace(s)

	s = trailingWS.ReplaceAllString(s, "\n")
	s = multiBlank.ReplaceAllString(s, "\n\n")
	return s
}

func contentTypeShort(ct string) string {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.TrimSpace(ct)
}
