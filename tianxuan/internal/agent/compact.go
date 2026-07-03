package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"tianxuan/internal/event"
	"tianxuan/internal/provider"
)

// CompactionConfig controls the LLM-based compaction policy.
// Compact 生成不可变的摘要 digest，旧摘要原样保留，只有 digest 之后到最近 tail
// 之间的内容被折叠。这样 [system + firstUser + digest1...N] 作为固定前缀全量 cache hit。
type CompactionConfig struct {
	Window          int     // context window in tokens (0 = disabled)
	Ratio           float64 // trigger ratio (default 0.8)
	ForceRatio      float64 // force compaction at this high-water mark
	SoftRatio       float64 // send a one-time notice when reaching this ratio
	TailTokens      int     // verbatim recent-tail budget in tokens
	RecentKeep      int     // min recent messages to keep verbatim (fallback)
	ArchiveDir      string  // archive directory for saved sessions

	// Internal state
	LastPrompt   int  // prompt tokens from last turn
	CompactCount int  // how many times we've compacted this session
	softNoticed  bool // one-shot soft-ratio notice
	KeepPolicy KeepPolicy // V10.0: messages to retain verbatim during compaction (0=none)
}

const (
	defaultSoftCompactRatio  = 0.5   // ~50% — emit a growing-context notice
	defaultCompactRatio      = 0.8   // ~80% — trigger compaction
	defaultCompactForceRatio = 0.9   // ~90% — force compaction
	defaultCompactTarget     = 0.5   // kept tail never exceeds this fraction
	defaultTailTokens        = 16384 // verbatim recent-tail budget
	minCompactMessages       = 2     // skip compaction below this many compactable messages
	maxPinnedFirstUserTokens = 1500  // cap on pinning the first user turn verbatim
	pinnedFirstUserWindowFrac = 0.15 // and never pin >15% of the window
	minRecentKeep            = 5     // never keep fewer recent messages (used in tailFloor + checkpoint)
)

// summaryTag wraps the compaction summary so the model can distinguish it from
// live user input. Subsequent compactions detect the tag and preserve prior digests.
const (
	summaryTagOpen  = "<compaction-summary>"
	summaryTagClose = "</compaction-summary>"
)

// summaryTimeout bounds one summarizer call.
const summaryTimeout = 90 * time.Second

// summarySystemPrompt steers the summarizer to produce a structured briefing.
const summarySystemPrompt = `You are compacting the earlier part of a coding agent's conversation to save context.
The agent keeps your summary alongside the user's own turns (kept verbatim) and the recent tail; your job is to fold the assistant/tool work into a briefing it can resume from.
Write under these exact headings, omitting a heading only if it has no content:

## Standing facts & constraints
Everything the user stated that still governs the work — names, paths, IDs, versions, tokens, preferences, and hard "never do X" rules — in their own words. Be exhaustive; this is the durable contract, so prefer over- to under-including.

## Goal
The user's request and intent.

## Decisions & rationale
Key choices made so far and why — so they are not re-litigated or reversed.

## Files & code
Files read or modified, with the specific facts that matter: signatures, line locations, data shapes, and exact edits applied. Be concrete; this is what lets the agent act without re-reading everything.

## Commands & outcomes
Commands run (builds, tests, git) and their relevant results — what passed, what failed, and the error text that matters.

## Errors & fixes
Problems hit and how they were resolved (or not), so the same dead ends are not repeated.

## Pending & next step
What is still in progress or unstarted, and the single most concrete next action to take.

Rules: be terse — bullet points and fragments, not prose. Preserve identifiers, paths, and numbers exactly. Do NOT invent anything not present in the messages; if something is unknown, leave it out rather than guessing.`

