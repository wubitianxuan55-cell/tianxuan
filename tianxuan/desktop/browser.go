package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"tianxuan/internal/tool"
)

// BrowserFetchText implements the browser panel's "text mode": it fetches a URL
// and returns the cleaned readable text (HTML stripped), reusing the kernel's
// web_fetch tool so the SSRF guard, domain policy and retry logic stay in one
// place. The frontend falls back to this when a site refuses iframe embedding
// (X-Frame-Options / CSP) or when the user prefers reading text.
func (a *App) BrowserFetchText(rawURL string) (string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return "", errors.New("url is required")
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("url must be an absolute http(s) address")
	}
	t, ok := tool.LookupBuiltin("web_fetch")
	if !ok {
		return "", errors.New("web_fetch tool unavailable")
	}
	args, err := json.Marshal(map[string]string{"url": strings.TrimSpace(rawURL)})
	if err != nil {
		return "", err
	}
	return t.Execute(context.Background(), args)
}
