package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"tianxuan/internal/provider"
)

// ─── Task / chat classifier ───────────────────────────────────────────────
// Ported from DeepSeek-Reasonix V1.17.10. Distinguishes actionable tasks
// ("fix the bug") from conversational inputs ("hello", "thanks") to avoid
// wasting execution turns on greetings and acknowledgements.
// Layers: LLM (with 2s timeout) → heuristic fallback → length heuristic.

// TaskClassifier decides whether user input is a task (requires action) or
// chat (conversational).
type TaskClassifier interface {
	IsTask(ctx context.Context, input string) (bool, error)
}

// NewTaskClassifier creates a classifier backed by the given provider. When
// the provider is nil the returned classifier is pure heuristic (no LLM
// dependency).
func NewTaskClassifier(prov provider.Provider) TaskClassifier {
	fallback := newHeuristicClassifier()
	if prov == nil {
		return fallback
	}
	return newLLMClassifier(prov, fallback)
}

// ─── LLM path ──────────────────────────────────────────────────────────────

type llmClassifier struct {
	provider provider.Provider
	cache    *classificationCache
	fallback TaskClassifier
}

func newLLMClassifier(prov provider.Provider, fallback TaskClassifier) *llmClassifier {
	return &llmClassifier{
		provider: prov,
		cache:    newClassificationCache(),
		fallback: fallback,
	}
}

const classificationSystemPrompt = `You are a classifier that determines whether user input is a "task" (requires action/execution) or "chat" (conversational/greeting).

Task: Any input that asks the assistant to write code, fix bugs, run commands, analyze code, create files, or perform any action.
Chat: Greetings, acknowledgments, confirmations, questions about the assistant itself, or purely conversational inputs.

Respond with ONLY one word: "task" or "chat"

Examples:
- "fix the auth bug" → task
- "create a component" → task
- "run tests" → task
- "the login isn't working" → task
- "can you help with this error?" → task
- "继续处理" → task
- "修复这个问题" → task
- "hello" → chat
- "thanks" → chat
- "ok" → chat
- "I see" → chat
- "你好" → chat
- "谢谢" → chat
- "thanks for fixing that" → chat`

func (c *llmClassifier) IsTask(ctx context.Context, input string) (bool, error) {
	if cached, ok := c.cache.Get(input); ok {
		return cached, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req := provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: classificationSystemPrompt},
			{Role: provider.RoleUser, Content: "Classify: " + input},
		},
		MaxTokens:   10,
		Temperature: 0,
	}

	ch, err := c.provider.Stream(ctx, req)
	if err != nil {
		return c.fallback.IsTask(ctx, input)
	}

	var response strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			return c.fallback.IsTask(ctx, input)
		}
		response.WriteString(chunk.Text)
	}

	result := strings.ToLower(strings.TrimSpace(response.String()))
	isTask := strings.Contains(result, "task")
	c.cache.Set(input, isTask)
	return isTask, nil
}

// ─── Heuristic path ────────────────────────────────────────────────────────

type heuristicClassifier struct{}

func newHeuristicClassifier() *heuristicClassifier { return &heuristicClassifier{} }

