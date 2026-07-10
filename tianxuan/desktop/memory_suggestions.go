package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"tianxuan/internal/agent"
	"tianxuan/internal/config"
	"tianxuan/internal/memory"
	"tianxuan/internal/provider"
	"tianxuan/internal/skill"
)

const (
	suggestionSessionLimit = 12
	memorySuggestionLimit  = 6
	maxMemoryStatementChars = 420
)

// MemorySuggestion is a user-confirmed candidate for an active saved memory.
// It is generated read-only from recent local history and only persisted through
// AcceptMemorySuggestion.
type MemorySuggestion struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Body        string   `json:"body"`
	Reason      string   `json:"reason"`
	Evidence    []string `json:"evidence"`
}

// SkillSuggestion is a user-confirmed candidate for a reusable skill.
type SkillSuggestion struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Scope       string   `json:"scope"`
	Body        string   `json:"body"`
	Reason      string   `json:"reason"`
	Evidence    []string `json:"evidence"`
}

// MemorySuggestionsView is the desktop Memory page's suggestion payload.
type MemorySuggestionsView struct {
	Memories    []MemorySuggestion `json:"memories"`
	Skills      []SkillSuggestion  `json:"skills"`
	GeneratedAt string             `json:"generatedAt"`
	Available   bool               `json:"available"`
	Source      string             `json:"source"`
}

// suggestionSession is a loaded session snapshot for suggestion scanning.
type suggestionSession struct {
	Path     string
	ID       string
	Preview  string
	LastSeen time.Time
	Messages []provider.Message
}

// workflowCategory defines a detectable workflow pattern that can be suggested
// as a skill.
type workflowCategory struct {
	Name        string
	Description string
	Reason      string
	Keywords    []string
	Steps       []string
}

// MemorySuggestions scans recent local history and returns draft memory/skill
// candidates. It does not modify memory, skills, sessions, or model context.
func (a *App) MemorySuggestions() MemorySuggestionsView {
	view := MemorySuggestionsView{
		Memories:    []MemorySuggestion{},
		Skills:      []SkillSuggestion{},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return view
	}
	set := ctrl.Memory()
	if set == nil {
		return view
	}
	view.Available = true
	view.Source = "local-history"

	sessionDir := config.WorkspaceSessionDir("")
	sessions := loadSuggestionSessions(sessionDir, suggestionSessionLimit)
	view.Memories = suggestMemories(set, sessions)
	view.Skills = suggestSkills("", ctrl.Skills(), sessions)
	return view
}

