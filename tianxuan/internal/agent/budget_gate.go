package agent

import (
	"fmt"
	"sync"

	"tianxuan/internal/provider"
)

// ─── V5.15: BudgetGate (Kun checkBudgetGate 移植) ─────────────────────

// BudgetStatus 是预算检查结果。
type BudgetStatus int

const (
	BudgetOK    BudgetStatus = iota // 预算充足
	BudgetWarn                      // 已用 ≥80%，发出警告
	BudgetBlock                     // 已用 ≥100%，阻断
)

func (s BudgetStatus) String() string {
	switch s {
	case BudgetOK:
		return "ok"
	case BudgetWarn:
		return "warn"
	case BudgetBlock:
		return "block"
	default:
		return "unknown"
	}
}

// BudgetGate 追踪会话累计 API 费用，在阈值处警告/阻断。
// 线程安全——Check 可在多个 goroutine 中调用。
type BudgetGate struct {
	mu      sync.Mutex
	limit   float64 // 预算上限（元）
	used    float64 // 已使用（元）
	warned  bool    // 是否已发出过警告
	blocked bool    // 是否已阻断
}

const budgetWarnRatio = 0.8

// NewBudgetGate 创建预算门控。
// limit 为会话总预算（元），<=0 表示无限制。
func NewBudgetGate(limit float64) *BudgetGate {
	return &BudgetGate{limit: limit}
}

// Check 根据本次 usage 更新累计成本并返回状态。
// pricing 为 nil 时不计数（无法计算成本）。
func (b *BudgetGate) Check(pricing *provider.Pricing, usage *provider.Usage) BudgetStatus {
	if b.limit <= 0 || usage == nil {
		return BudgetOK
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if pricing != nil {
		b.used += pricing.Cost(usage)
	}

	if b.used >= b.limit {
		b.blocked = true
		return BudgetBlock
	}
	if b.used >= b.limit*budgetWarnRatio {
		b.warned = true
		return BudgetWarn
	}
	return BudgetOK
}

// Used 返回当前累计费用。
func (b *BudgetGate) Used() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

// Reset 清零累计费用（新会话开始时调用）。
func (b *BudgetGate) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.used = 0
	b.warned = false
	b.blocked = false
}

// Limit 返回预算上限。
func (b *BudgetGate) Limit() float64 { return b.limit }

// StatusMessage 返回人类可读的状态消息。
func (b *BudgetGate) StatusMessage() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.blocked {
		return fmt.Sprintf("预算已耗尽: ¥%.2f / ¥%.2f (100%%)", b.used, b.limit)
	}
	if b.warned {
		return fmt.Sprintf("预算警告: ¥%.2f / ¥%.2f (%.0f%%)", b.used, b.limit, b.used/b.limit*100)
	}
	return fmt.Sprintf("预算: ¥%.2f / ¥%.2f", b.used, b.limit)
}