// maybeCompact checks if the prompt has grown too large and applies compaction.
// Strategy (inspired by Reasonix's three-tier approach):
//  1. Soft notice at ~50% — one-shot warning, no message change.
//  2. Prune stale tool results — free, may push prompt below threshold.
//  3. LLM summary compaction — produces an immutable digest message.
//  4. Mechanical fold fallback — when summarizer is unreachable.
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

	high := int(float64(a.compaction.Window) * a.ratio())
	soft := int(float64(a.compaction.Window) * a.softRatio())

	// ── Tier 1: Soft notice (no message change) ──
	if prompt >= soft && prompt < high && !a.compaction.softNoticed {
		a.compaction.softNoticed = true
		a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
			Text: fmt.Sprintf("context reached %.0f%% of window; keeping cache-first prefix until compact threshold %.0f%%",
				a.softRatio()*100, a.ratio()*100)})
		return
	}
	if prompt < high {
		a.compaction.LastPrompt = prompt
		a.consecutiveCompacts = 0
		a.compactStuck = false
		return
	}
	if a.compactStuck {
		return
	}

	force := prompt >= int(float64(a.compaction.Window)*a.forceRatio())

	// ── Tier 2: Prune stale tool results (free) ──
	if st, err := a.PruneStaleToolResults(); err == nil && st.Results > 0 {
		ratio := a.tokPerChar()
		saved := int(float64(st.SavedChars) * ratio)
		a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
			Text: fmt.Sprintf("pruned %d stale tool results (~%d tokens est.) before compaction", st.Results, saved)})
		if !force && prompt-saved < high {
			return
		}
	}

	// ── Tier 3: LLM summary compaction ──
	instructions := ""
	// V10.7: 注入当前计划进度，compaction 后 agent 可恢复 todo 状态
	if progress := readProgressFile(); progress != "" {
		instructions = "Include this task progress in the summary under ## Pending & next step:\n" + progress
	}
	if err := a.compact(ctx, "auto", instructions, force); err != nil {
		a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo,
			Text: fmt.Sprintf("compaction skipped: %v", err)})
		return
	}

	a.consecutiveCompacts++
	if a.consecutiveCompacts >= 2 {
		a.compactStuck = true
		a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
			Text: fmt.Sprintf("context_window=%d is too small for compaction to help; auto-compaction paused until prompt drops.",
				a.compaction.Window)})
	}
}

// compact summarizes the older middle of the session and replaces it in place.
// The session becomes: system + firstUser + (old digests) + new digest + recent tail.
func (a *AgentRunner) compact(ctx context.Context, trigger, instructions string, force bool) error {
	msgs := a.session.Messages
	head, start, ok := a.planCompaction(msgs, minCompactMessages)
	if !ok {
		head, start, ok = a.planCompaction(msgs, 1)
	}
	if !ok {
		return nil
	}
	region := msgs[head:start]

	// Split: kept (old digests + small user turns) vs fold (into summary).
	kept, fold := a.partitionFold(region)
	if len(fold) == 0 {
		return nil
	}

	// Economic check: skip if savings too small.
	if !force && !foldEconomics(fold) {
		return nil
	}

	a.sink.Emit(event.Event{Kind: event.CompactionStarted,
		Compaction: event.Compaction{Trigger: trigger}})

	// Archive the folded messages before summarization.
	archived := ""
	if a.compaction.ArchiveDir != "" {
		path, err := archiveMessages(a.compaction.ArchiveDir, fold)
		if err != nil {
			return fmt.Errorf("archive: %w", err)
		}
		archived = path
	}

	summary, err := a.summarizeWithRetry(ctx, fold, instructions)
	if err != nil {
		a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
			Text: "compaction summary unavailable (" + err.Error() + "); folded mechanically"})
		summary = mechanicalFoldDigest(len(fold), archived)
	}

	compacted := make([]provider.Message, 0, head+len(kept)+1+len(msgs)-start)
	compacted = append(compacted, msgs[:head]...)
	compacted = append(compacted, kept...)
	compacted = append(compacted, provider.Message{
		Role: provider.RoleUser,
		Content: summaryTagOpen + "\n" +
			"Summary of earlier conversation (older messages were compacted to save context):\n" +
			summary + "\n" +
			summaryTagClose,
	})
	compacted = append(compacted, msgs[start:]...)
	a.session.Replace(compacted)
	a.session.IncrementRewrite()

	a.sink.Emit(event.Event{Kind: event.CompactionDone,
		Compaction: event.Compaction{Trigger: trigger, Messages: len(fold), Summary: summary, Archive: archived}})
	return nil
}