// MemorySuggestionsForTab scans recent history for a specific tab.
func (a *App) MemorySuggestionsForTab(tabID string) MemorySuggestionsView {
	view := MemorySuggestionsView{
		Memories:    []MemorySuggestion{},
		Skills:      []SkillSuggestion{},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	ctrl := a.ctrlByTabID(tabID)
	if ctrl == nil {
		return view
	}
	set := ctrl.Memory()
	if set == nil {
		return view
	}
	view.Available = true
	view.Source = "local-history"

	sessionDir := config.WorkspaceSessionDir("")
	sessions := loadSuggestionSessions(sessionDir, suggestionSessionLimit)
	view.Memories = suggestMemories(set, sessions)
	view.Skills = suggestSkills("", ctrl.Skills(), sessions)
	return view
}

// AcceptMemorySuggestion persists a previously previewed memory candidate.
func (a *App) AcceptMemorySuggestion(candidate MemorySuggestion) (string, error) {
	ctrl := a.ctrlByTabID("")
	if ctrl == nil {
		return "", nil
	}
	desc := oneLine(candidate.Description)
	body := strings.TrimSpace(candidate.Body)
	if desc == "" || body == "" {
		return "", fmt.Errorf("memory suggestion requires description and body")
	}
	name := suggestionName(candidate.Name, desc, "memory-candidate")
	set := ctrl.Memory()
	if set == nil {
		return "", nil
	}
	return set.Store.Save(memory.Memory{
		Name:        name,
		Title:       oneLine(candidate.Title),
		Description: desc,
		Type:        memory.NormalizeType(candidate.Type),
		Body:        body,
	})
}

// AcceptSkillSuggestion writes a previewed skill candidate.
func (a *App) AcceptSkillSuggestion(candidate SkillSuggestion) (string, error) {
	name := strings.TrimSpace(candidate.Name)
	desc := oneLine(candidate.Description)
	body := strings.TrimSpace(candidate.Body)
	if name == "" || desc == "" || body == "" {
		return "", fmt.Errorf("skill suggestion requires name, description, and body")
	}
	st := skillStoreForWorkspace("")
	scope := skill.ScopeGlobal
	content := renderSkillSuggestionFile(name, desc, body)
	return st.CreateWithContent(name, scope, content)
}

// --- internal helpers ---

func loadSuggestionSessions(dir string, limit int) []suggestionSession {
	if strings.TrimSpace(dir) == "" || limit <= 0 {
		return nil
	}
	infos, err := agent.ListSessions(dir)
	if err != nil || len(infos) == 0 {
		return nil
	}
	if len(infos) > limit {
		infos = infos[:limit]
	}
	var sessions []suggestionSession
	for _, info := range infos {
		loaded, err := agent.LoadSession(info.Path)
		if err != nil {
			continue
		}
		msgs := loaded.Messages
		sessions = append(sessions, suggestionSession{
			Path:     info.Path,
			ID:       strings.TrimSuffix(filepath.Base(info.Path), ".jsonl"),
			Preview:  info.Preview,
			LastSeen: info.ModTime,
			Messages: msgs,
		})
	}
	return sessions
}

func suggestMemories(set *memory.Set, sessions []suggestionSession) []MemorySuggestion {
	if set == nil || len(sessions) == 0 {
		return []MemorySuggestion{}
	}
	existing := existingMemoryText(set)
	seen := map[string]bool{}
	var out []MemorySuggestion
	for _, sess := range sessions {
		for _, msg := range sess.Messages {
			if msg.Role != provider.RoleUser {
				continue
			}
			statement, reason := extractMemoryStatement(msg.Content)
			if statement == "" {
				continue
			}
			key := normalizeSuggestionKey(statement)
			if key == "" || seen[key] || existingCovers(existing, key) {
				continue
			}
			seen[key] = true
			name := suggestionName("", statement, fmt.Sprintf("memory-candidate-%d", len(out)+1))
			title := suggestionTitle(statement, "Memory candidate")
			typ := inferMemoryType(statement)
			out = append(out, MemorySuggestion{
				ID:          "memory-" + name,
				Name:        name,
				Title:       title,
				Description: oneLine(statement),
				Type:        string(typ),
				Body:        memoryCandidateBody(statement, reason, sess),
				Reason:      reason,
				Evidence:    []string{sessionEvidence(sess, statement)},
			})
			if len(out) >= memorySuggestionLimit {
				return out
			}
		}
	}
	return out
}

func suggestSkills(workspaceRoot string, existing []skill.Skill, sessions []suggestionSession) []SkillSuggestion {
	if len(sessions) == 0 {
		return []SkillSuggestion{}
	}
	existingNames := map[string]bool{}
	for _, sk := range existing {
		existingNames[strings.ToLower(sk.Name)] = true
	}
	scope := "global"
	if strings.TrimSpace(workspaceRoot) != "" {
		scope = "project"
	}

	var out []SkillSuggestion
	for _, cat := range workflowCategories() {
		if existingNames[strings.ToLower(cat.Name)] {
			continue
		}
		evidence := workflowEvidence(cat, sessions)
		if len(evidence) < 2 {
			continue
		}
		out = append(out, SkillSuggestion{
			ID:          "skill-" + cat.Name,
			Name:        cat.Name,
			Description: cat.Description,
			Scope:       scope,
			Body:        skillCandidateBody(cat, evidence),
			Reason:      cat.Reason,
			Evidence:    evidence,
		})
	}
	return out
}

func existingMemoryText(set *memory.Set) []string {
	// The *Set returned by ctrl.Memory() is an immutable snapshot — Docs and
	// Store.List() are never mutated in place; refreshMemoryLocked creates a
	// new *Set. So reading without the controller lock is safe.
	var out []string
	for _, d := range set.Docs {
		out = append(out, normalizeSuggestionKey(d.Body))
	}
	for _, f := range set.Store.List() {
		out = append(out, normalizeSuggestionKey(strings.Join([]string{f.Name, f.Title, f.Description, f.Body}, " ")))
	}
	return out
}

func existingCovers(existing []string, key string) bool {
	if key == "" {
		return true
	}
	for _, text := range existing {
		if text != "" && (strings.Contains(text, key) || strings.Contains(key, text)) {
			return true
		}
	}
	return false
}

func extractMemoryStatement(content string) (string, string) {
	text := oneLine(content)
	if len([]rune(text)) < 8 || len([]rune(text)) > maxMemoryStatementChars {
		return "", ""
	}
	lower := strings.ToLower(text)
	type marker struct {
		value  string
		reason string
	}
	markers := []marker{
		{"记住", "explicit remember request"},
		{"以后", "future-facing preference"},
		{"始终", "persistent working rule"},
		{"总是", "persistent working rule"},
		{"每次", "repeated workflow preference"},
		{"默认", "default behavior preference"},
		{"不要", "negative working preference"},
		{"偏好", "user preference"},
		{"规则", "durable rule"},
		{"约定", "project convention"},
		{"remember", "explicit remember request"},
		{"always", "persistent working rule"},
		{"never", "negative working preference"},
		{"prefer", "user preference"},
		{"preference", "user preference"},
		{"by default", "default behavior preference"},
	}
	for _, m := range markers {
		if strings.Contains(lower, m.value) {
			return trimMemoryLead(text, m.value), m.reason
		}
	}
	return "", ""
}

func trimMemoryLead(text, marker string) string {
	idx := strings.Index(strings.ToLower(text), marker)
	if idx < 0 {
		return text
	}
	trimmed := strings.TrimSpace(text[idx:])
	for _, sep := range []string{"：", ":", "-", "—"} {
		trimmed = strings.TrimPrefix(trimmed, marker+sep)
	}
	return strings.TrimSpace(trimmed)
}

func inferMemoryType(statement string) memory.Type {
	lower := strings.ToLower(statement)
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "github.com/") {
		return memory.TypeReference
	}
	if hasAny(lower, "反馈", "回复", "回答", "不要", "always", "never", "始终", "总是") {
		return memory.TypeFeedback
	}
	if hasAny(lower, "项目", "分支", "pr", "pull request", "仓库", "repo", "约定") {
		return memory.TypeProject
	}
	return memory.TypeUser
}

