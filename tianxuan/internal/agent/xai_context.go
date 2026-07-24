package agent

import (
	"fmt"
	"strings"
	"sync"

	"tianxuan/internal/provider"
)

// xaiContext 实现 XAI 规划者的分层上下文管理。
//
// 分层模型：
//   L0: 系统提示词 — 固化不变
//   L1: 项目骨架 — 探索后缓存
//   L2: 任务上下文 — Goal + 用户反馈
//   L3: 探索发现 — 工具输出的压缩摘要
//   L4: 对话历史 — 消息列表
//
// 压缩策略（借鉴 Headroom live-zone 压缩）：
//   - 只压缩 L3 和 L4 中的新增内容，L0-L2 前缀不动
//   - 不丢弃消息，而是压缩保留（摘要替代正文）
//   - 内容感知：grep/read_file/glob 各有专用压缩器
//   - 重要性评分：错误消息 > 近期消息 > 普通消息
type xaiContext struct {
	mu sync.Mutex

	// L0: 系统提示词（固化）
	systemMessages []provider.Message

	// L1: 项目骨架
	projectSkeleton string

	// L2: 任务上下文
	goal        string
	constraints []string
	userNotes   []string

	// L3: 探索发现（压缩后的关键发现）
	discoveries []string

	// L4: 对话历史
	history []historyEntry

	// ccr 存储被压缩的原始内容（LRU，最多 20 条）
	ccrStore map[string]string
	ccrKeys  []string // LRU 顺序
}

// historyEntry 是一条对话历史记录，附带元数据用于重要性评分。
type historyEntry struct {
	msg     provider.Message
	score   int    // 重要性评分 0-10
	summary string // 压缩摘要（"" = 原文保留）
}

func newXAIContext(systemMsgs []provider.Message) *xaiContext {
	return &xaiContext{
		systemMessages: systemMsgs,
		ccrStore:       make(map[string]string),
	}
}

// SetGoal 设置当前任务目标（L2）。
func (c *xaiContext) SetGoal(goal string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.goal = goal
}

// SetProjectSkeleton 设置项目骨架（L1）。
func (c *xaiContext) SetProjectSkeleton(skeleton string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projectSkeleton = skeleton
}

// AddDiscovery 添加工具输出的压缩摘要（L3）。
// 原始内容存入 CCR store，压缩摘要放入 L3。
func (c *xaiContext) AddDiscovery(toolName, rawOutput string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	summary := compressToolOutput(toolName, rawOutput)
	if summary == "" {
		return
	}

	// 如果原始输出被压缩了，存入 CCR
	if summary != rawOutput && len(rawOutput) > len(summary)*2 {
		c.storeCCR(rawOutput)
	}

	if len(c.discoveries) >= 10 {
		c.discoveries = c.discoveries[1:]
	}
	c.discoveries = append(c.discoveries, summary)
}

// AddHistory 追加对话历史（L4），自动计算重要性评分。
func (c *xaiContext) AddHistory(msg provider.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := historyEntry{msg: msg}
	entry.score = scoreMessage(msg, c.goal)
	c.history = append(c.history, entry)
}

// AddUserNote 追加用户反馈（L2）。
func (c *xaiContext) AddUserNote(note string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userNotes = append(c.userNotes, note)
	if len(c.userNotes) > 5 {
		c.userNotes = c.userNotes[len(c.userNotes)-5:]
	}
}

// BuildMessages 组装完整的消息数组。
// L0→L1→L2→L3→L4→当前任务，按层拼接。
// L4 中被压缩的消息使用 summary 替代原文。
func (c *xaiContext) BuildMessages(taskPrompt string) []provider.Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	var msgs []provider.Message

	// L0: 系统提示词
	msgs = append(msgs, c.systemMessages...)

	// L1: 项目骨架
	if c.projectSkeleton != "" {
		msgs = append(msgs, provider.Message{
			Role:    provider.RoleSystem,
			Content: c.projectSkeleton,
		})
	}

	// L2: 任务上下文
	taskCtx := c.buildTaskContext()
	if taskCtx != "" {
		msgs = append(msgs, provider.Message{
			Role:    provider.RoleSystem,
			Content: taskCtx,
		})
	}

	// L3: 探索发现
	discovery := c.buildDiscoveryBlock()
	if discovery != "" {
		msgs = append(msgs, provider.Message{
			Role:    provider.RoleSystem,
			Content: discovery,
		})
	}

	// L4: 对话历史（压缩后的）
	for _, e := range c.history {
		if e.summary != "" {
			msgs = append(msgs, provider.Message{
				Role:    e.msg.Role,
				Content: "[compressed] " + e.summary,
			})
		} else {
			msgs = append(msgs, e.msg)
		}
	}

	// 当前任务（首轮传完整任务，后续可以为空让模型从工具结果自然继续）
	if taskPrompt != "" {
		msgs = append(msgs, provider.Message{
			Role:    provider.RoleUser,
			Content: taskPrompt,
		})
	}

	return msgs
}

