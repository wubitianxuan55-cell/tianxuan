package cache

import (
	"strings"
	"sync"
	"time"

	"tianxuan/internal/strutil"
)

// RuntimeLayer is the L2 cache domain — session-stable context injected as a
// second system message between the L1 prefix and conversation history.
// ProjectState is locked after the first turn and never changes; SessionState
// tracks workspace edits and short-term execution memory, serialised on demand.
//
// V3.0: upgraded from RuntimeContext. Adds ProjectState/SessionState/ExecutionState.
type RuntimeLayer struct {
	mu         sync.Mutex
	project    ProjectState
	session    SessionState
	hints      []string
	promptHint string
	locked     bool
	compactL2  bool // V5.30: 启用 L2 紧凑格式（结构化 KV，非 Markdown）
}

// ProjectState holds workspace properties that never change during a session.
type ProjectState struct {
	// V4.0: project domain for task routing. Empty/"code" = code project (default).
	// "data" = data analysis, "writing" = document writing, "general" = mixed.
	Domain       string
	Language     string
	Module       string
	EntryPoints  []string
	TopDirs      []string
	TotalFiles   int
	Dependencies []string
	// RootPath is the workspace root directory name (not full path — just the
	// basename, e.g. "tianxuan"). Injected into L2 so the agent knows which
	// project it's working in after a workspace switch. V10.18+.
	RootPath string
	// V3.3: reserved for future multimodal content (images, diagrams, file previews).
	// Inserted at the end of L2 so it doesn't disturb the cache-stable prefix.
	Extra []string `json:"extra,omitempty"`
}

// EditEntry records a single file edit with a version counter for conflict
// detection between parent and forked sub-agents. V3.4.
type EditEntry struct {
	Path      string `json:"path"`
	Version   int    `json:"version"`
	Timestamp int64  `json:"timestamp"` // unix nano
}

// SessionState holds per-session dynamic state that can change across turns.
type SessionState struct {
	RecentEdits   []EditEntry    // V3.4: files edited this session, with version tracking
	ActiveModule  string         // currently active module
	CurrentBranch string
	ShellInfo     string
	Execution     ExecutionState // short-term working memory
	// V3.4: when non-nil, sub-agent Fork is restricted to these paths (read-only
	// elsewhere). nil means no restriction (full access).
	ForkReadOnlyWhitelist []string
}

// ExecutionState captures the agent's current working context — goal, phase,
// active files, pending todos. It is preserved across compaction so the model
// doesn't lose its place after context truncation.
type ExecutionState struct {
	Goal         string     // current sub-goal
	Phase        string     // current phase (e.g. "reproducing bug")
	ActiveFiles  []string   // files currently being worked on
	Hypothesis   string     // current hypothesis (debugging)
	LastAction   string     // last action taken
	PendingTodos []TodoSnap // pending todo items snapshot
}

// TodoSnap is a lightweight snapshot of a todo item for execution memory.
type TodoSnap struct {
	Content string
	Status  string // pending | in_progress | completed
}

// NewRuntimeLayer creates an empty, unlocked RuntimeLayer.
func NewRuntimeLayer() *RuntimeLayer {
	return &RuntimeLayer{}
}

// CopyProjectTo copies the project state to another RuntimeLayer.
// Used by ContextManager.Fork() to share L2 project state with sub-agents.
func (rc *RuntimeLayer) CopyProjectTo(dst *RuntimeLayer) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	dst.mu.Lock()
	defer dst.mu.Unlock()
	dst.project = rc.project
}

// CopySessionTo copies the session state (workspace + execution memory) to
// another RuntimeLayer. Used by ForkCollaborative to give sub-agents
// awareness of the parent's current context.
func (rc *RuntimeLayer) CopySessionTo(dst *RuntimeLayer) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	dst.mu.Lock()
	defer dst.mu.Unlock()
	dst.session = rc.session
}

// SetProject stores the workspace profile. Must be called before Lock().
func (rc *RuntimeLayer) SetProject(p ProjectState) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.locked {
		return
	}
	rc.project = p
}

// ApplyRoute records the GoalRouter classification result. Ignored when locked.
func (rc *RuntimeLayer) ApplyRoute(cfg DomainConfig) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.locked {
		return
	}
	if cfg.Kind != "" && cfg.Kind != KindDefault {
		rc.session.Execution.Goal = string(cfg.Kind)
	}
	if len(cfg.ContextFocus) > 0 {
		rc.session.ActiveModule = strings.Join(cfg.ContextFocus, ", ")
	}
}

// RecentEdits returns a snapshot of the files edited this session.
// Safe to call after Lock(). Used by the controller to inject edits
// as a turn-tail prefix — never into the cache-stable L2 system prompt.
func (rc *RuntimeLayer) RecentEdits() []EditEntry {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	out := make([]EditEntry, len(rc.session.RecentEdits))
	copy(out, rc.session.RecentEdits)
	return out
}

