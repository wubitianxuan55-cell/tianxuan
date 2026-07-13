package agent

import (
	"tianxuan/internal/context"
	"tianxuan/internal/jobs"
	"tianxuan/internal/provider"
)

// Options configures an AgentRunner.
type Options struct {
	MaxSteps    int
	Temperature float64
	Pricing     *provider.Pricing // optional, for per-turn cost display

	// Gate is the per-call permission gate. nil disables gating.
	Gate Gate

	// Hooks fires PreToolUse / PostToolUse shell hooks around tool calls. nil
	// disables hook firing.
	Hooks ToolHooks

	// Jobs is the session's background-job manager (nil disables background tools).
	Jobs *jobs.Manager

	// Context management. ContextWindow <= 0 disables compaction.
	ContextWindow int
	// Compaction groups compaction settings (V3.0).
	Compaction CompactionConfig
	// Dispatcher is the centralized pre-execution check pipeline (V2.4).
	// nil means the agent uses inline checks (backward compatible).
	Dispatcher *ToolDispatcher
	// CtxMgr is the TCCA context kernel (V3.0). When set, the agent uses it
	// for prompt assembly and tool filtering instead of inline logic.
	CtxMgr *context.ContextManager
	// AuditFunc, when non-nil, is called after every tool execution with a
	// summary of the call. V3.2: foundational audit trail.
	AuditFunc func(tool string, taskKind string, readOnly bool, outcome string, errMsg string, outputLen int, durationMs int64)

	// ParamStorm enables parameter-level duplicate tool call detection (V5.13).
	// nil disables; non-nil provides WindowSize/Threshold/ExemptTools.
	ParamStorm *ParamStormOptions
	// BudgetLimit is the per-session cost budget in yuan (V5.15).
	// <=0 means unlimited. When set, the agent tracks cumulative cost
	// and warns at 80% / blocks at 100%.
	BudgetLimit float64

	// ModelProfile overrides compaction thresholds for specific models (V5.17).
	// nil means use defaults from CompactionConfig.
	ModelProfile *ModelProfile

	// TemplatePrefix is the sub-agent template prefix injected before the
	// user message in spawned agents. Same-class sub-agents share the same
	// template bytes — DeepSeek prefix cache hits across sub-agent invocations.
	TemplatePrefix string
	// ActiveSchemas are the filtered tool schemas for sub-agents (V5.30).
	// When set, RunSubAgent uses these as the tools JSON field so the
	// prefix cache includes the same tools the parent sends.
	ActiveSchemas []provider.ToolSchema
	RuntimePrompt string
	// Goal is the session-level stopping condition (V6.0 P7). When non-empty,
	// the stop gate checks whether the model's final answer satisfies the goal.
	Goal string
	// DisableVerify suppresses the orchestrate verify nudge (V10.22).
	// Sub-agents set this to true so the verify gate doesn't inject
	// "[system] All tasks complete" into their fresh session.
	DisableVerify bool
	// PlannerMode skips executor-specific logic in runDirect —
	// turn preferences, todo rebuild, steer, repeat detection,
	// bg cycle detection, and grace round (V10.46).
	PlannerMode bool
	// StrictEvidence enables host-side verification of complete_step
	// claims (bash command must appear in turn ledger, todo step must
	// match latest todo_write). ON by default in dual-model / Plan Mode
	// where execution follows a structured plan; OFF in single-model
	// mode where the model manages its own flow.
	StrictEvidence bool
	// PlanModeAllowedTools names extra custom tools the plan-mode policy may treat
	// as safe. Known blocked tools (write_file, bash, etc.) cannot be overridden.
	PlanModeAllowedTools []string
	// PlanModeReadOnlyCommands names concrete shell command prefixes that plan mode
	// treats as read-only (e.g. "gh issue view", "git log").
	PlanModeReadOnlyCommands []string
}

// StormBreaker tracks repeated failures to detect death spirals (V3.0 Phase 4).
type StormBreaker struct {
	Sig   string // per-turn fixation signature
	Count int    // consecutive identical failures
}

// Agent is a backward-compatible alias for AgentRunner.
type Agent = AgentRunner

// fallbackTokPerChar is ~4 chars per token — the middle-of-the-road estimate
// used before any provider usage data is available to calibrate.
const fallbackTokPerChar = 0.25

// MidTurnSteerPrefix marks user messages that were injected mid-turn as
// guidance (via Steer). The model sees them as instructions; frontends
// display them as a notice, not a regular user bubble.
// (Design adopted from DeepSeek-Reasonix-V1.12)
const MidTurnSteerPrefix = "[Mid-turn steer queued by the user. Do not treat this as a new task; use it only as additional guidance for the current task after completing the current step.]"
