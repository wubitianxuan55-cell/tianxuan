package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
)

// CompactionConfig controls the truncation-based history pruning (V5.0).
// The LLM-based summarization has been removed; with DeepSeek 1M window
// we simply truncate old messages when the prompt exceeds a high threshold.
type CompactionConfig struct {
	Window        int     // context window in tokens (0 = disabled)
	Ratio         float64 // trigger ratio (default 0.8)
	RecentKeep    int     // min recent messages to keep verbatim
	ArchiveDir    string  // archive directory for saved sessions
	L2Dir         string  // L2 persistence directory

	// Internal state (V5.0: simplified — no LLM-based compact loop)
	LastPrompt    int  // prompt tokens from last turn
	TruncateCount int  // how many times we've truncated this session
}

const (
	defaultCompactRatio  = 0.95
	defaultTailTokens    = 131072 // V5.0: keep 64K tok tail when truncating (1M window)
	minRecentKeep        = 5     // never keep fewer than 5 recent messages
)

// maybeCompact checks if the prompt has grown too large and applies history
// truncation. Priority: legacy truncation first (cache-friendly, preserves
// prefix structure), budgeted rebuild as fallback when truncation can't
// reduce context enough.
//
// V7.3: swapped fallback order — legacyTruncate first, Budgeted second.
// Legacy truncation preserves [L1 + prefix + summary + tail] structure
// so DeepSeek can cache L1 + the first N messages across compactions.
// Budgeted rebuild produces a completely different message layout and
// drops cache hit rate from 95% to near 0%.
func (a *AgentRunner) maybeCompact(ctx context.Context, u *provider.Usage) {
	if a.compaction.Window <= 0 {
		return
	}
	prompt := 0
	if u != nil {
		prompt = u.PromptTokens
	}
	if prompt == 0 {
		prompt = a.compaction.LastPrompt
	}
	if prompt == 0 {
		return
	}
	high := int(float64(a.compaction.Window) * a.compaction.Ratio)
	if prompt < high {
		a.compaction.LastPrompt = prompt
		a.consecutiveCompacts = 0
		a.compactStuck = false
		return
	}

	// Truncate: keep system + first user + last N messages
	msgs := a.session.Messages
	if len(msgs) <= minRecentKeep+2 {
		return
	}

	// V5.13: 三级压缩
	keep := a.compaction.RecentKeep
	if keep < minRecentKeep {
		keep = minRecentKeep
	}
	mode := "normal"
	aggressiveThreshold := high + int(float64(a.compaction.Window-high)*0.6)
	if prompt >= a.compaction.Window {
		mode = "force"
		if keep > 1 {
			keep = 1
		}
	} else if prompt >= aggressiveThreshold {
		mode = "aggressive"
		if keep > 2 {
			keep = 2
		}
	}
	if keep > len(msgs)-1 {
		keep = len(msgs) - 1
	}

	// V5.9: tool_use/tool_result 配对保护
	keepFrom := len(msgs) - keep
	for keepFrom > 1 && keepFrom < len(msgs) {
		firstPreserved := msgs[keepFrom]
		if firstPreserved.Role != provider.RoleTool {
			break
		}
		prev := msgs[keepFrom-1]
		if prev.Role == provider.RoleAssistant && len(prev.ToolCalls) > 0 {
			break
		}
		keepFrom--
	}
	keep = len(msgs) - keepFrom

	// V7.3 DSR improve: compactStuck 不静默返回，降级到纯截断（force mode）
	// 不注入 checkpoint/memory/tasks，最大化缓存连续性
	if a.compactStuck {
		a.legacyTruncate(msgs, keepFrom, 1, prompt, "force")
		return
	}

	// ─── Strategy ①: Legacy Truncate first (cache-friendly) ─────────────
	// 保留 [L1] + [前 N 条 prefix] + [摘要] + [tail]
	// DeepSeek 至少可缓存 L1 + prefix 部分，只断一次
	prefixCount := 0
	switch mode {
	case "normal":
		prefixCount = a.compaction.RecentKeep
		if prefixCount > keepFrom-2 {
			prefixCount = keepFrom - 2
		}
	case "aggressive", "force":
		prefixCount = 2
		if prefixCount > keepFrom-2 {
			prefixCount = keepFrom - 2
		}
	}
	if prefixCount < 0 {
		prefixCount = 0
	}

	middleStart := 1 + prefixCount
	if middleStart < 1 {
		middleStart = 1
	}
	summary := BuildCompactSummary(msgs[middleStart:keepFrom])

	prefixEnd := 1 + prefixCount
	legacyRep := make([]provider.Message, 0, 2+prefixCount+keep)
	legacyRep = append(legacyRep, msgs[0])
	if prefixEnd > 1 {
		legacyRep = append(legacyRep, msgs[1:prefixEnd]...)
	}
	if summary != "" {
		legacyRep = append(legacyRep, provider.Message{
			Role:    provider.RoleUser,
			Content: summary,
		})
	}
	legacyRep = append(legacyRep, msgs[keepFrom:]...)

	if len(legacyRep) < len(msgs) {
		a.session.Replace(legacyRep)
		a.compaction.TruncateCount++
		a.compaction.LastPrompt = 0
		a.consecutiveCompacts = 0
		a.compactStuck = false
		a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
			Text: "history truncated [" + mode + "] - " + itoa(len(msgs)) + " -> " + itoa(len(legacyRep)) + " messages, prefix " + itoa(prefixCount) + " kept"})
		return
	}

	// ─── Strategy ②: Budgeted Rebuild (fallback) ───────────────────────
	// Legacy truncation didn't reduce context — try budgeted rebuild.
	cpSummary := BuildCompactSummary(msgs[1:keepFrom])
	cpTodos := extractTodos(msgs)

	type budgetComponent struct {
		name       string
		content    string
		importance float64
		budgetPct  float64
	}

	tpc := a.tokPerChar()

	overhead := 0
	if len(msgs) > 0 {
		overhead += int(float64(msgChars(msgs[0]))*tpc) + 1
	}
	budget := high - overhead - 100
	if budget < 200 {
		budget = 200
	}

	components := []budgetComponent{
		{name: "recent", content: "", importance: 1.0, budgetPct: 0.45},
		{name: "checkpoint", content: cpSummary, importance: 0.9, budgetPct: 0.30},
		{name: "memory", content: a.buildMemoryContext(), importance: 0.7, budgetPct: 0.15},
		{name: "tasks", content: formatTodosForBudget(cpTodos), importance: 0.5, budgetPct: 0.10},
	}

	totalWeight := 0.0
	for _, c := range components {
		totalWeight += c.importance
	}

	replacement := make([]provider.Message, 0, 2+len(components))
	replacement = append(replacement, msgs[0])
	if len(msgs) > 1 && msgs[1].Role == provider.RoleSystem {
		replacement = append(replacement, msgs[1])
	}

	sort.Slice(components, func(i, j int) bool {
		return components[i].importance > components[j].importance
	})

	allocations := make([]int, len(components))
	for i, c := range components {
		allocations[i] = int(float64(budget) * (c.importance / totalWeight))
	}

	tailMsgs := msgs[keepFrom:]
	usedBudget := 0
	for i, c := range components {
		if c.name == "recent" {
			continue
		}
		if c.content == "" {
			continue
		}
		alloc := allocations[i]
		runes := []rune(c.content)
		if int(float64(len(runes))*tpc) > alloc {
			maxRunes := alloc * 4
			if maxRunes < 50 {
				maxRunes = 50
			}
			if maxRunes < len(runes) {
				runes = runes[:maxRunes]
				c.content = string(runes) + "\n[truncated]"
			}
		}
		if c.content != "" {
			replacement = append(replacement, provider.Message{
				Role:    provider.RoleUser,
				Content: "[Context: " + c.name + "]\n" + c.content,
			})
		}
		usedBudget += int(float64(len(c.content)) * tpc)
	}

	recentBudget := budget - usedBudget
	if recentBudget < 50 {
		recentBudget = 50
	}
	tailAdded := 0
	for _, m := range tailMsgs {
		mTok := int(float64(msgChars(m)) * tpc)
		if tailAdded+mTok > recentBudget && tailAdded > 0 {
			break
		}
		tailAdded += mTok
		replacement = append(replacement, m)
	}

	if len(replacement) >= len(msgs) {
		// Budgeted rebuild also didn't reduce context.
		a.consecutiveCompacts++
		if a.consecutiveCompacts >= 2 {
			a.compactStuck = true
			a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
				Text: fmt.Sprintf("context window too small for compaction to help (system prompt + one turn exceeds %.0f%% of %d); raise context_window or shrink tool output. Auto-compaction paused.",
					a.compaction.Ratio*100, a.compaction.Window)})
		}
		return
	}

	a.session.Replace(replacement)
	a.compaction.TruncateCount++
	a.compaction.LastPrompt = 0
	a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
		Text: fmt.Sprintf("budgeted rebuild [%s] — %d→%d msgs (checkpoint:%d mem:%d task:%d tail:%d tok)",
			mode, len(msgs), len(replacement),
			int(float64(len(cpSummary))*a.tokPerChar()),
			int(float64(len(a.buildMemoryContext()))*a.tokPerChar()),
			int(float64(len(formatTodosForBudget(cpTodos)))*a.tokPerChar()), tailAdded)})

	if a.memQueue != nil && cpSummary != "" {
		a.memQueue.QueueMemory("checkpoint [" + mode + "]: " + truncateText(cpSummary, 300))
	}
}