// TrackEdit records a file edit for workspace state tracking.
// V3.4: increments the version counter for conflict detection.
// Safe to call after Lock().
func (rc *RuntimeLayer) TrackEdit(path string) {
	if path == "" {
		return
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()

	now := timeNowUnix()
	// Look for existing entry to bump version
	for i, e := range rc.session.RecentEdits {
		if e.Path == path {
			rc.session.RecentEdits[i].Version++
			rc.session.RecentEdits[i].Timestamp = now
			return
		}
	}

	// New entry
	if len(rc.session.RecentEdits) >= 20 {
		rc.session.RecentEdits = rc.session.RecentEdits[1:]
	}
	rc.session.RecentEdits = append(rc.session.RecentEdits, EditEntry{
		Path:      path,
		Version:   1,
		Timestamp: now,
	})
}

// DetectConflict checks whether a sub-agent's edit conflicts with the
// parent's edits. Returns the conflicting path and true if a conflict exists.
// V3.4: called when a forked sub-agent returns to merge its session state.
func (rc *RuntimeLayer) DetectConflict(child *RuntimeLayer) (string, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	child.mu.Lock()
	defer child.mu.Unlock()

	for _, ce := range child.session.RecentEdits {
		for _, pe := range rc.session.RecentEdits {
			if ce.Path == pe.Path && pe.Version > ce.Version {
				return ce.Path, true
			}
		}
	}
	return "", false
}

// MergeChildEdits merges a sub-agent's RecentEdits into the parent.
// Non-conflicting entries are appended; conflicting ones trigger a notice.
// V3.4: returns the list of conflicting paths (empty = clean merge).
func (rc *RuntimeLayer) MergeChildEdits(child *RuntimeLayer) []string {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	child.mu.Lock()
	defer child.mu.Unlock()

	var conflicts []string
	for _, ce := range child.session.RecentEdits {
		conflict := false
		for i, pe := range rc.session.RecentEdits {
			if ce.Path == pe.Path {
				if pe.Version > ce.Version {
					conflicts = append(conflicts, ce.Path)
				} else {
					rc.session.RecentEdits[i] = ce
				}
				conflict = true
				break
			}
		}
		if !conflict {
			rc.session.RecentEdits = append(rc.session.RecentEdits, ce)
		}
	}
	return conflicts
}

func init() {
	timeNowUnix = func() int64 { return time.Now().UnixNano() }
}

var timeNowUnix = func() int64 { return 0 } // replaced at init

// UpdateExecution refreshes the short-term execution state.
// Safe to call after Lock().
func (rc *RuntimeLayer) UpdateExecution(es ExecutionState) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.session.Execution = es
}

// SetPromptHint 设置 PromptHint，在 Lock() 之前注入到 L2 中。
// Lock 后调用被忽略，确保 L2 缓存稳定。
func (rc *RuntimeLayer) SetPromptHint(hint string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.locked {
		return
	}
	rc.promptHint = hint
}

// AppendHint adds a corrective hint from the FailureDetector. Allowed after lock.
func (rc *RuntimeLayer) AppendHint(hint string) {
	if hint == "" {
		return
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.hints = append(rc.hints, hint)
}

// SetCompactL2 切换 L2 紧凑格式。必须在 Lock() 之前调用，
// 紧凑格式使用 @p/@w/@g 前缀的 KV 行替代 Markdown 列表，
// L2 token 减少约 60%（~100 tok → ~40 tok）。
func (rc *RuntimeLayer) SetCompactL2(v bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.locked {
		return
	}
	rc.compactL2 = v
}

// Lock marks the project state as immutable. Hints and session state can still
// change. Idempotent.
func (rc *RuntimeLayer) Lock() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.locked = true
}

// IsLocked reports whether Lock() has been called.
func (rc *RuntimeLayer) IsLocked() bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.locked
}

// SystemPrompt returns the L2 content as a single string for injection as a
// second system message. Returns "" when no content (first turn, no overhead).
//
// V5.30: 当 compactL2 为 true 时，使用紧凑格式替代 Markdown 格式。
// 紧凑格式使用 @p/@w/@g 前缀的行级 KV 结构，L2 token 减少约 60%。
// 格式在 Lock() 时确定，之后不会变化——不破坏前缀缓存稳定性。
func (rc *RuntimeLayer) SystemPrompt() string {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.compactL2 {
		return rc.compactSystemPrompt()
	}
	return rc.verboseSystemPrompt()
}

