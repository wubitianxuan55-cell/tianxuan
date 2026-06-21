package openai

import (
	"net/url"
	"strings"
)

// matchesVendorHost 判断 baseURL 是否指向指定厂商的 API 主机名（精确匹配）
// 或任意子域名。apex 的子域名（如 eu.deepseek.com）也会匹配，但裸 apex
// 本身（如 deepseek.com）不会匹配——裸 apex 通常是官网而非 API 端点。
func matchesVendorHost(baseURL, apex string, canonical ...string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, c := range canonical {
		if host == c {
			return true
		}
	}
	return strings.HasSuffix(host, "."+apex)
}

// IsDeepSeek 判断 baseURL 是否指向 DeepSeek API
// (api.deepseek.com 或 *.deepseek.com 子域名)。
func IsDeepSeek(baseURL string) bool {
	return matchesVendorHost(baseURL, "deepseek.com", "api.deepseek.com")
}

// IsMiniMax 判断 baseURL 是否指向 MiniMax 的 OpenAI 兼容端点
// (api.minimaxi.com 或 *.minimaxi.com 子域名)。
func IsMiniMax(baseURL string) bool {
	return matchesVendorHost(baseURL, "minimaxi.com", "api.minimaxi.com")
}
