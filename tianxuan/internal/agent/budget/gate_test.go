package budget

import (
	"testing"

	"tianxuan/internal/provider"
)

func TestGateBelowThreshold(t *testing.T) {
	bg := NewGate(10.0) // ¥10 budget
	pricing := &provider.Pricing{Input: 2.0, Output: 10.0}

	usage := &provider.Usage{CacheMissTokens: 750_000, CompletionTokens: 0}
	status := bg.Check(pricing, usage)
	if status != OK {
		t.Errorf("below 80%% should be OK, got %s", status)
	}
	if bg.Used() < 1.0 {
		t.Errorf("used should track cumulative cost, got ¥%.4f", bg.Used())
	}
}

func TestGateWarningAt80Percent(t *testing.T) {
	bg := NewGate(10.0)
	pricing := &provider.Pricing{Input: 2.0, Output: 10.0}

	usage := &provider.Usage{CacheMissTokens: 4_250_000, CompletionTokens: 0}
	status := bg.Check(pricing, usage)
	if status != Warn {
		t.Errorf("at 85%% should be Warn, got %s", status)
	}
}

func TestGateBlockAt100Percent(t *testing.T) {
	bg := NewGate(10.0)
	pricing := &provider.Pricing{Input: 2.0, Output: 10.0}

	bg.Check(pricing, &provider.Usage{CacheMissTokens: 4_000_000, CompletionTokens: 0})
	status := bg.Check(pricing, &provider.Usage{CacheMissTokens: 1_250_000, CompletionTokens: 0})
	if status != Block {
		t.Errorf("over budget should be Block, got %s", status)
	}
}

func TestGateAccumulates(t *testing.T) {
	bg := NewGate(10.0)
	pricing := &provider.Pricing{Input: 2.0, Output: 10.0}

	bg.Check(pricing, &provider.Usage{CacheMissTokens: 500_000, CompletionTokens: 10_000})
	bg.Check(pricing, &provider.Usage{CacheMissTokens: 500_000, CompletionTokens: 10_000})
	bg.Check(pricing, &provider.Usage{CacheMissTokens: 500_000, CompletionTokens: 10_000})

	if bg.Used() < 3.0 || bg.Used() > 4.0 {
		t.Errorf("cumulative cost should be ~¥3.3, got ¥%.4f", bg.Used())
	}
}

func TestGateNilPricing(t *testing.T) {
	bg := NewGate(10.0)
	status := bg.Check(nil, &provider.Usage{CacheMissTokens: 1_000_000})
	if status != OK {
		t.Errorf("nil pricing should always be OK, got %s", status)
	}
	if bg.Used() != 0 {
		t.Errorf("nil pricing should not accumulate cost, got ¥%.4f", bg.Used())
	}
}

func TestGateReset(t *testing.T) {
	bg := NewGate(10.0)
	pricing := &provider.Pricing{Input: 2.0, Output: 10.0}

	bg.Check(pricing, &provider.Usage{CacheMissTokens: 1_000_000})
	bg.Reset()

	if bg.Used() != 0 {
		t.Errorf("after Reset, used should be 0, got ¥%.4f", bg.Used())
	}
}