// verboseSystemPrompt 是旧的 Markdown 格式 L2 系统提示（V5.29- 默认）。
func (rc *RuntimeLayer) verboseSystemPrompt() string {
	var parts []string

	// Project state (locked, always included after first turn)
	if rc.project.Language != "" || rc.project.RootPath != "" {
		var sb strings.Builder
		sb.WriteString("## Project\n")
		if rc.project.RootPath != "" {
			sb.WriteString("- Root: " + rc.project.RootPath + "\n")
		}
		if rc.project.Module != "" {
			sb.WriteString("- Module: " + rc.project.Module + "\n")
		}
		if rc.project.Language != "" {
			sb.WriteString("- Language: " + rc.project.Language + "\n")
		}
		if len(rc.project.EntryPoints) > 0 {
			sb.WriteString("- Entry points: " + strings.Join(rc.project.EntryPoints, ", ") + "\n")
		}
		if len(rc.project.TopDirs) > 0 {
			sb.WriteString("- Top dirs: " + strings.Join(rc.project.TopDirs, ", ") + "\n")
		}
		parts = append(parts, sb.String())
	}

	// Session workspace state (V5.7: RecentEdits removed — it changes every
	// turn and breaks DeepSeek prefix cache. Controller injects it via turn-tail.)
	if rc.session.ActiveModule != "" || rc.session.CurrentBranch != "" {
		var sb strings.Builder
		sb.WriteString("## Workspace\n")
		if rc.session.ActiveModule != "" {
			sb.WriteString("- Active module: " + rc.session.ActiveModule + "\n")
		}
		if rc.session.CurrentBranch != "" {
			sb.WriteString("- Branch: " + rc.session.CurrentBranch + "\n")
		}
		parts = append(parts, sb.String())
	}

	// Execution state (short-term memory). V5.7: ActiveFiles/Phase/Hypothesis
	// removed — they change between turns and break DeepSeek prefix cache.
	// Only Goal is kept (set once via ApplyRoute and locked thereafter).
	es := rc.session.Execution
	if es.Goal != "" {
		parts = append(parts, "## Current Task\n- Goal: "+es.Goal)
	}

	// Guideline hint (V5.30: PromptHint 注入 L2，首轮锁定后不变)
	if rc.promptHint != "" {
		parts = append(parts, "## Guidelines\n"+rc.promptHint)
	}

	// V5.25: Hints 不再出现在 L2 系统消息中（类似 V5.7 移除 RecentEdits）。
	// hints 每轮可能增长（FailureDetector 追加），出现在 L2 中会破坏缓存。
	// Controller 通过 TurnTailHints() 在 turn-tail 注入。
	return strings.Join(parts, "\n\n")
}
// compactSystemPrompt 是紧凑格式 L2 系统提示（V5.30+）。
// 使用 @p/@w/@g 前缀的 KV 行替代 Markdown 列表，token 减少约 60%。
// 完全确定性——相同输入→相同输出，不破坏前缀缓存稳定性。
func (rc *RuntimeLayer) compactSystemPrompt() string {
	var lines []string

	// Project line: @p root=name m=module l=lang ep=entry dir=dirs files=N
	if rc.project.Language != "" || rc.project.RootPath != "" {
		var sb strings.Builder
		sb.WriteString("@p")
		if rc.project.RootPath != "" {
			sb.WriteString(" root=" + rc.project.RootPath)
		}
		if rc.project.Module != "" {
			sb.WriteString(" m=" + rc.project.Module)
		}
		if rc.project.Language != "" {
			sb.WriteString(" l=" + rc.project.Language)
		}
		if len(rc.project.EntryPoints) > 0 {
			sb.WriteString(" ep=" + strings.Join(rc.project.EntryPoints, ","))
		}
		if len(rc.project.TopDirs) > 0 {
			sb.WriteString(" dir=" + strings.Join(rc.project.TopDirs, ","))
		}
		if rc.project.TotalFiles > 0 {
			sb.WriteString(" files=" + strutil.Itoa(rc.project.TotalFiles))
		}
		lines = append(lines, sb.String())
	}

	// Workspace line: @w mod=activeModule branch=currentBranch
	if rc.session.ActiveModule != "" || rc.session.CurrentBranch != "" {
		var sb strings.Builder
		sb.WriteString("@w")
		if rc.session.ActiveModule != "" {
			sb.WriteString(" mod=" + rc.session.ActiveModule)
		}
		if rc.session.CurrentBranch != "" {
			sb.WriteString(" branch=" + rc.session.CurrentBranch)
		}
		lines = append(lines, sb.String())
	}

	// Goal line: @g goalText
	if rc.session.Execution.Goal != "" {
		lines = append(lines, "@g "+rc.session.Execution.Goal)
	}

	// Guideline hint: @h hintText
	if rc.promptHint != "" {
		lines = append(lines, "@h "+rc.promptHint)
	}

	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// TurnTailHints returns accumulated corrective hints for turn-tail injection.
// V5.25: extracted from SystemPrompt() to keep L2 cache-stable.
func (rc *RuntimeLayer) TurnTailHints() []string {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	out := make([]string, len(rc.hints))
	copy(out, rc.hints)
	return out
}
