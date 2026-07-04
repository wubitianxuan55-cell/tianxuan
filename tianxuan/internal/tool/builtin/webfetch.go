package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	nethtml "golang.org/x/net/html"

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

	// Domain allow/deny policy check.
	if err := checkDomainPolicy(u.Hostname()); err != nil {
		return "", err
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
	multiBlank = regexp.MustCompile(`\n[\t ]*\n([\t ]*\n)+`)
	trailingWS = regexp.MustCompile(`[\t ]+\n`)
)

// ── HTML tokenizer (V10.29) ───────────────────────────────────────────
// Ported from Reasonix V1.15 (MIT). Uses golang.org/x/net/html tokenizer
// to produce structured Markdown-like output with headings, lists, code
// blocks, links, and tables — replacing the old regex-based flattener.

// htmlToText converts HTML to readable Markdown-like text.
func htmlToText(s string) string {
	w := &htmlTextWriter{}
	tokenizer := nethtml.NewTokenizer(strings.NewReader(s))
	skipDepth := 0
	preDepth := 0
	for {
		tt := tokenizer.Next()
		switch tt {
		case nethtml.ErrorToken:
			return normalizeHTMLText(w.String())
		case nethtml.TextToken:
			if skipDepth == 0 {
				w.Text(string(tokenizer.Text()), preDepth > 0)
			}
		case nethtml.StartTagToken:
			name, hasAttr := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if tag == "script" || tag == "style" {
				skipDepth++
				continue
			}
			if skipDepth > 0 {
				continue
			}
			if tag == "a" {
				w.StartLink(htmlAttr(tokenizer, hasAttr, "href"))
				continue
			}
			w.StartTag(tag)
			if tag == "pre" {
				preDepth++
			}
		case nethtml.SelfClosingTagToken:
			name, _ := tokenizer.TagName()
			w.SelfClosingTag(strings.ToLower(string(name)))
		case nethtml.EndTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if skipDepth > 0 {
				if tag == "script" || tag == "style" {
					skipDepth--
				}
				continue
			}
			if tag == "pre" && preDepth > 0 {
				preDepth--
			}
			w.EndTag(tag)
		}
	}
}

type htmlTextWriter struct {
	b     strings.Builder
	links []string
}

func (w *htmlTextWriter) String() string { return w.b.String() }

func (w *htmlTextWriter) StartTag(tag string) {
	switch tag {
	case "title":
		w.ensureBlankLine()
		w.b.WriteString("# ")
	case "h1":
		w.ensureBlankLine()
		w.b.WriteString("# ")
	case "h2":
		w.ensureBlankLine()
		w.b.WriteString("## ")
	case "h3":
		w.ensureBlankLine()
		w.b.WriteString("### ")
	case "h4", "h5", "h6":
		w.ensureBlankLine()
		w.b.WriteString("#### ")
	case "li":
		w.ensureNewline()
		w.b.WriteString("- ")
	case "pre":
		w.ensureBlankLine()
		w.b.WriteString("```\n")
	case "blockquote":
		w.ensureBlankLine()
		w.b.WriteString("> ")
	case "tr":
		w.ensureNewline()
	case "td", "th":
		w.ensureCellBoundary()
	default:
		if htmlBreakTag(tag) || htmlBlockTag(tag) {
			w.ensureNewline()
		}
	}
}

func (w *htmlTextWriter) SelfClosingTag(tag string) {
	if htmlBreakTag(tag) || htmlBlockTag(tag) {
		w.ensureNewline()
	}
}

func (w *htmlTextWriter) EndTag(tag string) {
	switch tag {
	case "a":
		w.EndLink()
	case "title", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote":
		w.ensureBlankLine()
	case "pre":
		w.ensureNewline()
		w.b.WriteString("```\n")
		w.ensureBlankLine()
	case "li", "p", "tr":
		w.ensureNewline()
	default:
		if htmlBlockTag(tag) {
			w.ensureNewline()
		}
	}
}

func (w *htmlTextWriter) StartLink(href string) {
	w.links = append(w.links, strings.TrimSpace(href))
}

func (w *htmlTextWriter) EndLink() {
	if len(w.links) == 0 {
		return
	}
	href := w.links[len(w.links)-1]
	w.links = w.links[:len(w.links)-1]
	if href != "" {
		w.b.WriteString(" (" + href + ")")
	}
}