// CompactNow runs one compaction pass immediately.
func (a *AgentRunner) CompactNow(ctx context.Context, instructions string) error {
	return a.compact(ctx, "manual", instructions, true)
}

// SummarizeFrom replaces messages from boundary onward with a single summary.
func (a *AgentRunner) SummarizeFrom(ctx context.Context, boundary int) error {
	msgs := a.session.Messages
	if boundary < 0 || boundary >= len(msgs) {
		return nil
	}
	fold := msgs[boundary:]
	if len(fold) == 0 {
		return nil
	}
	summary, err := a.summarizeWithRetry(ctx, fold, "")
	if err != nil {
		return err
	}
	replacement := make([]provider.Message, boundary+1)
	copy(replacement, msgs[:boundary])
	replacement = append(replacement, provider.Message{
		Role: provider.RoleUser,
		Content: summaryTagOpen + "\n" +
			"Summary of previous conversation:\n" + summary + "\n" +
			summaryTagClose,
	})
	a.session.Replace(replacement)
	a.session.IncrementRewrite()
	return nil
}

// SummarizeUpTo replaces messages up to boundary with a single summary.
func (a *AgentRunner) SummarizeUpTo(ctx context.Context, boundary int) error {
	msgs := a.session.Messages
	if boundary <= 0 || boundary > len(msgs) {
		return nil
	}
	head := 0
	for head < boundary && msgs[head].Role == provider.RoleSystem {
		head++
	}
	fold := msgs[head:boundary]
	if len(fold) == 0 {
		return nil
	}
	summary, err := a.summarizeWithRetry(ctx, fold, "")
	if err != nil {
		return err
	}
	compacted := append([]provider.Message(nil), msgs[:head]...)
	compacted = append(compacted, provider.Message{
		Role: provider.RoleUser,
		Content: summaryTagOpen + "\n" +
			"Summary of earlier conversation:\n" + summary + "\n" +
			summaryTagClose,
	})
	compacted = append(compacted, msgs[boundary:]...)
	a.session.Replace(compacted)
	a.session.IncrementRewrite()
	return nil
}

// ─── Helper: planCompaction locates the region to compact ───

func (a *AgentRunner) planCompaction(msgs []provider.Message, min int) (head, start int, ok bool) {
	head = a.pinnedPrefixLen(msgs)
	budget := defaultTailTokens
	if a.compaction.Window > 0 {
		if maxByWin := int(float64(a.compaction.Window) * defaultCompactTarget); maxByWin < budget {
			budget = maxByWin
		}
		// V10.11: guarantee at least 25% of the window for recent turns,
		// clamped to [2000, 8000] tokens. Borrowed from opencode's
		// preserveRecentBudget. Ensures protected tool outputs and
		// recent context survive compaction.
		const (
			minRecentTokens = 2000
			maxRecentTokens = 8000
			recentRatio     = 0.25
		)
		recentBudget := int(float64(a.compaction.Window) * recentRatio)
		if recentBudget < minRecentTokens {
			recentBudget = minRecentTokens
		} else if recentBudget > maxRecentTokens {
			recentBudget = maxRecentTokens
		}
		if budget < recentBudget {
			budget = recentBudget
		}
		start = tailStart(msgs, head, budget, a.tokPerChar(), a.tailFloor())
	} else {
		// V10.0: no window configured — fall back to message-count tail
		// so manual /compact and unchecked providers still work.
		start = len(msgs) - a.tailFloor()
		for start > head && start < len(msgs) && msgs[start].Role == provider.RoleTool {
			start--
		}
	}
	if start < head {
		start = head
	}
	if start-head < min {
		return head, start, false
	}
	return head, start, true
}

func (a *AgentRunner) tailFloor() int {
	if a.compaction.RecentKeep > minRecentKeep {
		return a.compaction.RecentKeep
	}
	return minRecentKeep
}

