package agent

import (
	"context"
	"encoding/json"
	"strings"
	"tianxuan/internal/agent/offload"
	"tianxuan/internal/archive"
	"tianxuan/internal/diff"
	"tianxuan/internal/event"
	"tianxuan/internal/learning"
	"tianxuan/internal/memory"
	"tianxuan/internal/nilutil"
	"tianxuan/internal/planmode"
	"tianxuan/internal/provider"
	"tianxuan/internal/skill"
	"tianxuan/internal/tool"
)

// SetAutoSkillStore installs the skill store used for automatic skill
// injection (V10.122). nil disables it. Read-only; bodies are fetched per turn.
func (a *AgentRunner) SetAutoSkillStore(st *skill.Store) {
	a.autoSkill = st
}

// SetActiveSchemas installs a tool subset for this session. Pass nil to revert
// to the full registry. Called by the controller after GoalRouter classification.
// Thread-safe: may be called while stream() reads activeSchemas.
func (a *AgentRunner) SetActiveSchemas(schemas []provider.ToolSchema) {
	a.activeSchemasMu.Lock()
	a.activeSchemas = schemas
	a.activeSchemasMu.Unlock()
}

// SetPlanMode flips the read-only plan-mode gate. While true, executeOne refuses
// any non-read-only tool call using planmode.Policy.Decide.
// Ported from DeepSeek-Reasonix.
func (a *AgentRunner) SetPlanMode(v bool) {
	a.planModeGate.Store(v)
}

// setPlannerMode 临时切换 plannerMode（规划轮语义）：跳过 executor 专属
// 逻辑与三闸门，使规划轮输出计划后自然停止。仅在 PlannerHost 的单轮
// turn 内调用（同一 goroutine 串行），不做并发保护。
func (a *AgentRunner) setPlannerMode(v bool) {
	a.plannerMode = v
}

// SetPlanModePolicy installs the plan-mode tool safety policy.
func (a *AgentRunner) SetPlanModePolicy(p planmode.Policy) {
	a.planModePolicy = p
}

// planModeBlocked checks whether a tool call is blocked by the plan-mode gate.
func (a *AgentRunner) planModeBlocked(name string, readOnly, untrusted bool, safety planmode.PlanSafety, args json.RawMessage) (bool, string) {
	decision := a.planModePolicy.Decide(planmode.Call{
		Name:      name,
		ReadOnly:  readOnly,
		Untrusted: untrusted,
		Safety:    safety,
		Args:      args,
	})
	if !decision.Blocked {
		return false, ""
	}
	return true, decision.Message
}

// SetGate installs the per-call permission gate. MUST be called before the
// run loop starts — executeOne reads gate from concurrent goroutines and
// SetGate does not lock. The happens-before guarantee is provided by the
// caller (Controller) wiring the gate before dispatching the first Send().
// nil disables gating.
func (a *AgentRunner) SetGate(g Gate) {
	if nilutil.IsNil(g) {
		g = nil
	}
	a.gate = g
}

// SetAsker installs the asker the `ask` tool uses to question the user.
// Interactive frontends wire one in; headless runs leave it nil.
func (a *AgentRunner) SetAsker(as Asker) { a.asker = as }

// MergeRuntimePrompt 将运行时上下文合并到系统提示词（L1）末尾，
// 取代原 L2 注入方案。合并后消息前缀永不改变，DeepSeek 可自然缓存。
// 必须在首轮 stream() 调用前调用一次。
func (a *AgentRunner) MergeRuntimePrompt(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	a.session.AppendSystemPrompt(content)
}
func (a *AgentRunner) SetGoal(g string) { a.goal = g }

// SetMemoryQueue installs the sink the remember/forget tools use to apply a
// memory change in the current session. The controller wires itself in.
func (a *AgentRunner) SetMemoryQueue(q memory.Queue) { a.memQueue = q }

// SetSessionSaver installs the sink the remember tool uses when session=true.
func (a *AgentRunner) SetSessionSaver(s memory.SessionSaver) { a.sessionSaver = s }

// SetPromoter installs the sink the promote_session_facts tool uses.
func (a *AgentRunner) SetPromoter(p memory.SessionFactPromoter) { a.promoter = p }

// SetArchive installs the session archive store for cross-session Dream/Distill.
// nil disables archiving. V7.0.
func (a *AgentRunner) SetLSPManager(m interface {
	Diagnostics(ctx context.Context, file string) (string, error)
}) {
	a.lspManager = m
}

// Sink returns the current event sink. SetSink replaces it.
func (a *AgentRunner) Sink() event.Sink { return a.sink }

// SetSink replaces the agent's event sink. Callers must ensure no concurrent
// Run() — this is intended for one-time setup or between-turn sink wrapping.
func (a *AgentRunner) SetSink(s event.Sink) { a.sink = s }

func (a *AgentRunner) SetPatternExtractor(e interface {
	Observe(toolName, result string) *learning.Pattern
}) {
	a.patternExtractor = e
}
func (a *AgentRunner) SetArchive(ar *archive.Store, sessionID string) {
	a.archive = ar
	a.sessionID = sessionID
	// Offload store creation is deferred to here because its per-session
	// subdirectory derives from sessionID (see SetOffload).
	if a.offloadDir != "" && a.offloadStore == nil {
		s, err := offload.NewStore(a.offloadDir, sessionID)
		if err != nil {
			a.offloadStore = nil
			return
		}
		a.offloadStore = s
	}
}

