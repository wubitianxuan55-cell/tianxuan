package boot

import (
	"testing"

	"tianxuan/internal/config"
	"tianxuan/internal/provider/failover"

	_ "tianxuan/internal/provider/openai"
)

// TestNewProviderBuildsFailoverChain 验证配置了 Fallbacks 的 entry 返回
// failover 链（蒸馏自 OpenClaw model-failover 的接线）。
func TestNewProviderBuildsFailoverChain(t *testing.T) {
	entry := &config.ProviderEntry{
		Name:      "deepseek",
		Kind:      "openai",
		BaseURL:   "https://example.invalid",
		Model:     "deepseek-v4-pro",
		Fallbacks: []string{"deepseek-flash"},
	}
	p, err := NewProvider(entry)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	chain, ok := p.(*failover.Chain)
	if !ok {
		t.Fatalf("entry with fallbacks must build a *failover.Chain, got %T", p)
	}
	if chain.Name() != "deepseek" {
		t.Errorf("chain Name() = %q, want primary instance name %q", chain.Name(), "deepseek")
	}
}

// TestNewProviderSkipsEmptyAndDuplicateFallbacks 验证空/重复 fallback 被跳过，
// 全部无效时退化为普通 provider。
func TestNewProviderSkipsEmptyAndDuplicateFallbacks(t *testing.T) {
	entry := &config.ProviderEntry{
		Name:      "deepseek",
		Kind:      "openai",
		BaseURL:   "https://example.invalid",
		Model:     "deepseek-v4-pro",
		Fallbacks: []string{"", "deepseek-v4-pro"},
	}
	p, err := NewProvider(entry)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, ok := p.(*failover.Chain); ok {
		t.Fatal("empty/duplicate fallbacks must not build a chain")
	}
}

// TestNewProviderNoFallbacksReturnsPlainProvider 验证无 fallback 时不包装链，
// 保持原有行为（不增加任何开销）。
func TestNewProviderNoFallbacksReturnsPlainProvider(t *testing.T) {
	entry := &config.ProviderEntry{
		Name:    "deepseek",
		Kind:    "openai",
		BaseURL: "https://example.invalid",
		Model:   "deepseek-v4-pro",
	}
	p, err := NewProvider(entry)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if _, ok := p.(*failover.Chain); ok {
		t.Fatal("no fallbacks must not build a chain")
	}
}