// Compact 执行 live-zone 压缩。
// L0-L2 前缀不动。L3 保留最近 5 条发现，旧的合并为一条摘要。
// L4 评分低于阈值的消息替换为压缩摘要，高于阈值的保留原文。
func (c *xaiContext) Compact() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// L3: 保留最近 5 条，剩下的合并
	if len(c.discoveries) > 5 {
		old := c.discoveries[:len(c.discoveries)-5]
		c.discoveries = c.discoveries[len(c.discoveries)-5:]
		merged := "## 早期探索摘要\n" + strings.Join(old, "\n")
		// 压缩合并的摘要
		c.discoveries = append([]string{compressText(merged, 300)}, c.discoveries...)
	}

	// L4: 按重要性评分压缩
	const keepScore = 6 // 评分 >= 6 的保留原文
	maxKeep := 6        // 最多保留 6 条原文消息

	kept := 0
	for i := range c.history {
		e := &c.history[i]
		if e.summary != "" {
			continue // 已压缩过的跳过
		}
		if e.score >= keepScore && kept < maxKeep {
			kept++
			continue // 保留原文
		}
		// 压缩为摘要
		e.summary = summarizeMessage(e.msg)
	}
}

// TokenEstimate 估算当前上下文 token 数。
func (c *xaiContext) TokenEstimate() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	total := 0
	for _, m := range c.systemMessages {
		total += len(m.Content)
	}
	total += len(c.projectSkeleton)
	total += len(c.goal)
	for _, s := range c.constraints {
		total += len(s)
	}
	for _, s := range c.discoveries {
		total += len(s)
	}
	for _, e := range c.history {
		if e.summary != "" {
			total += len(e.summary)
		} else {
			total += len(e.msg.Content)
		}
	}
	return total / 4
}

// CCRKeys 返回可检索的原始内容哈希列表（供未来 CCR retrieve 工具使用）。
func (c *xaiContext) CCRKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, len(c.ccrKeys))
	copy(keys, c.ccrKeys)
	return keys
}

// RetrieveCCR 按 key 检索原始内容。
func (c *xaiContext) RetrieveCCR(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.ccrStore[key]
	return v, ok
}

// ── 内部方法 ──

func (c *xaiContext) buildTaskContext() string {
	var parts []string
	if c.goal != "" {
		parts = append(parts, "Goal: "+c.goal)
	}
	if len(c.constraints) > 0 {
		parts = append(parts, "Constraints:\n- "+strings.Join(c.constraints, "\n- "))
	}
	if len(c.userNotes) > 0 {
		parts = append(parts, "User feedback:\n- "+strings.Join(c.userNotes, "\n- "))
	}
	return strings.Join(parts, "\n\n")
}

func (c *xaiContext) buildDiscoveryBlock() string {
	if len(c.discoveries) == 0 {
		return ""
	}
	return "## 代码库探索发现\n\n" + strings.Join(c.discoveries, "\n\n")
}

func (c *xaiContext) storeCCR(content string) {
	key := hashContent(content)
	if _, ok := c.ccrStore[key]; ok {
		return
	}
	c.ccrStore[key] = content
	c.ccrKeys = append(c.ccrKeys, key)
	if len(c.ccrKeys) > 20 {
		old := c.ccrKeys[0]
		delete(c.ccrStore, old)
		c.ccrKeys = c.ccrKeys[1:]
	}
}

// ── 内容感知压缩器 ──