func (w *htmlTextWriter) Text(text string, pre bool) {
	text = stdhtml.UnescapeString(text)
	text = strings.ReplaceAll(text, "\u00a0", " ")
	if !pre {
		text = collapseHTMLInlineText(text)
	}
	if strings.TrimSpace(text) == "" {
		if !w.lastIsSpace() {
			w.b.WriteByte(' ')
		}
		return
	}
	if !pre && w.b.Len() > 0 && !w.lastIsSpace() && !startsWithSpaceOrPunct(text) {
		w.b.WriteByte(' ')
	}
	w.b.WriteString(text)
}

func (w *htmlTextWriter) ensureNewline() {
	if w.b.Len() == 0 || w.lastByte() == '\n' {
		return
	}
	w.b.WriteByte('\n')
}

func (w *htmlTextWriter) ensureBlankLine() {
	if w.b.Len() == 0 || strings.HasSuffix(w.b.String(), "\n\n") {
		return
	}
	w.ensureNewline()
	w.b.WriteByte('\n')
}

func (w *htmlTextWriter) ensureCellBoundary() {
	if w.b.Len() == 0 || w.lastByte() == '\n' {
		return
	}
	if !strings.HasSuffix(w.b.String(), " | ") {
		w.b.WriteString(" | ")
	}
}

func (w *htmlTextWriter) lastByte() byte {
	s := w.b.String()
	if len(s) == 0 {
		return 0
	}
	return s[len(s)-1]
}

func (w *htmlTextWriter) lastIsSpace() bool {
	if w.b.Len() == 0 {
		return false
	}
	return unicode.IsSpace(rune(w.lastByte()))
}

func normalizeHTMLText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = trailingWS.ReplaceAllString(s, "\n")
	s = multiBlank.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func collapseHTMLInlineText(s string) string {
	if s == "" {
		return ""
	}
	leading := unicode.IsSpace([]rune(s)[0])
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return " "
	}
	out := strings.Join(fields, " ")
	if leading {
		out = " " + out
	}
	return out
}

func startsWithSpaceOrPunct(s string) bool {
	for _, r := range s {
		return unicode.IsSpace(r) || strings.ContainsRune(".,;:!?)]}", r)
	}
	return false
}

func htmlAttr(tokenizer *nethtml.Tokenizer, hasAttr bool, name string) string {
	for hasAttr {
		key, val, more := tokenizer.TagAttr()
		if strings.EqualFold(string(key), name) {
			return string(val)
		}
		hasAttr = more
	}
	return ""
}

func htmlBreakTag(tag string) bool {
	switch tag {
	case "br", "hr", "img", "input":
		return true
	}
	return false
}

func htmlBlockTag(tag string) bool {
	switch tag {
	case "div", "section", "article", "header", "footer", "nav", "aside",
		"main", "figure", "figcaption", "details", "summary", "form", "fieldset":
		return true
	}
	return false
}

func contentTypeShort(ct string) string {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.TrimSpace(ct)
}

// domainMatches
// Supports wildcards: "*.example.com" matches "sub.example.com" but not
// "example.com" itself; "example.com" matches exactly.
func domainMatches(host, pattern string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		return strings.HasSuffix(host, suffix) && host != suffix[1:]
	}
	return host == pattern
}

// checkDomainPolicy applies configured domain allow/deny rules. Returns an error
// if the host is denied or not in the allowed list.
func checkDomainPolicy(host string) error {
	if searchCfg == nil {
		return nil
	}
	host = strings.ToLower(host)
	// Deny takes precedence.
	for _, d := range searchCfg.DenyDomains {
		if domainMatches(host, d) {
			return fmt.Errorf("domain %q is denied by fetch policy (matches deny rule %q)", host, d)
		}
	}
	// If allow list is non-empty, host must match one.
	if len(searchCfg.AllowDomains) > 0 {
		for _, a := range searchCfg.AllowDomains {
			if domainMatches(host, a) {
				return nil
			}
		}
		return fmt.Errorf("domain %q is not in the fetch allow list; configure [search].allow_domains to include it", host)
	}
	return nil
}
