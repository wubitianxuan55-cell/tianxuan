// Package budget tracks per-session API cost and model context profiles.
package budget

import (
	"fmt"
	"sync"

	"tianxuan/internal/provider"
)

// Status is the result of a budget check.
type Status int

const (
	OK    Status = iota // within budget
	Warn                // ≥80% used
	Block               // ≥100% used
)

func (s Status) String() string {
	switch s {
	case OK:
		return "ok"
	case Warn:
		return "warn"
	case Block:
		return "block"
	default:
		return "unknown"
	}
}

// Gate tracks cumulative API cost across a session, warning/blocking at
// thresholds. Thread-safe — Check may be called from multiple goroutines.
type Gate struct {
	mu      sync.Mutex
	limit   float64
	used    float64
	warned  bool
	blocked bool
}

const warnRatio = 0.8

// NewGate creates a budget gate.
// limit is the total session budget in yuan; ≤0 means unlimited.
func NewGate(limit float64) *Gate {
	return &Gate{limit: limit}
}

// Check updates cumulative cost from usage and returns the status.
// pricing is nil means don't count (can't compute cost).
func (b *Gate) Check(pricing *provider.Pricing, usage *provider.Usage) Status {
	if b.limit <= 0 || usage == nil {
		return OK
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if pricing != nil {
		b.used += pricing.Cost(usage)
	}

	if b.used >= b.limit {
		b.blocked = true
		return Block
	}
	if b.used >= b.limit*warnRatio {
		b.warned = true
		return Warn
	}
	return OK
}

// Used returns the current cumulative cost.
func (b *Gate) Used() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

// Reset zeroes cumulative cost (called at the start of a new session).
func (b *Gate) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.used = 0
	b.warned = false
	b.blocked = false
}

// Limit returns the budget cap.
func (b *Gate) Limit() float64 { return b.limit }

// StatusMessage returns a human-readable status string.
func (b *Gate) StatusMessage() string {
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
