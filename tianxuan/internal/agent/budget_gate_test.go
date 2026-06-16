package agent

import (
	"testing"

	"tianxuan/internal/provider"
)

// ─── V5.15: BudgetGate 测试 (Kun checkBudgetGate 移植) ────────────────

func TestBudgetGateBelowThreshold(t *testing.T) {
	bg := NewBudgetGate(10.0) // ¥10 budget
	pricing := &provider.Pricing{Input: 2.0, Output: 10.0}

	// 使用 ¥1.5 = 75万 miss tokens (CacheMissTokens 是 Pricing.Cost 使用的字段)
	usage := &provider.Usage{CacheMissTokens: 750_000, CompletionTokens: 0}
	status := bg.Check(pricing, usage)
	if status != BudgetOK {
		t.Errorf("below 80%% should be OK, got %s", status)
	}
	if bg.Used() < 1.0 {
		t.Errorf("used should track cumulative cost, got ¥%.4f", bg.Used())
	}
}

func TestBudgetGateWarningAt80Percent(t *testing.T) {
	bg := NewBudgetGate(10.0)
	pricing := &provider.Pricing{Input: 2.0, Output: 10.0}

	// ¥8.5 = 425万 miss tokens → Cost = 4.25M/1M * 2 = ¥8.5 (85% of ¥10)
	usage := &provider.Usage{CacheMissTokens: 4_250_000, CompletionTokens: 0}
	status := bg.Check(pricing, usage)
	if status != BudgetWarn {
		t.Errorf("at 85%% should be Warn, got %s", status)
	}
}

func TestBudgetGateBlockAt100Percent(t *testing.T) {
	bg := NewBudgetGate(10.0)
	pricing := &provider.Pricing{Input: 2.0, Output: 10.0}

	// 先用到 ¥8 (80%)
	bg.Check(pricing, &provider.Usage{CacheMissTokens: 4_000_000, CompletionTokens: 0})

	// 再用 ¥2.5 → 总计 ¥10.5 > ¥10
	status := bg.Check(pricing, &provider.Usage{CacheMissTokens: 1_250_000, CompletionTokens: 0})
	if status != BudgetBlock {
		t.Errorf("over budget should be Block, got %s", status)
	}
}

func TestBudgetGateAccumulates(t *testing.T) {
	bg := NewBudgetGate(10.0)
	pricing := &provider.Pricing{Input: 2.0, Output: 10.0}

	// 三次小调用累积
	bg.Check(pricing, &provider.Usage{CacheMissTokens: 500_000, CompletionTokens: 10_000})
	bg.Check(pricing, &provider.Usage{CacheMissTokens: 500_000, CompletionTokens: 10_000})
	bg.Check(pricing, &provider.Usage{CacheMissTokens: 500_000, CompletionTokens: 10_000})

	// 总计: 1.5M miss * ¥2 = ¥3 + 30K completion * ¥10 = ¥0.3 → ¥3.3
	if bg.Used() < 3.0 || bg.Used() > 4.0 {
		t.Errorf("cumulative cost should be ~¥3.3, got ¥%.4f", bg.Used())
	}
}

func TestBudgetGateNilPricing(t *testing.T) {
	bg := NewBudgetGate(10.0)
	// nil pricing → 不计数
	status := bg.Check(nil, &provider.Usage{CacheMissTokens: 1_000_000})
	if status != BudgetOK {
		t.Errorf("nil pricing should always be OK, got %s", status)
	}
	if bg.Used() != 0 {
		t.Errorf("nil pricing should not accumulate cost, got ¥%.4f", bg.Used())
	}
}

func TestBudgetGateReset(t *testing.T) {
	bg := NewBudgetGate(10.0)
	pricing := &provider.Pricing{Input: 2.0, Output: 10.0}

	bg.Check(pricing, &provider.Usage{CacheMissTokens: 1_000_000})
	bg.Reset()

	if bg.Used() != 0 {
		t.Errorf("after Reset, used should be 0, got ¥%.4f", bg.Used())
	}
}
