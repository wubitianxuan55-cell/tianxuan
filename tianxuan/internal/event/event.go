// Package event defines the typed event stream the agent emits as it runs a
// turn, and the Sink it emits to. It decouples "what happened" (the model
// produced reasoning, a tool was dispatched, a turn used N tokens) from "how to
// show it" (ANSI scrollback in a terminal, a card in a webview).
//
// The agent depends only on Sink; each frontend implements one. The chat TUI
// renders events to its scrollback; a headless run renders them to plain ANSI
// on stdout; a future GUI/serve transport forwards them to a webview or
// websocket. This replaces the old io.Writer contract, where the agent wrote
// pre-formatted ANSI and the consumer had to re-derive structure by matching
// line prefixes — fragile, and lossy for any frontend richer than a terminal.
package event

import (
	"tianxuan/internal/evidence"
	"tianxuan/internal/nilutil"
	"tianxuan/internal/provider"
)

// Kind tags an Event. Read the field(s) documented for that kind.
type Kind int

const (
	// TurnStarted marks the start of one top-level Run (one user turn). Sinks
	// reset any per-turn rendering state on it. Carries no payload.
	TurnStarted Kind = iota
	// Reasoning is a thinking-mode reasoning delta (Text). Streamed before the
	// visible answer; sinks typically render it muted under a "thinking" header.
	Reasoning
	// Text is an answer-text delta (Text).
	Text
	// Message marks the assistant turn's text as complete: Text holds the full
	// answer and Reasoning the full chain-of-thought (both already streamed via
	// the deltas above). A sink may use it to re-render the streamed raw text as
	// styled markdown; a plain sink can ignore it.
	Message
	// ToolDispatch announces a tool call is about to run (Tool: ID/Name/Args/ReadOnly).
	ToolDispatch
	// ToolResult reports a finished tool call (Tool: Output/Err/Truncated set).
	ToolResult
	// Usage carries per-turn token telemetry (Usage; Pricing optional, for cost).
	Usage
	// Notice is an out-of-band message — a warning, truncation, block, or
	// compaction notice (Level + Text).
	Notice
	// Phase marks a coordinator boundary, e.g. planner→executor handoff (Text =
	// label such as "deepseek · planning").
	Phase
	// ApprovalRequest asks the frontend to approve a pending tool call
	// (Approval: ID/Tool/Subject). The run blocks until the controller's
	// Approve(ID, …) resolves it; a frontend shows a prompt and answers.
	ApprovalRequest
	// AskRequest asks the frontend to put one or more structured multiple-choice
	// questions to the user (Ask: ID + Questions). The run blocks until the
	// controller's AnswerQuestion(ID, …) resolves it. Powers the `ask` tool.
	AskRequest
	// TurnDone marks the end of one top-level Run (Err non-nil on failure;
	// nil also for a user cancellation, which is not an error). Always the
	// last event of a turn.
	TurnDone
	// CompactionStarted marks the start of a context-compaction pass (Compaction
	// payload: Trigger). A frontend shows a "compacting…" placeholder while the
	// summarizer runs; CompactionDone replaces it. Mirrors ToolDispatch/ToolResult.
	CompactionStarted
	// CompactionDone reports a finished compaction pass (Compaction payload:
	// Trigger/Messages/Summary/Archive). An aborted pass emits this with an empty
	// Summary so the placeholder still resolves. Replaces the older plain Notice
	// so a sink can render a distinct, expandable card.
	CompactionDone
	// ToolProgress streams a chunk of a still-running tool's combined output
	// (Tool: ID + Output = the new chunk). Emitted between ToolDispatch and
	// ToolResult for long tools like bash so a frontend can show live progress.
	ToolProgress
	// MCPSurfaceReady fires once per server when its background-loaded surface
	// (prompts or resources) finishes after startup. Lets UIs refresh /mcp
	// status without polling. Text carries "<server>: <surface> ready (<count>
	// items)".
	MCPSurfaceReady
	// Retrying fires before each backoff sleep while the provider re-attempts the
	// connection+header phase after a transient failure (RetryAttempt of RetryMax).
	Retrying
	// Steer fires when a mid-turn steer message is consumed from the queue and
	// injected as a user message.
	Steer
	// MemoryCompilerStatsEvent carries content-free Memory v5 participation metrics.
	MemoryCompilerStatsEvent
	// GuardianAssessment reports the outcome of a guardian sub-agent safety review.
	GuardianAssessment
	// KindCount is a sentinel one past the last real Kind.
	KindCount
)

// Level classifies a Notice so sinks can style or filter it.
type Level int