func (h *heuristicClassifier) IsTask(_ context.Context, input string) (bool, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return false, nil
	}

	normalized := strings.ToLower(strings.Trim(trimmed, " \t\r\n.!?。！？,，;；:："))

	// 1. Short greeting white-list (≤3 words).
	shortGreetings := []string{
		"hello", "hi", "hey", "你好", "您好", "nihao",
		"thanks", "thank you", "谢谢", "谢了",
		"ok", "okay", "好的", "嗯", "行",
		"got it", "i see", "明白", "了解", "收到", "我知道了", "先不用",
	}
	words := strings.Fields(normalized)
	if len(words) <= 3 {
		for _, greeting := range shortGreetings {
			if normalized == greeting {
				return false, nil
			}
		}
	}

	// 2. Polite acknowledgements that mention completed-task vocabulary.
	chatPhrases := []string{
		"thanks for", "thank you for", "i'll check later", "i will check later",
		"i'll test it later", "i will test it later", "that test was helpful", "the test was helpful",
		"谢谢你", "辛苦了",
	}
	for _, phrase := range chatPhrases {
		if strings.Contains(normalized, phrase) {
			return false, nil
		}
	}

	// 3. File references → strong task signal.
	if strings.Contains(trimmed, "@") || strings.Contains(trimmed, ".go") ||
		strings.Contains(trimmed, ".js") || strings.Contains(trimmed, ".py") ||
		strings.Contains(trimmed, ".ts") {
		return true, nil
	}

	// 4. Failure / help descriptions.
	taskPhrases := []string{
		"not working", "isn't working", "doesn't work", "dont work", "don't work",
		"can you help", "help with", "broken", "error", "bug", "issue", "failed", "failing", "crash", "cannot", "can't",
		"问题", "不工作", "无法", "不能", "报错", "错误", "失败", "崩溃", "异常",
		"卡住", "卡住了", "没反应", "不生效", "异常退出",
	}
	for _, phrase := range taskPhrases {
		if strings.Contains(normalized, phrase) {
			return true, nil
		}
	}

	// 5. Action keywords.
	actionNeedles := []string{
		"fix", "debug", "repair", "resolve", "reproduce",
		"create", "add", "write", "edit", "update", "change", "delete", "remove", "rename",
		"review", "inspect", "analyze", "check", "test", "run", "build", "implement", "refactor",
		"continue work", "continue the", "continue this",
		"修复", "调试", "解决", "复现", "创建", "新建", "添加", "编写", "编辑", "修改", "更新",
		"删除", "移除", "重命名", "评审", "检查", "分析", "测试", "运行", "构建", "实现", "重构", "继续处理",
		"看看", "看下", "帮我看", "帮我看下", "处理下", "处理一下", "排查", "定位",
	}
	for _, needle := range actionNeedles {
		if containsTaskNeedle(normalized, needle) {
			return true, nil
		}
	}

	// Default: longer input → more likely a task.
	return len(words) > 5, nil
}

func containsTaskNeedle(input, needle string) bool {
	if needle == "" {
		return false
	}
	if containsNonASCII(needle) || strings.Contains(needle, " ") {
		return strings.Contains(input, needle)
	}
	for _, word := range strings.FieldsFunc(input, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_'
	}) {
		if word == needle {
			return true
		}
	}
	return false
}

func containsNonASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return true
		}
	}
	return false
}

// ─── Classification cache ──────────────────────────────────────────────────

type classificationCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	maxSize int
	ttl     time.Duration
}

type cacheEntry struct {
	isTask    bool
	timestamp time.Time
}

const (
	defaultCacheMaxSize = 100
	defaultCacheTTL     = 5 * time.Minute
)

func newClassificationCache() *classificationCache {
	return &classificationCache{
		entries: make(map[string]cacheEntry),
		maxSize: defaultCacheMaxSize,
		ttl:     defaultCacheTTL,
	}
}

func (c *classificationCache) Get(input string) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := normalizeInputForCache(input)
	entry, ok := c.entries[key]
	if !ok || time.Since(entry.timestamp) > c.ttl {
		return false, false
	}
	return entry.isTask, true
}

func (c *classificationCache) Set(input string, isTask bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := normalizeInputForCache(input)
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}
	c.entries[key] = cacheEntry{isTask: isTask, timestamp: time.Now()}
}

func (c *classificationCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry)
}

func (c *classificationCache) evictOldest() {
	type entry struct {
		key       string
		timestamp time.Time
	}
	entries := make([]entry, 0, len(c.entries))
	for k, v := range c.entries {
		entries = append(entries, entry{key: k, timestamp: v.timestamp})
	}
	if len(entries) == 0 {
		return
	}
	// Bubble-sort by timestamp (ascending) — small N.
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].timestamp.After(entries[j].timestamp) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	toDelete := len(entries) / 5
	if toDelete == 0 {
		toDelete = 1
	}
	for i := 0; i < toDelete && i < len(entries); i++ {
		delete(c.entries, entries[i].key)
	}
}

func normalizeInputForCache(input string) string {
	normalized := strings.ToLower(strings.TrimSpace(input))
	normalized = strings.Join(strings.Fields(normalized), " ")
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}