// SetOffload enables context offloading for this session. dir is the parent
// directory for offloaded files; a session-specific subdirectory is created
// automatically. Pass an empty dir to disable. thresholdChars is the output
// size above which results are offloaded (0 = default).
//
// Store creation is deferred until sessionID is known (SetArchive), because
// the per-session subdirectory derives from it. If called after SetArchive,
// the store is created immediately.
func (a *AgentRunner) SetOffload(dir string, thresholdChars int) {
	a.offloadDir = dir
	a.offloadThresholdChars = thresholdChars
	if dir == "" {
		a.offloadStore = nil
		return
	}
	if a.sessionID != "" {
		s, err := offload.NewStore(dir, a.sessionID)
		if err != nil {
			a.offloadStore = nil
			return
		}
		a.offloadStore = s
	}
}

// OffloadStore returns the active offload store, or nil when offloading is
// disabled or not yet initialized (sessionID not set).
func (a *AgentRunner) OffloadStore() *offload.Store {
	return a.offloadStore
}

// CloseOffload cleans up the offload store, deleting all offloaded files.
// Safe to call when offloading is disabled (no-op).
func (a *AgentRunner) CloseOffload() {
	if a.offloadStore != nil {
		_ = a.offloadStore.RemoveAll()
		a.offloadStore = nil
	}
}

// SetPreEditHook installs the pre-edit snapshot hook (see onPreEdit). The
// controller wires it to its per-session checkpoint store; nil disables capture.
func (a *AgentRunner) SetPreEditHook(fn func(diff.Change)) { a.onPreEdit = fn }

// PendingDiffs returns the file changes recorded during the current turn.
// Used by the WorkspacePanel to show session-level file modifications.
func (a *AgentRunner) PendingDiffs() []diff.Change {
	a.preMu.Lock()
	defer a.preMu.Unlock()
	out := make([]diff.Change, len(a.pendingDiffs))
	copy(out, a.pendingDiffs)
	return out
}

// Session returns the agent's current conversation, useful for persistence
// hooks that need to read the message log between turns. sessMu serialises this
// pointer read against SetSession, so a frontend (serve's concurrent /history and
// /new handlers) can't race the swap. The run loop touches a.session directly and
// only swaps it via SetSession while idle, so its reads need no lock.
func (a *AgentRunner) Session() *Session {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	return a.session
}

// SetSession replaces the agent's conversation wholesale. Used by
// `tianxuan chat --resume` to load a saved JSONL transcript before the first turn,
// so the model picks up exactly where it left off. Callers serialise it against a
// running turn (it only fires while idle); sessMu guards the pointer swap itself.
func (a *AgentRunner) SetSession(s *Session) {
	a.sessMu.Lock()
	defer a.sessMu.Unlock()
	a.session = s
	// V8.3.2: reset prefix fingerprint baseline for the new session.
	// verifyPrefix compares L1/L2/tools hashes against the saved baseline;
	// a fresh session has different L1 content, so we must let it re-establish
	// the baseline rather than panic on mismatch.
	a.prefixFingerprintSet = false
	a.lastToolFingerprintSet = false
	// V8.4.1: reset session-level cache counters to prevent cross-session
	// accumulation from producing hit rates > 100%. sessCacheHit/sessCacheMiss
	// increment on every API call and must reset when starting a new session.
	a.sessCacheHit.Store(0)
	a.sessCacheMiss.Store(0)
	// cacheBreakCount removed (Phase 3)
}

// ResetCompactState resets the compaction sticky-state counters so a fresh
// planner session does not inherit stuck status from a previous session.
func (a *AgentRunner) ResetCompactState() {
	a.consecutiveCompacts = 0
	a.compactStuck = false
}

// LastUsage returns the most recent per-turn token telemetry the provider
// reported (nil if no turn has run yet). The TUI uses it to show a context
// gauge alongside the prompt; the actual cache decisions still live inside
// maybeCompact.
func (a *AgentRunner) LastUsage() *provider.Usage { return a.lastUsage.Load() }

// SessionCache returns the cumulative cache hit/miss prompt tokens across every
// API call this session �� the basis for the status line's aggregate hit-rate.
func (a *AgentRunner) SessionCache() (hit, miss int) {
	return int(a.sessCacheHit.Load()), int(a.sessCacheMiss.Load())
}

// ContextWindow returns the configured context-window size in tokens. 0
// means compaction is disabled for this agent.
func (a *AgentRunner) CacheBreakCount() int {
	return a.compaction.CompactCount
}

// systemPrompt returns the concatenated system messages (L1 + L2).
func (a *AgentRunner) systemPrompt() string {
	var b strings.Builder
	for _, m := range a.session.Messages {
		if m.Role == provider.RoleSystem {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(m.Content)
		}
	}
	return b.String()
}

func (a *AgentRunner) ContextWindow() int { return a.compaction.Window }

// CompactRatio returns the fraction of the window at which auto-compaction
// fires (e.g. 0.8). The status line uses it to show headroom to the next compact.
func (a *AgentRunner) CompactRatio() float64 { return a.compaction.Ratio }

// Provider returns the LLM provider this runner uses.
func (a *AgentRunner) Provider() provider.Provider { return a.prov }

// Registry returns the tool registry.
func (a *AgentRunner) Registry() *tool.Registry { return a.tools }
