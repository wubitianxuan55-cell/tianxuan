package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ─── V9.2: AnySearch API 客户端（借鉴 anysearch-skill）───────────────

const anysearchEndpoint = "https://api.anysearch.com/mcp"
const anysearchTimeout = 25 * time.Second

// anysearchCall 调用 AnySearch JSON-RPC API。
// toolName: "search" | "extract" | "batch_search" | "get_sub_domains"
// 返回 result.content[0].text；失败时返回 error。
func anysearchCall(ctx context.Context, toolName string, args map[string]any) (string, error) {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("anysearch marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anysearchEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("anysearch request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tianxuan/9.2")

	// API Key（可选）: 环境变量 > .env 文件
	if key := anysearchAPIKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	client := &http.Client{Timeout: anysearchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("anysearch %s: %w", toolName, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MB cap
	if err != nil {
		return "", fmt.Errorf("anysearch read: %w", err)
	}

	var result struct {
		Error  *json.RawMessage `json:"error"`
		Result *struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("anysearch parse: %w (body=%.200s)", err, string(respBody))
	}
	if result.Error != nil {
		return "", fmt.Errorf("anysearch error: %s", string(*result.Error))
	}
	if result.Result == nil {
		return "", fmt.Errorf("anysearch: empty result (body=%.200s)", string(respBody))
	}
	for _, c := range result.Result.Content {
		if c.Type == "text" {
			return c.Text, nil
		}
	}
	// fallback: marshal whole result
	fallback, _ := json.Marshal(result.Result)
	return string(fallback), nil
}

// anysearchAPIKey 从环境变量或 .env 文件读取 API Key。
func anysearchAPIKey() string {
	if k := os.Getenv("ANYSEARCH_API_KEY"); k != "" {
		return k
	}
	// 尝试读取 .env 文件（当前目录 + skill 目录）
	for _, path := range []string{".env", "../.env"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
				continue
			}
			kv := strings.SplitN(line, "=", 2)
			if strings.TrimSpace(kv[0]) == "ANYSEARCH_API_KEY" {
				return strings.Trim(strings.TrimSpace(kv[1]), "\"'")
			}
		}
	}
	return ""
}

// anysearchRetry 带指数退避重试的 AnySearch 调用。
// 最多 3 次尝试: 1s → 2s → 4s
func anysearchRetry(ctx context.Context, toolName string, args map[string]any) (string, error) {
	var lastErr error
	backoff := 1 * time.Second
	for attempt := 0; attempt < 3; attempt++ {
		result, err := anysearchCall(ctx, toolName, args)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt < 2 {
			select {
			case <-time.After(backoff):
				backoff *= 2
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
	}
	return "", fmt.Errorf("anysearch %s failed after 3 attempts: %w", toolName, lastErr)
}