// tailStart walks newest→oldest, growing the verbatim tail until the next
// message would push its token estimate past budgetTokens, then aligns the
// boundary off any tool result.
func tailStart(msgs []provider.Message, head, budgetTokens int, tokPerChar float64, minKeep int) int {
	start := len(msgs)
	acc := 0
	for i := len(msgs) - 1; i > head; i-- {
		c := int(float64(msgChars(msgs[i])) * tokPerChar)
		if len(msgs)-i > minKeep && acc+c > budgetTokens {
			break
		}
		acc += c
		start = i
	}
	for start > head && start < len(msgs) && msgs[start].Role == provider.RoleTool {
		start--
	}
	return start
}

// ─── Prefix pinning: what stays verbatim ───

func (a *AgentRunner) pinnedPrefixLen(msgs []provider.Message) int {
	i := 0
	if i < len(msgs) && msgs[i].Role == provider.RoleSystem {
		i++
	}
	if i < len(msgs) && msgs[i].Role == provider.RoleSystem {
		i++ // L1 + L2
	}
	if i < len(msgs) && msgs[i].Role == provider.RoleUser &&
		!isCompactionSummary(msgs[i]) && a.pinnableUserTurn(msgs[i]) {
		i++
	}
	for i < len(msgs) && isCompactionSummary(msgs[i]) {
		i++
	}
	return i
}

func (a *AgentRunner) pinnableUserTurn(m provider.Message) bool {
	budget := maxPinnedFirstUserTokens
	if a.compaction.Window > 0 {
		if f := int(float64(a.compaction.Window) * pinnedFirstUserWindowFrac); f < budget {
			budget = f
		}
	}
	return int(float64(msgChars(m))*a.tokPerChar()) <= budget
}

func isCompactionSummary(m provider.Message) bool {
	return m.Role == provider.RoleUser &&
		strings.HasPrefix(strings.TrimLeft(m.Content, "\n "), summaryTagOpen)
}

// ─── Partition: what to keep vs fold ───
// partitionFold splits the compaction region into kept (prefix-stable) and fold.
// 缓存前缀不变性：只保留结构性条件——现有 digest 和可 pin 的短 user turn。
// 禁止基于消息内容（如重要性评分）做保留决策，否则 kept 区膨胀破坏前缀。

func (a *AgentRunner) partitionFold(region []provider.Message) (kept, fold []provider.Message) {
	policyKeep := keepIndexes(region, a.keepPolicy)
	for i, m := range region {
		if policyKeep[i] || isCompactionSummary(m) || (m.Role == provider.RoleUser && a.pinnableUserTurn(m)) {
			kept = append(kept, m)
		} else {
			fold = append(fold, m)
		}
	}
	return kept, fold
}

// ─── KeepPolicy helpers ───

// protectedTools is the set of tools whose outputs are foundational context that
// should never be pruned or summarized away during compaction. Pattern borrowed
// from opencode (PRUNE_PROTECTED_TOOLS).
var protectedTools = map[string]bool{
	"read_skill":   true, // skill definitions must persist
	"memory_search": true, // memory search results are foundational context
	"remember":     true, // memory facts should survive compaction
}

func isProtectedToolResult(m provider.Message) bool {
	return m.Role == provider.RoleTool && protectedTools[m.Name]
}

// keepIndexes returns a bool slice indicating which messages in region should be
// kept verbatim due to KeepPolicy. Retention only applies since the latest digest;
// older kept messages are allowed to fold on the next pass.
func keepIndexes(region []provider.Message, policy KeepPolicy) []bool {
	keep := make([]bool, len(region))
	policyStart := 0
	for i, m := range region {
		if isCompactionSummary(m) {
			policyStart = i + 1
		}
	}
	for i, m := range region {
		if i >= policyStart && shouldKeepMessage(m, policy) {
			keep[i] = true
		}
	}
	// When a tool result or assistant message is kept, keep its entire
	// tool-call group so the pairing stays intact.
	for i, m := range region {
		if !keep[i] {
			continue
		}
		switch m.Role {
		case provider.RoleTool:
			if j := findToolCaller(region, i, m.ToolCallID); j >= 0 {
				keepToolCallGroup(region, keep, j)
			}
		case provider.RoleAssistant:
			keepToolCallGroup(region, keep, i)
		}
	}
	return keep
}