const (
	LevelInfo Level = iota
	LevelWarn
)

// Profile carries the subagent model/effort resolved for this call.
type Profile struct {
	Model  string
	Effort string
}

// Tool describes a tool call for ToolDispatch / ToolResult events.
type Tool struct {
	ID         string
	Name       string
	Args       string
	Output     string
	Err        string
	Recoverable bool
	ReadOnly   bool
	Truncated  bool
	DurationMs int64
	Partial   bool
	ParentID  string
	FileDiff
	Profile *Profile
}

// FileDiff is a previewed change carried on a writer tool call.
type FileDiff struct {
	Diff    string
	Added   int
	Removed int
}

// Approval identifies a pending tool-call approval.
type Approval struct {
	ID      string
	Tool    string
	Subject string
	Reason  string
}

// AskOption is one choice the user can pick.
type AskOption struct {
	Label       string
	Description string
}

// AskQuestion is one structured question.
type AskQuestion struct {
	ID      string
	Header  string
	Prompt  string
	Options []AskOption
	Multi   bool
}

// Ask carries an AskRequest.
type Ask struct {
	ID        string
	Questions []AskQuestion
}

// Compaction carries a context-compaction pass.
type Compaction struct {
	Trigger  string
	Messages int
	Summary  string
	Archive  string
}

// GuardianResult carries the outcome of a guardian sub-agent safety review.
type GuardianResult struct {
	ID                string
	Tool              string
	Subject           string
	Outcome           string
	RiskLevel         string
	UserAuthorization string
	Rationale         string
	DurationMs        int64
	Usage             *provider.Usage
	Pricing           *provider.Pricing
}

// AskAnswer is the user's reply to one AskQuestion.
type AskAnswer struct {
	QuestionID string
	Selected   []string
}

// CacheDiagnostics describes whether and why the cacheable prefix changed.
type CacheDiagnostics struct {
	PrefixHash          string
	PrefixChanged       bool
	PrefixChangeReasons []string
	SystemHash          string
	ToolsHash           string
	LogRewriteVersion   int
	ToolSchemaTokens    int
	CacheMissTokens     int
	CacheHitTokens      int
}

// Usage source constants for billable call tracking.
const (
	UsageSourceExecutor   = "executor"
	UsageSourcePlanner    = "planner"
	UsageSourceSubagent   = "subagent"
	UsageSourceCompaction = "compaction"
	UsageSourceClassifier = "classifier"
	UsageSourceTitle      = "title"
)

// Event is one increment in a turn's event stream.
type Event struct {
	Kind             Kind
	Text             string
	Reasoning        string
	MemoryCitations  []provider.MemoryCitation
	MemoryCompiler   *MemoryCompilerStats
	Tool             Tool
	Usage            *provider.Usage
	Pricing          *provider.Pricing
	Source           string
	UsageSource      string
	CacheDiagnostics *CacheDiagnostics
	SessionHit       int
	SessionMiss      int
	Turn             int
	Level            Level
	Approval         Approval
	Ask              Ask
	Err              error
	Compaction       Compaction
	Guardian         GuardianResult
	RetryAttempt     int
	RetryMax         int
}

// MemoryCompilerStats is limited to counts and token estimates.
// It must never carry memory text, prompts, tool output, paths, or IDs.
type MemoryCompilerStats struct {
	Injected         bool
	UsefulIR         bool
	CompiledTokens   int
	IROverheadTokens int
	MemoryReferences int
	Constraints      int
	RiskNotes        int
	ExecutionSteps   int
	TotalNodes       int
	HighSignalNodes  int
	ToolResultNodes  int
	DecisionNodes    int
	StrategyCount    int
	LearningCount    int
}

// ReadinessAuditSink is an optional sink capability.
type ReadinessAuditSink interface {
	RecordReadinessAudit(evidence.ReadinessAudit)
}

// RecordReadinessAudit forwards a readiness audit receipt to sinks that opt in.
func RecordReadinessAudit(s Sink, a evidence.ReadinessAudit) {
	if nilutil.IsNil(s) {
		return
	}
	if rs, ok := s.(ReadinessAuditSink); ok {
		rs.RecordReadinessAudit(a)
	}
}

// Sink consumes a turn's events.
type Sink interface {
	Emit(Event)
}

// FuncSink adapts a plain function to a Sink.
type FuncSink func(Event)

// Emit calls the wrapped function.
func (f FuncSink) Emit(e Event) {
	if f != nil {
		f(e)
	}
}

// Discard is a Sink that drops every event.
var Discard Sink = FuncSink(func(Event) {})