func memoryCandidateBody(statement, reason string, sess suggestionSession) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(statement))
	b.WriteString("\n\n**Why:** ")
	b.WriteString(reason)
	b.WriteString("\n**How to apply:** ")
	b.WriteString(sessionEvidence(sess, statement))
	return b.String()
}

func workflowCategories() []workflowCategory {
	return []workflowCategory{
		{
			Name:        "tianxuan-code-review",
			Description: "Review code changes, run tests, verify build, and prepare commits with Conventional Commits format.",
			Reason:      "recent history repeatedly touched code review, testing, commits, or CI verification",
			Keywords:    []string{"review", "test", "commit", "pr", "pull request", "build", "ci", "评审", "测试", "提交"},
			Steps: []string{
				"Read the diff or changed files to understand the scope of changes.",
				"Run relevant tests and verify the build passes.",
				"Flag any issues: correctness, security, missing tests, or style violations.",
				"Prepare commits using Conventional Commits format with clear behavior-focused messages.",
				"Push to the branch and confirm CI passes.",
			},
		},
		{
			Name:        "tianxuan-refactor",
			Description: "Safely refactor code: understand dependencies, rename symbols, verify no regressions.",
			Reason:      "recent history repeatedly discussed refactoring, renaming, or restructuring code",
			Keywords:    []string{"refactor", "rename", "restructure", "extract", "重组", "重命名", "重构", "提取"},
			Steps: []string{
				"Map all callers and callees of the target symbol before changing anything.",
				"Use the LSP rename tool when available for safe cross-file renames.",
				"Run the full test suite after changes; investigate any failures.",
				"Review the diff one final time for unintended side effects.",
			},
		},
		{
			Name:        "tianxuan-bug-fix",
			Description: "Diagnose and fix a reported bug: reproduce, isolate, fix, verify, and document.",
			Reason:      "recent history repeatedly discussed debugging, fixing bugs, or investigating errors",
			Keywords:    []string{"bug", "fix", "debug", "error", "crash", "broken", "修复", "错误", "崩溃", "调试"},
			Steps: []string{
				"Reproduce the bug and capture the exact error message or unexpected behavior.",
				"Trace the execution path from entry point to failure using logs or codegraph.",
				"Apply the minimal fix and verify the original reproduction case no longer fails.",
				"Add a regression test to prevent the same bug from recurring.",
				"Document the root cause in the commit message.",
			},
		},
	}
}