func keepToolCallGroup(region []provider.Message, keep []bool, assistantIndex int) {
	if assistantIndex < 0 || assistantIndex >= len(region) {
		return
	}
	m := region[assistantIndex]
	if m.Role != provider.RoleAssistant || len(m.ToolCalls) == 0 {
		return
	}
	keep[assistantIndex] = true
	ids := toolCallIDs(m)
	for j := assistantIndex + 1; j < len(region) && region[j].Role == provider.RoleTool; j++ {
		if ids[region[j].ToolCallID] {
			keep[j] = true
		}
	}
}

func shouldKeepMessage(m provider.Message, policy KeepPolicy) bool {
	if policy&KeepErrors != 0 && isErrorMessage(m) {
		return true
	}
	if policy&KeepUserMarked != 0 && isUserMarked(m) {
		return true
	}
	if policy&KeepProtected != 0 && isProtectedToolResult(m) {
		return true
	}
	return false
}

func isErrorMessage(m provider.Message) bool {
	if m.Role != provider.RoleTool {
		return false
	}
	s := strings.TrimSpace(strings.ToLower(m.Content))
	return strings.HasPrefix(s, "error:") || strings.HasPrefix(s, "blocked:")
}

func isUserMarked(m provider.Message) bool {
	if m.Role != provider.RoleUser {
		return false
	}
	s := strings.TrimSpace(strings.ToLower(m.Content))
	return strings.HasPrefix(s, "[[keep]]") ||
		strings.HasPrefix(s, "[keep]") ||
		strings.HasPrefix(s, "<keep>") ||
		strings.HasPrefix(s, "<!-- keep -->")
}

func findToolCaller(region []provider.Message, toolIndex int, id string) int {
	for i := toolIndex - 1; i >= 0; i-- {
		if region[i].Role != provider.RoleAssistant {
			continue
		}
		for _, tc := range region[i].ToolCalls {
			if tc.ID == id {
				return i
			}
		}
	}
	return -1
}

func toolCallIDs(m provider.Message) map[string]bool {
	ids := make(map[string]bool, len(m.ToolCalls))
	for _, tc := range m.ToolCalls {
		ids[tc.ID] = true
	}
	return ids
}

// ─── Summarizer ───

func (a *AgentRunner) summarizeWithRetry(ctx context.Context, fold []provider.Message, instructions string) (string, error) {
	summary, err := a.summarize(ctx, fold, instructions)
	if err == nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return summary, err
	}
	return a.summarize(ctx, fold, instructions)
}

func (a *AgentRunner) summarize(ctx context.Context, fold []provider.Message, instructions string) (string, error) {
	if a.prov == nil {
		return "", fmt.Errorf("no provider available for summarization")
	}
	ctx, cancel := context.WithTimeout(ctx, summaryTimeout)
	defer cancel()

	sysPrompt := summarySystemPrompt
	if instructions != "" {
		sysPrompt = instructions + "\n\n" + sysPrompt
	}
	// 缓存前缀不变性：禁止向 summarizer prompt 注入任何动态内容（含 BuildCompactSummary），
	// 否则摘要输出变化 → 注入的 user 消息变化 → 前缀断裂。

	transcript := renderTranscript(fold)
	req := provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: sysPrompt},
			{Role: provider.RoleUser, Content: transcript},
		},
		Temperature: 0,
	}

	ch, err := a.prov.Stream(ctx, req)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	var usage *provider.Usage
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case chunk, ok := <-ch:
			if !ok {
				if usage != nil && usage.TotalTokens > 0 {
					a.sink.Emit(event.Event{Kind: event.Usage, Usage: usage, Pricing: a.pricing})
				}
				s := strings.TrimSpace(b.String())
				if s == "" {
					return "", fmt.Errorf("summarizer returned empty output")
				}
				return s, nil
			}
			switch chunk.Type {
			case provider.ChunkText:
				b.WriteString(chunk.Text)
			case provider.ChunkUsage:
				usage = chunk.Usage
			case provider.ChunkError:
				return "", chunk.Err
			}
		}
	}
}