// buildMemoryContext returns a compact memory summary string for budgeted injection.
// Returns empty string when no memory is available.
func (a *AgentRunner) buildMemoryContext() string {
	if a.memQueue == nil {
		return ""
	}
	// Memory is stored in session messages; extract recent memory-updates
	var notes []string
	for _, m := range a.session.Messages {
		if strings.Contains(m.Content, "<memory-update>") {
			notes = append(notes, truncateText(m.Content, 200))
		}
	}
	if len(notes) == 0 {
		return ""
	}
	return "Project memory:\n" + joinStr(notes, "\n")
}

// formatTodosForBudget formats checkpoint todos for budgeted injection.
func formatTodosForBudget(todos []checkpointTodo) string {
	if len(todos) == 0 {
		return ""
	}
	var parts []string
	parts = append(parts, "Task progress:")
	for _, t := range todos {
		icon := "○"
		switch t.Status {
		case "completed":
			icon = "✓"
		case "in_progress":
			icon = "▶"
		}
		parts = append(parts, "  "+icon+" "+t.Content)
	}
	return joinStr(parts, "\n")
}

// V5.0 legacy: simple truncation fallback (used when budgeted rebuild doesn't reduce context).
func (a *AgentRunner) legacyTruncate(msgs []provider.Message, keepFrom, keep, prompt int, mode string) {
	prefixCount := 0
	switch mode {
	case "normal":
		prefixCount = a.compaction.RecentKeep
		if prefixCount > keepFrom-2 { prefixCount = keepFrom - 2 }
	case "aggressive", "force":
		prefixCount = 2
		if prefixCount > keepFrom-2 { prefixCount = keepFrom - 2 }
	}
	if prefixCount < 0 { prefixCount = 0 }

	middleStart := 1 + prefixCount
	if middleStart < 1 { middleStart = 1 }
	summary := BuildCompactSummary(msgs[middleStart:keepFrom])

	// V7.3: digest marker removed — it served no purpose other than proving
	// compaction changed the prefix, which the cache miss already proves.

	prefixEnd := 1 + prefixCount
	replacement := make([]provider.Message, 0, 2+prefixCount+keep)
	replacement = append(replacement, msgs[0])
	if prefixEnd > 1 {
		replacement = append(replacement, msgs[1:prefixEnd]...)
	}
	if summary != "" {
		replacement = append(replacement, provider.Message{
			Role:    provider.RoleUser,
			Content: summary,
		})
	}
	replacement = append(replacement, msgs[keepFrom:]...)

	a.session.Replace(replacement)
	a.compaction.TruncateCount++
	a.compaction.LastPrompt = 0

	a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
		Text: "history truncated [" + mode + "] - " + itoa(len(msgs)) + " -> " + itoa(len(replacement)) + " messages, prefix " + itoa(prefixCount) + " kept"})
}