func workflowEvidence(cat workflowCategory, sessions []suggestionSession) []string {
	seenSession := map[string]bool{}
	var evidence []string
	for _, sess := range sessions {
		for _, msg := range sess.Messages {
			if msg.Role != provider.RoleUser {
				continue
			}
			text := oneLine(msg.Content)
			if text == "" || !hasAny(strings.ToLower(text), cat.Keywords...) {
				continue
			}
			if seenSession[sess.ID] {
				continue
			}
			seenSession[sess.ID] = true
			evidence = append(evidence, sessionEvidence(sess, text))
			break
		}
	}
	if len(evidence) > 4 {
		return evidence[:4]
	}
	return evidence
}

func skillCandidateBody(cat workflowCategory, evidence []string) string {
	var b strings.Builder
	title := strings.ReplaceAll(cat.Name, "-", " ")
	b.WriteString("# " + title + "\n\n")
	b.WriteString("Use this skill when the user asks for this repeated workflow.\n\n")
	b.WriteString("## Evidence\n\n")
	for _, ev := range evidence {
		b.WriteString("- " + ev + "\n")
	}
	b.WriteString("\n## Workflow\n\n")
	for i, step := range cat.Steps {
		fmt.Fprintf(&b, "%d. %s\n", i+1, step)
	}
	b.WriteString("\n## Stop Condition\n\n")
	b.WriteString("Finish only after the requested change is implemented, verified, and delivered.\n")
	return b.String()
}

func skillStoreForWorkspace(workspaceRoot string) *skill.Store {
	cfg, err := config.Load()
	var custom []string
	if err == nil && cfg != nil {
		custom = cfg.SkillCustomPaths()
	}
	return skill.New(skill.Options{
		ProjectRoot: strings.TrimSpace(workspaceRoot),
		CustomPaths: custom,
	})
}

func renderSkillSuggestionFile(name, desc, body string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n" + strings.TrimSpace(body) + "\n"
}

func suggestionName(given, source, fallback string) string {
	if name := asciiSlug(given); name != "" {
		return name
	}
	if name := asciiSlug(source); name != "" {
		return name
	}
	if name := asciiSlug(fallback); name != "" {
		return name
	}
	return "candidate"
}

func asciiSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == '.':
			if b.Len() > 0 && !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		case unicode.IsSpace(r):
			if b.Len() > 0 && !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
		if b.Len() >= 56 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

func suggestionTitle(s, fallback string) string {
	title := truncateRunes(oneLine(s), 64)
	if title == "" {
		return fallback
	}
	return title
}

func sessionEvidence(sess suggestionSession, text string) string {
	label := sess.ID
	if label == "" {
		label = filepath.Base(sess.Path)
	}
	return label + ": " + truncateRunes(oneLine(text), 160)
}

func normalizeSuggestionKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "..."
}

func hasAny(hay string, needles ...string) bool {
	hay = strings.ToLower(hay)
	for _, needle := range needles {
		if strings.Contains(hay, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