func mechanicalFoldDigest(n int, archive string) string {
	where := "."
	if archive != "" {
		where = " (archived to " + archive + ")."
	}
	return fmt.Sprintf("%d earlier message(s) were folded here to free context, but the automatic summary was unavailable%s Ask the user if you need details from before this point.", n, where)
}

// ─── Token estimation ───



func estimateTextTokens(s string) int {
	if s == "" {
		return 0
	}
	bytes := len(s)
	runes := utf8.RuneCountInString(s)
	// ASCII / European text: ~4 bytes per token
	// CJK / multi-byte text: ~2 characters per token (BPE tokenizer)
	byBytes := (bytes + 3) / 4
	byRunes := (runes + 1) / 2
	if byRunes > byBytes {
		return byRunes
	}
	return byBytes
}

func foldEconomics(region []provider.Message) bool {
	const minFoldTokens = 400
	return estimateMessagesTokens(region) >= minFoldTokens
}

func estimateMessagesTokens(msgs []provider.Message) int {
	total := 0
	for _, m := range msgs {
		total += 4
		total += estimateTextTokens(m.Content)
		total += estimateTextTokens(m.ReasoningContent)
		total += estimateTextTokens(m.Name)
		total += estimateTextTokens(m.ToolCallID)
		for _, tc := range m.ToolCalls {
			total += 8
			total += estimateTextTokens(tc.ID)
			total += estimateTextTokens(tc.Name)
			total += estimateTextTokens(tc.Arguments)
		}
	}
	return total
}

// ─── Transcript rendering ───

func renderTranscript(msgs []provider.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case provider.RoleUser:
			fmt.Fprintf(&b, "[user]\n%s\n\n", m.Content)
		case provider.RoleAssistant:
			if m.Content != "" {
				fmt.Fprintf(&b, "[assistant]\n%s\n", m.Content)
			}
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, "[assistant calls %s] %s\n", tc.Name, summarizeToolArgs(tc.Arguments))
			}
			b.WriteString("\n")
		case provider.RoleTool:
			fmt.Fprintf(&b, "[tool %s result]\n%s\n\n", m.Name, m.Content)
		case provider.RoleSystem:
			fmt.Fprintf(&b, "[system]\n%s\n\n", m.Content)
		}
	}
	return b.String()
}

func summarizeToolArgs(args string) string {
	if args == "" {
		return "(no arguments)"
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		return fmt.Sprintf("(%d bytes)", len(args))
	}
	keys := make([]string, 0, len(parsed))
	for k := range parsed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return fmt.Sprintf("{%s} (%d keys)", strings.Join(keys, ", "), len(parsed))
}

func archiveMessages(dir string, msgs []provider.Message) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, time.Now().Format("20060102-150405.000")+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, m := range msgs {
		if err := enc.Encode(m); err != nil {
			return "", err
		}
	}
	return path, nil
}

func (a *AgentRunner) ratio() float64 {
	if a.compaction.Ratio > 0 {
		return a.compaction.Ratio
	}
	return defaultCompactRatio
}

func (a *AgentRunner) softRatio() float64 {
	if a.compaction.SoftRatio > 0 {
		return a.compaction.SoftRatio
	}
	return defaultSoftCompactRatio
}

func (a *AgentRunner) forceRatio() float64 {
	if a.compaction.ForceRatio > 0 {
		return a.compaction.ForceRatio
	}
	return defaultCompactForceRatio
}

// ─── V5.0 legacy: preserved for checkpoint use ───

// ─── Todos extraction for backward compatibility ───

// readProgressFile reads .tianxuan/progress.md from the project root (found by
// walking up from cwd). Returns "" if the file doesn't exist or can't be read.
// Used by maybeCompact to inject current todo state into compaction instructions.
func readProgressFile() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	// Walk up looking for .tianxuan/
	for {
		candidate := filepath.Join(dir, ".tianxuan", "progress.md")
		if data, err := os.ReadFile(candidate); err == nil {
			return string(data)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