// compressToolOutput 根据工具类型选择压缩策略。
func compressToolOutput(toolName, raw string) string {
	if raw == "" {
		return ""
	}
	if len(raw) <= 300 {
		return raw // 小内容不压缩
	}

	switch toolName {
	case "grep":
		return compressGrepOutput(raw)
	case "read_file":
		return compressFileContent(raw)
	case "glob":
		return compressGlobOutput(raw)
	case "bash":
		return compressLogOutput(raw)
	case "lsp_definition", "lsp_references", "lsp_hover":
		return compressLSPOutput(raw)
	case "code_index":
		return compressCodeIndexOutput(raw)
	default:
		return compressText(raw, 500)
	}
}

// compressGrepOutput 压缩 grep 结果：每个文件保留最相关的一行。
func compressGrepOutput(raw string) string {
	lines := strings.Split(raw, "\n")
	if len(lines) <= 10 {
		return raw
	}

	seen := make(map[string]bool)
	var kept []string
	for _, line := range lines {
		// 提取 file:line 前缀
		parts := strings.SplitN(line, ":", 3)
		filePrefix := ""
		if len(parts) >= 2 {
			filePrefix = parts[0] + ":" + parts[1]
		}
		if filePrefix != "" && seen[filePrefix] {
			continue // 同文件同行的重复匹配跳过
		}
		seen[filePrefix] = true
		kept = append(kept, line)
	}
	if len(kept) > 20 {
		kept = kept[:20]
	}
	return strings.Join(kept, "\n") + fmtSuffix(len(lines), len(kept))
}

// compressFileContent 压缩文件内容：保留包声明、导入、类型/函数签名。
func compressFileContent(raw string) string {
	if len(raw) <= 800 {
		return raw
	}
	lines := strings.Split(raw, "\n")
	var kept []string
	skipBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 保留：包声明、导入、类型/接口/函数声明、注释
		if isSignatureLine(trimmed) {
			kept = append(kept, line)
			skipBlock = false
			continue
		}

		// 函数体内部：跳过
		if skipBlock {
			if trimmed == "}" || trimmed == "};" {
				kept = append(kept, "  // ... body omitted")
				kept = append(kept, line)
				skipBlock = false
			}
			continue
		}

		// 函数/方法定义的开始行
		if strings.HasPrefix(trimmed, "func ") || strings.HasPrefix(trimmed, "pub ") {
			kept = append(kept, line)
			if !strings.HasSuffix(trimmed, "{") && !strings.HasSuffix(trimmed, "(") {
				skipBlock = true
			}
			continue
		}

		// 注释保留
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			kept = append(kept, line)
			continue
		}

		// 空行
		if trimmed == "" {
			kept = append(kept, line)
		}
	}

	return strings.Join(kept, "\n") + fmtSuffix(len(lines), len(kept))
}

func isSignatureLine(line string) bool {
	prefixes := []string{
		"package ", "import ", "type ", "interface ", "func ",
		"class ", "def ", "struct ", "enum ", "trait ", "impl ",
		"export ", "const ", "var ", "let ",
		"@",           // decorators/annotations
		"pub ", "fn ", // Rust
		"#include",    // C/C++
	}
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// compressGlobOutput 压缩 glob 结果：去重目录，保留文件列表。
func compressGlobOutput(raw string) string {
	paths := strings.Split(strings.TrimSpace(raw), "\n")
	if len(paths) <= 15 {
		return raw
	}

	// 按目录分组
	dirs := make(map[string]int)
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		dir := p
		if idx := strings.LastIndex(p, "/"); idx > 0 {
			dir = p[:idx]
		}
		dirs[dir]++
	}

	var result []string
	for dir, count := range dirs {
		if count == 1 {
			result = append(result, dir+"/*")
		} else {
			result = append(result, dir+"/ ("+itoa(count)+" files)")
		}
	}
	return strings.Join(result, "\n") + fmtSuffix(len(paths), len(result))
}

