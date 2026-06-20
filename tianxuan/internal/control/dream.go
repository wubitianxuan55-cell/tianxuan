package control

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"tianxuan/internal/agent"
)

// DreamResult holds the output of a dream analysis pass.
type DreamResult struct {
	SessionCount   int      `json:"session_count"`
	TotalMessages  int      `json:"total_messages"`
	RecentTopics   []string `json:"recent_topics"`
	ToolUsage      map[string]int `json:"tool_usage"`
	CommonPatterns []Pattern `json:"common_patterns"`
	KnowledgeCandidates []string `json:"knowledge_candidates"`
}

// Pattern describes a repeated workflow sequence found across sessions.
type Pattern struct {
	Sequence  []string `json:"sequence"` // tool names in order
	Frequency int     `json:"frequency"`
	Example   string  `json:"example"`   // first user request that triggered it
}

// DistillResult holds the distillation analysis output.
type DistillResult struct {
	Patterns      []Pattern `json:"patterns"`
	TopTools      []ToolStat `json:"top_tools"`
	SkillCandidates []string `json:"skill_candidates"`
}

// ToolStat aggregates usage stats for a single tool.
type ToolStat struct {
	Name     string `json:"name"`
	Count    int   `json:"count"`
	Sessions int   `json:"sessions"` // how many distinct sessions used it
}

