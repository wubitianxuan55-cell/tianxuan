package agent

import (
	"fmt"
	"sort"
)

// TokenBudget tracks cumulative token usage across a session and injects
// progressive reminders when the remaining budget crosses configured
// thresholds (distilled from codex CLI's rollout_budget). Unlike the cost
// BudgetGate (a hard ¥-based stop), this is a soft planning signal: the model
// sees "N tokens left in the shared session budget" and can choose to converge
// (stop exploring, wrap up, narrow scope) before the budget runs out.
//
// 缓存铁律: the reminder is injected as an ordinary user message in the
// session stream — it never touches the tools schema list, so the prefix
// cache invariant holds.
type TokenBudget struct {
	limit     int64   // cumulative token cap; 0 = disabled
	reminders []int64 // remaining-token thresholds, sorted descending
	used      int64   // cumulative tokens consumed this session
	delivered int     // how many thresholds have fired (index into reminders)
}

// NewTokenBudget builds a tracker. limit <= 0 disables reminders. thresholds
// may be given in any order; they are sorted descending internally.
func NewTokenBudget(limit int64, thresholds []int64) *TokenBudget {
	rs := append([]int64(nil), thresholds...)
	sort.Slice(rs, func(i, j int) bool { return rs[i] > rs[j] })
	return &TokenBudget{limit: limit, reminders: rs}
}

// Check records a turn's token usage and, when the remaining budget has just
// crossed one of the configured thresholds, returns a reminder message for the
// model. turnTokens is this turn's consumption (accumulated internally);
// cumulativeTokens, when > 0, overrides the internal accumulator with the
// provider's session-cumulative figure. ok=false means nothing to inject. It
// is safe for concurrent use by the run loop.
func (b *TokenBudget) Check(turnTokens, cumulativeTokens int64) (string, bool) {
	if b == nil || b.limit <= 0 {
		return "", false
	}
	if cumulativeTokens > 0 {
		b.used = cumulativeTokens
	} else {
		b.used += turnTokens
	}
	remaining := b.limit - b.used
	if remaining <= 0 {
		return fmt.Sprintf("[system] 会话 token 预算已耗尽（上限 %d tokens）。请立即收尾：总结已完成的改动、明确剩余未做事项，不要再启动新的探索或大改。", b.limit), true
	}
	for b.delivered < len(b.reminders) {
		threshold := b.reminders[b.delivered]
		if remaining > threshold {
			break
		}
		b.delivered++
		return fmt.Sprintf("[system] 会话 token 预算剩余 %d tokens（上限 %d）。请评估当前进度：若任务接近完成则收敛收尾；若仍需大量工作，考虑拆分为后续会话。", remaining, b.limit), true
	}
	return "", false
}

// Used returns the cumulative tokens consumed so far.
func (b *TokenBudget) Used() int64 {
	if b == nil {
		return 0
	}
	return b.used
}