// compressLogOutput 压缩构建/测试日志：保留错误行和摘要。
func compressLogOutput(raw string) string {
	lines := strings.Split(raw, "\n")
	if len(lines) <= 20 {
		return raw
	}

	var errors, warnings, summary []string
	errorCount := 0

	for _, line := range lines {
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "error") || strings.Contains(lower, "fail") || strings.Contains(lower, "panic"):
			errors = append(errors, line)
			errorCount++
		case strings.Contains(lower, "warn"):
			warnings = append(warnings, line)
		case strings.Contains(lower, "pass") || strings.Contains(lower, "ok") || strings.Contains(lower, "success"):
			summary = append(summary, line)
		}
	}

	var result []string
	if len(errors) > 0 {
		result = append(result, fmt.Sprintf("## Errors (%d)", errorCount))
		result = append(result, trimLines(errors, 10)...)
	}
	if len(warnings) > 0 && len(warnings) <= 5 {
		result = append(result, "## Warnings")
		result = append(result, warnings...)
	}
	if len(summary) > 0 {
		result = append(result, "## Summary")
		result = append(result, trimLines(summary, 5)...)
	}
	if len(result) == 0 {
		return compressText(raw, 400)
	}
	return strings.Join(result, "\n")
}

// compressLSPOutput 压缩 LSP 结果：保留符号名和位置。
func compressLSPOutput(raw string) string {
	if len(raw) <= 400 {
		return raw
	}
	// LSP 输出通常包含文件路径和符号信息
	lines := strings.Split(raw, "\n")
	var kept []string
	for _, line := range lines {
		if len(kept) >= 10 {
			break
		}
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "{") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n") + fmtSuffix(len(lines), len(kept))
}

// compressCodeIndexOutput 压缩 code_index 结果。
func compressCodeIndexOutput(raw string) string {
	return compressText(raw, 500)
}

// compressText 通用文本压缩：保留头部和尾部。
func compressText(raw string, maxLen int) string {
	if len(raw) <= maxLen {
		return raw
	}
	head := maxLen * 3 / 4
	tail := maxLen / 4
	return raw[:head] + "\n... (" + itoa(len(raw)-maxLen) + " chars omitted) ...\n" + raw[len(raw)-tail:]
}

// ── 消息重要性评分 ──

// scoreMessage 对消息进行重要性评分（0-10）。
func scoreMessage(msg provider.Message, goal string) int {
	score := 5 // 基础分

	content := strings.ToLower(msg.Content)

	// 错误消息权重最高
	if strings.Contains(content, "error") || strings.Contains(content, "fail") ||
		strings.Contains(content, "panic") || strings.Contains(content, "blocked") {
		score += 3
	}

	// 工具结果权重
	if msg.Role == provider.RoleTool {
		score += 1
	}

	// 与目标相关
	if goal != "" {
		goalWords := strings.Fields(strings.ToLower(goal))
		for _, w := range goalWords {
			if len(w) > 3 && strings.Contains(content, w) {
				score += 1
				break
			}
		}
	}

	// 用户消息权重
	if msg.Role == provider.RoleUser {
		score += 1
	}

	if score > 10 {
		score = 10
	}
	return score
}

// summarizeMessage 生成消息压缩摘要。
func summarizeMessage(msg provider.Message) string {
	content := msg.Content
	if len(content) <= 200 {
		return content
	}

	prefix := ""
	switch msg.Role {
	case provider.RoleAssistant:
		prefix = "[assistant] "
	case provider.RoleUser:
		prefix = "[user] "
	case provider.RoleTool:
		prefix = "[tool:" + msg.Name + "] "
	}

	// 提取关键行（错误行、结论行）
	lines := strings.Split(content, "\n")
	var keyLines []string
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "fail") ||
			strings.Contains(lower, "success") || strings.Contains(lower, "✅") ||
			strings.Contains(lower, "❌") || strings.Contains(lower, "conclusion") ||
			strings.Contains(lower, "summary") || strings.Contains(lower, "result") {
			keyLines = append(keyLines, strings.TrimSpace(line))
		}
	}

	if len(keyLines) > 0 {
		return prefix + strings.Join(trimLines(keyLines, 5), "; ")
	}
	return prefix + compressText(content, 150)
}

// ── 工具函数 ──

func hashContent(s string) string {
	h := 0
	for _, c := range s {
		h = h*31 + int(c)
	}
	return itoa(h)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func fmtSuffix(total, kept int) string {
	if total == kept {
		return ""
	}
	return "\n[压缩: " + itoa(total) + " → " + itoa(kept) + "]"
}

func trimLines(lines []string, max int) []string {
	if len(lines) <= max {
		return lines
	}
	return lines[:max]
}
