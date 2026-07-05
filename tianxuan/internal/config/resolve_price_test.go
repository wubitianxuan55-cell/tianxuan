package config

import (
	"testing"

	"github.com/BurntSushi/toml"
)

// TestResolveModelPricing_ProviderModelFormat verifies ResolveModel returns
// non-nil Price for "provider/model" format with single-model providers.
func TestResolveModelPricing_ProviderModelFormat(t *testing.T) {
	const userConfig = `
default_model = "deepseek-pro"

[[providers]]
name        = "deepseek-flash"
model       = "deepseek-v4-flash"
price       = { cache_hit = 0.020, input = 1.0, output = 2.0, currency = "¥" }

[[providers]]
name        = "deepseek-pro"
model       = "deepseek-v4-pro"
price       = { cache_hit = 0.025, input = 3.0, output = 6.0, currency = "¥" }
`
	var cfg Config
	if _, err := toml.Decode(userConfig, &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Resolve by provider name (single-model provider with "price" field)
	entry, ok := cfg.ResolveModel("deepseek-pro")
	if !ok {
		t.Fatal("ResolveModel returned not ok for deepseek-pro")
	}
	if entry.Price == nil {
		t.Fatal("Price is nil — cost will be 0")
	}
	if entry.Price.Input <= 0 {
		t.Errorf("Price.Input = %v, want > 0", entry.Price.Input)
	}
	t.Logf("Resolved: provider=%q model=%q price.input=%v",
		entry.Name, entry.Model, entry.Price.Input)
}

// TestResolveModelPricing_WithMultiModelProvider verifies resolution when
// a multi-model provider using "prices" (map) co-exists with single-model
// providers using "price" (scalar). The "deepseek" provider uses "prices"
// which maps to nothing in Go struct — so HasModel returns false for it,
// and resolution falls through to single-model providers.
func TestResolveModelPricing_WithMultiModelProvider(t *testing.T) {
	const userConfig = `
default_model = "deepseek/deepseek-v4-pro"

[[providers]]
name        = "deepseek"
prices      = { "deepseek-v4-flash" = { cache_hit = 0.020, input = 1.0, output = 2.0, currency = "¥" }, "deepseek-v4-pro" = { cache_hit = 0.025, input = 3.0, output = 6.0, currency = "¥" } }

[[providers]]
name        = "deepseek-pro"
model       = "deepseek-v4-pro"
price       = { cache_hit = 0.025, input = 3.0, output = 6.0, currency = "¥" }
`
	var cfg Config
	if _, err := toml.Decode(userConfig, &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// "deepseek" provider has no models/model field (only prices map),
	// so HasModel returns false. Resolution should fall through to deepseek-pro.
	entry, ok := cfg.ResolveModel("deepseek/deepseek-v4-pro")
	if !ok {
		t.Fatal("ResolveModel returned not ok — resolution failed entirely")
	}
	if entry.Name != "deepseek-pro" {
		t.Logf("Resolved provider: %q (expected deepseek-pro)", entry.Name)
	}
	if entry.Price == nil {
		t.Fatal("Price is nil — planner cost will be 0. TOML 'prices' map is likely not parsed.")
	}
	t.Logf("Resolved: provider=%q model=%q price=%+v",
		entry.Name, entry.Model, entry.Price)
}
