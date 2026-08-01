package agent

import (
	"fmt"
	"strings"
	"time"

	"tianxuan/internal/provider"
)

// recallReminderNudge is injected at the start of the first user turn when the
// session has memory artifacts. Compile-time constant for cache stability.
const recallReminderNudge = "[system] This session has saved memory — " +
	"prior context, preferences, and project facts are stored as markdown files. " +
	"Before answering, check memory with read_file or search. " +
	"Don't ask the user about something that memory may already record."

// maybeRecallReminder injects a recall nudge once when the session has memory.
// Only fires on the first call, and only when memory content is actually present
// in the session (detected via <memory-update> blocks or memQueue being non-nil
// AND the session has progressed beyond the first turn).
func (a *AgentRunner) maybeRecallReminder() {
	if a.memQueue == nil {
		return
	}
	// One-shot: only remind once per session
	if a.recallReminderFired {
		return
	}
	a.recallReminderFired = true

	// Only inject if there's actual memory content in the session
	hasMemory := false
	for _, m := range a.session.Messages {
		if strings.Contains(m.Content, "<memory-update>") ||
			strings.Contains(m.Content, "TIANXUAN.md") ||
			strings.Contains(m.Content, "project memory") {
			hasMemory = true
			break
		}
	}
	if !hasMemory {
		return
	}

	a.session.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: recallReminderNudge,
	})
}

// V10.96: maybeAutoRecall 自动搜索记忆并注入相关结果，蒸馏自 jcode 语义记忆系统。
// 消除 Agent 显式调用 memory_search 的需要 — Agent 无需主动搜索即可获得上下文。
// 与 jcode 不同，tianxuan 用 FTS5 BM25 替代向量嵌入（零外部依赖）。
// MemorySearchFunc 由 boot 在初始化时注入。
//
// V10.101: 改为会话级一次性召回 — 四域缓存下每轮注入的动态块都落在
// 缓存未命中区（user 消息之后、assistant 之前），且 MEMORY.md 索引已折入
// boot 前缀；因此只在首轮注入一次，成为稳定历史供后续轮次命中缓存。

// MemoryResult 是自动记忆检索的结果条目。
type MemoryResult struct {
	Name    string    // 记忆名称
	Preview string    // 预览描述
	Body    string    // 记忆正文（用于截断注入）
	Mtime   time.Time // 记忆文件修改时间（用于新鲜度提示）
}

// MemorySearchFunc 是注入的记忆搜索函数。boot 在初始化时设置。
var MemorySearchFunc func(query string, limit int) []MemoryResult

// maxRecallBodyChars caps the injected memory body so a single auto-recall
// block cannot blow the context budget (Qwen Code uses 1200).
const maxRecallBodyChars = 1200

func (a *AgentRunner) maybeAutoRecall() {
	if MemorySearchFunc == nil {
		return
	}
	// 会话级一次性：首轮注入后不再重复（前缀稳定优先）。
	if a.autoRecallFired {
		return
	}
	a.autoRecallFired = true
	// 获取最近一条 user 消息作为搜索查询
	query := a.lastUserContent()
	if len(query) < 5 {
		return
	}

	// FTS5 BM25 搜索记忆
	hits := MemorySearchFunc(query, 3) // 最多 3 条
	if len(hits) == 0 {
		return
	}
	var b strings.Builder
	b.WriteString("[auto-recall] 相关记忆自动检索结果:\n")
	for _, h := range hits {
		b.WriteString("- ")
		b.WriteString(h.Name)
		if h.Preview != "" {
			b.WriteString(": ")
			b.WriteString(h.Preview)
		}
		if body := truncateRecallBody(h.Body); body != "" {
			b.WriteString("\n  ")
			b.WriteString(body)
		}
		if caveat := recallFreshnessCaveat(h.Mtime); caveat != "" {
			b.WriteString("\n  ")
			b.WriteString(caveat)
		}
		b.WriteString("\n")
	}
	a.session.Add(provider.Message{
		Role:    provider.RoleUser,
		Content: b.String(),
	})
}

// truncateRecallBody caps the injected body to maxRecallBodyChars.
func truncateRecallBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	r := []rune(body)
	if len(r) <= maxRecallBodyChars {
		return body
	}
	return string(r[:maxRecallBodyChars]) + "…(truncated)"
}

// recallFreshnessCaveat returns a staleness warning for memories older than a
// day, mirroring Qwen's memoryFreshnessText: memories are point-in-time
// observations, not live state.
func recallFreshnessCaveat(mtime time.Time) string {
	if mtime.IsZero() {
		return ""
	}
	days := int(time.Since(mtime).Hours() / 24)
	if days <= 1 {
		return ""
	}
	return fmt.Sprintf("> Note: this memory is %d days old — verify against current code before relying on it.", days)
}

// lastUserContent 返回最近一条 user 消息的内容，用于记忆检索查询。
func (a *AgentRunner) lastUserContent() string {
	for i := len(a.session.Messages) - 1; i >= 0; i-- {
		if a.session.Messages[i].Role == provider.RoleUser {
			return a.session.Messages[i].Content
		}
	}
	return ""
}