// sessionMeta is the on-disk format for a session file header.
type sessionMeta struct {
	ID        string    `json:"id"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
}

// dreamText scans recent sessions and returns a structured analysis report.
func (c *Controller) dreamText(dir string) string {
	if dir == "" {
		return "Dream: no session directory configured"
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Sprintf("Dream: cannot read session dir %s: %v", dir, err)
	}

	// Collect session files sorted by modification time (newest first)
	type sessionFile struct {
		path string
		mod  time.Time
	}
	var files []sessionFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, sessionFile{
			path: filepath.Join(dir, e.Name()),
			mod:  info.ModTime(),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	if len(files) > 10 {
		files = files[:10] // analyze up to 10 most recent sessions
	}

	var result DreamResult
	result.ToolUsage = make(map[string]int)
	result.SessionCount = len(files)

	for _, sf := range files {
		data, err := os.ReadFile(sf.path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		var userRequests []string
		toolCalls := make(map[string]int)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var msg struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			result.TotalMessages++
			if msg.Role == "user" && msg.Content != "" {
				short := msg.Content
				if len([]rune(short)) > 120 {
					short = string([]rune(short)[:120]) + "..."
				}
				userRequests = append(userRequests, short)
			}
		}
		// Collect recent topics from first few user requests
		topicLimit := 2
		for _, req := range userRequests {
			if topicLimit <= 0 {
				break
			}
			result.RecentTopics = append(result.RecentTopics, req)
			topicLimit--
		}
		// Aggregate tool usage
		for tool, count := range toolCalls {
			result.ToolUsage[tool] += count
		}
	}

	// Format output
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🧠 Dream: %d sessions scanned, %d messages\n", result.SessionCount, result.TotalMessages))
	if len(result.RecentTopics) > 0 {
		b.WriteString("\nRecent topics:")
		for _, t := range result.RecentTopics {
			b.WriteString("\n  - " + t)
		}
	}
	if len(result.ToolUsage) > 0 {
		b.WriteString("\n\nTool usage:")
		type kv struct{ k string; v int }
		var sorted []kv
		for k, v := range result.ToolUsage {
			sorted = append(sorted, kv{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })
		limit := 8
		if len(sorted) < limit {
			limit = len(sorted)
		}
		for _, p := range sorted[:limit] {
			b.WriteString(fmt.Sprintf("\n  %s × %d", p.k, p.v))
		}
	}
	b.WriteString("\n\nUse /dream extract to write learnings to memory, or /dream <query> to search")
	return b.String()
}

// distillText analyzes session tool patterns and returns distillation results.
func (c *Controller) distillText(dir string) string {
	if dir == "" {
		return "Distill: no session directory configured"
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Sprintf("Distill: cannot read session dir %s: %v", dir, err)
	}

	var result DistillResult
	toolSession := make(map[string]map[string]bool) // tool → set of session IDs
	toolCount := make(map[string]int)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(e.Name(), ".jsonl")
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var msg struct {
				Role      string `json:"role"`
				ToolCalls []struct {
					Name string `json:"name"`
				} `json:"tool_calls"`
			}
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}
			if msg.Role != "assistant" {
				continue
			}
			for _, tc := range msg.ToolCalls {
				toolCount[tc.Name]++
				if toolSession[tc.Name] == nil {
					toolSession[tc.Name] = make(map[string]bool)
				}
				toolSession[tc.Name][sessionID] = true
			}
		}
	}

	// Top tools sorted by use count
	for name, count := range toolCount {
		result.TopTools = append(result.TopTools, ToolStat{
			Name:     name,
			Count:    count,
			Sessions: len(toolSession[name]),
		})
	}
	sort.Slice(result.TopTools, func(i, j int) bool {
		return result.TopTools[i].Count > result.TopTools[j].Count
	})
	if len(result.TopTools) > 10 {
		result.TopTools = result.TopTools[:10]
	}

	// Identify skill candidates: tools used across 3+ sessions
	for _, tt := range result.TopTools {
		if tt.Sessions >= 3 {
			result.SkillCandidates = append(result.SkillCandidates, tt.Name)
		}
	}

	// Format output
	var b strings.Builder
	b.WriteString("🔬 Distill: tool usage across sessions\n\n")
	b.WriteString(fmt.Sprintf("%-25s %5s  %5s\n", "Tool", "Calls", "Sessions"))
	b.WriteString(strings.Repeat("-", 40) + "\n")
	for _, tt := range result.TopTools {
		b.WriteString(fmt.Sprintf("%-25s %5d  %5d\n", tt.Name, tt.Count, tt.Sessions))
	}
	if len(result.SkillCandidates) > 0 {
		b.WriteString("\nSkill candidates (used in 3+ sessions):")
		for _, sc := range result.SkillCandidates {
			b.WriteString("\n  /" + sc)
		}
	}
	return b.String()
}

// detectPatterns extracts repeated tool-call sequences from the current session.
// Returns pattern strings like "read_file → edit_file → bash".
func (c *Controller) detectPatterns() []string {
	if c.executor == nil {
		return nil
	}
	msgs := c.executor.Session().Snapshot()
	var toolSeq []string
	for _, m := range msgs {
		if m.Role == "assistant" {
			for _, tc := range m.ToolCalls {
				toolSeq = append(toolSeq, tc.Name)
			}
		}
	}
	if len(toolSeq) < 2 {
		return nil
	}
	return findRepeatedPatterns(toolSeq, 2)
}

// createSkillTemplates generates .tianxuan/skills/ markdown templates from detected patterns.
func (c *Controller) createSkillTemplates(patterns []string) int {
	skillsDir := filepath.Join(".tianxuan", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return 0
	}
	created := 0
	for i, p := range patterns {
		if i >= 5 {
			break
		}
		name := "auto-pattern-" + itoaForSkill(i+1)
		tools := strings.Split(strings.ReplaceAll(p, " -> ", ","), ",")
		for j := range tools {
			tools[j] = strings.TrimSpace(tools[j])
		}
		content := "# Auto-generated skill from /distill create\n"
		content += "# Pattern: " + p + "\n\n"
		content += "## Steps\n"
		for j, t := range tools {
			content += fmt.Sprintf("%d. Use `%s` tool\n", j+1, t)
		}
		content += "\n## Notes\n- Generated by tianxuan V6.0 distill\n- Review and customize before using\n"

		skillPath := filepath.Join(skillsDir, name+".md")
		if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
			continue
		}
		created++
	}
	return created
}

// Dream extracts knowledge from the current session into project memory.
// Uses deterministic session summary (no LLM call). V6.0 Feature.
func (c *Controller) Dream(ctx context.Context) error {
	if c.executor == nil {
		return nil
	}
	msgs := c.executor.Session().Snapshot()
	if len(msgs) < 2 {
		return nil
	}
	summary := agent.BuildCompactSummary(msgs[1:])
	if summary == "" {
		return nil
	}
	date := time.Now().Format("2006-01-02")
	// Search past memories related to this session and merge insights
	related := ""
	if mem := c.Memory(); mem != nil && mem.Search != nil {
		matches := mem.Search.Search("project architecture code design")
		if len(matches) > 0 {
			var sb strings.Builder
			sb.WriteString("\nRelated past memories:\n")
			limit := 3
			if len(matches) < limit {
				limit = len(matches)
			}
			for i, m := range matches[:limit] {
				fmt.Fprintf(&sb, "  %d. %s\n", i+1, m.Name)
			}
			related = sb.String()
		}
	}
	entry := "/dream (" + date + "):\n" + summary + "\n" + related
	c.QueueMemory(entry)
	c.notice("dream: knowledge extracted (" + fmt.Sprintf("%d", len(summary)/4) + " tok" + related + ")")
	return nil
}

// Distill analyzes session patterns and suggests skills. V6.0 Feature.
func (c *Controller) Distill(ctx context.Context) error {
	if c.executor == nil {
		return nil
	}
	msgs := c.executor.Session().Snapshot()
	toolSeq := []string{}
	for _, m := range msgs {
		if m.Role == "assistant" {
			for _, tc := range m.ToolCalls {
				toolSeq = append(toolSeq, tc.Name)
			}
		}
	}
	if len(toolSeq) < 3 {
		return nil
	}
	patterns := findRepeatedPatterns(toolSeq, 2)
	if len(patterns) == 0 {
		c.notice("distill: no repeated patterns found")
		return nil
	}
	var sb strings.Builder
	sb.WriteString("/distill (" + time.Now().Format("2006-01-02") + "):\n")
	sb.WriteString("Detected repeated tool patterns:\n")
	for _, p := range patterns {
		sb.WriteString("  - " + p + "\n")
	}
	c.QueueMemory(sb.String())
	c.notice("distill: " + fmt.Sprintf("%d", len(patterns)) + " patterns found")
	return nil
}

func findRepeatedPatterns(seq []string, minLen int) []string {
	seen := map[string]int{}
	for i := 0; i <= len(seq)-minLen; i++ {
		for j := i + minLen; j <= len(seq); j++ {
			pat := ""
			for k := i; k < j; k++ {
				if k > i {
					pat += " -> "
				}
				pat += seq[k]
			}
			if len(strings.Fields(pat)) >= minLen {
				seen[pat]++
			}
		}
	}
	var out []string
	for pat, count := range seen {
		if count >= 2 {
			out = append(out, pat+" (repeated "+fmt.Sprintf("%d", count)+"x)")
		}
	}
	return out
}

func itoaForSkill(n int) string {
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