// buildCompactSummary 从被截断的消息中提取确定性摘要（V5.9: claw-code 风格）。
// 提取：用户请求、工具统计、编辑文件、待办项、关键文件、最近工作。
// 完全确定性：相同输入 → 相同输出，不影响缓存稳定性。
func BuildCompactSummary(truncated []provider.Message) string {
	if len(truncated) == 0 {
		return ""
	}

	// 统计
	var filesEdited []string
	seenFiles := make(map[string]bool)
	toolCounts := make(map[string]int)
	turnCount := 0
	var recentUserReqs []string    // 最近 3 条用户请求
	var pendingItems []string      // 待办项（含 todo/next/pending/follow up）
	var keyFiles []string          // 引用到的关键文件
	seenKeyFiles := make(map[string]bool)

	for _, msg := range truncated {
		switch msg.Role {
		case provider.RoleUser:
			if msg.Content != "" && !strings.HasPrefix(msg.Content, "[") {
				turnCount++
				// 收集最近用户请求（最多5条）
				short := truncateText(msg.Content, 160)
				if short != "" {
					recentUserReqs = append(recentUserReqs, short)
				}
			}
		case provider.RoleAssistant:
			// 工具统计
			for _, tc := range msg.ToolCalls {
				toolCounts[tc.Name]++
				// 提取编辑操作的文件路径
				switch tc.Name {
				case "edit_file", "write_file", "multi_edit", "delete_range", "delete_symbol":
					path := extractFilePath(tc.Name, tc.Arguments)
					if path != "" && !seenFiles[path] {
						filesEdited = append(filesEdited, path)
						seenFiles[path] = true
					}
				}
			}
			// 检测待办项
			lower := strings.ToLower(msg.Content)
			for _, kw := range []string{"todo", "next", "pending", "follow up", "remaining"} {
				if strings.Contains(lower, kw) {
					short := truncateText(msg.Content, 160)
					if short != "" {
						pendingItems = append(pendingItems, short)
					}
					break
				}
			}
		}
		// 提取关键文件路径
		for _, fp := range extractKeyFiles(msg) {
			if !seenKeyFiles[fp] {
				keyFiles = append(keyFiles, fp)
				seenKeyFiles[fp] = true
			}
		}
	}

	if turnCount == 0 && len(toolCounts) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[Earlier conversation summary:\n")

	// 概览
	sb.WriteString("- Scope: ")
	sb.WriteString(itoa(len(truncated)))
	sb.WriteString(" messages compacted, ")
	sb.WriteString(itoa(turnCount))
	sb.WriteString(" turns\n")

	// 最近用户请求（最后 3 条）
	if len(recentUserReqs) > 0 {
		limit := len(recentUserReqs)
		if limit > 3 {
			limit = 3
		}
		start := len(recentUserReqs) - limit
		if start < 0 {
			start = 0
		}
		sb.WriteString("- Recent requests:\n")
		for _, req := range recentUserReqs[start:] {
			sb.WriteString("  - ")
			sb.WriteString(req)
			sb.WriteString("\n")
		}
	}

	// 编辑文件
	if len(filesEdited) > 0 {
		sb.WriteString("- Files modified: ")
		limit := len(filesEdited)
		if limit > 8 {
			limit = 8
		}
		for i := 0; i < limit; i++ {
			if i > 0 {
				sb.WriteString(", ")
			}
			short := filesEdited[i]
			if idx := strings.LastIndex(short, "/"); idx >= 0 {
				short = short[idx+1:]
			}
			sb.WriteString(short)
		}
		if len(filesEdited) > 8 {
			sb.WriteString(", ...")
		}
		sb.WriteString("\n")
	}

	// 工具使用
	if len(toolCounts) > 0 {
		sb.WriteString("- Tools used: ")
		type tc struct {
			name  string
			count int
		}
		var sorted []tc
		for name, count := range toolCounts {
			sorted = append(sorted, tc{name, count})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
		for i, t := range sorted {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(t.name)
			sb.WriteString("×")
			sb.WriteString(itoa(t.count))
		}
		sb.WriteString("\n")
	}

	// 待办项
	if len(pendingItems) > 0 {
		sb.WriteString("- Pending work:\n")
		limit := len(pendingItems)
		if limit > 3 {
			limit = 3
		}
		for _, item := range pendingItems[:limit] {
			sb.WriteString("  - ")
			sb.WriteString(item)
			sb.WriteString("\n")
		}
	}

	// 关键文件
	if len(keyFiles) > 0 {
		sb.WriteString("- Key files: ")
		limit := len(keyFiles)
		if limit > 8 {
			limit = 8
		}
		for i, f := range keyFiles[:limit] {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(f)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("]")
	return sb.String()
}

// truncateText 截断文本到 maxChars 字符，超长加 …。
func truncateText(s string, maxChars int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars]) + "…"
}

// extractKeyFiles 从消息中提取引用的文件路径。
func extractKeyFiles(msg provider.Message) []string {
	var texts []string
	if msg.Content != "" {
		texts = append(texts, msg.Content)
	}
	for _, tc := range msg.ToolCalls {
		texts = append(texts, tc.Arguments)
	}
	var files []string
	seen := make(map[string]bool)
	for _, text := range texts {
		for _, token := range strings.Fields(text) {
			token = strings.Trim(token, `,.:;()"'` + "`")
			if !strings.Contains(token, "/") {
				continue
			}
			// 检查是否有已知代码文件扩展名
			hasExt := false
			for _, ext := range []string{".go", ".ts", ".tsx", ".js", ".py", ".rs", ".java", ".md", ".json", ".yaml", ".yml", ".toml"} {
				if strings.HasSuffix(token, ext) {
					hasExt = true
					break
				}
			}
			if hasExt && !seen[token] {
				files = append(files, token)
				seen[token] = true
			}
		}
	}
	return files
}

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
