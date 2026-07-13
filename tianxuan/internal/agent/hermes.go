package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"tianxuan/internal/codegraph"
	"tianxuan/internal/event"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)


// Hermes runs two models in separate sessions to keep each one's prompt
// prefix cache-stable: a low-frequency planner proposes an approach, then the
// executor (a full tool-using AgentRunner) carries it out. The sessions never
// mix, so neither model's prefix is disturbed by the other's turns.
//
// V10.32: when readonlyTools is set, the planner uses AgentRunner with
// read-only tools (read_file/grep/glob/web_search/...) so it can investigate
// the codebase before proposing a plan. planMaxSteps caps planner turns.
type Hermes struct {
	hermesProvider provider.Provider
	hermesSess     *Session
	hermesSystem   string
	hermesPricing  *provider.Pricing
	hephaestus     *AgentRunner
	temperature    float64
	sink           event.Sink

	readonlyTools *tool.Registry // V10.32: if set, planner runs as AgentRunner
	planMaxSteps  int            // max planner tool-call turns (<=0 = unlimited)
	asker         Asker          // V10.34: interactive plan confirmation (nil = auto-confirm)

	// V10.36: persistent planner Agent with compaction — replaces per-turn temp AgentRunner.
	// The planner accumulates planning history + execution results across turns, with
	// compaction keeping the context bounded. This gives the planner a proper TCCA-like
	// architecture (L1 stable prefix + L4 growing flow + compaction).
	plannerAgent *AgentRunner

	// V10.54: workspace root for project map injection.
	wsRoot          string
	lastProjectHash string // hash of last injected ProjectMap; "" means not injected yet or stale
}

// NewHermes creates a Hermes orchestrator. hermesProvider is the planning model,
// hephaestus is the execution AgentRunner. sink receives events from both.
//
// V10.32: pass readonlyTools (nil for stream-only) and planMaxSteps to let
// Hermes use read-only tools for code investigation before proposing a plan.
// V10.36: contextWindow + archiveDir enable compaction on the planner's persistent session.
func NewHermes(hermesProvider provider.Provider, hermesSession *Session, hermesPricing *provider.Pricing, hephaestus *AgentRunner, temperature float64, sink event.Sink, readonlyTools *tool.Registry, planMaxSteps int, contextWindow int, archiveDir string, wsRoot string) *Hermes {
	if hermesSession == nil {
		hermesSession = NewSession("")
	}
	hermesSystem := sessionSystemPrompt(hermesSession)
	h := &Hermes{
		hermesProvider: hermesProvider,
		hermesSess:     hermesSession,
		hermesSystem:   hermesSystem,
		hermesPricing:  hermesPricing,
		hephaestus:     hephaestus,
		temperature:    temperature,
		sink:           sink,
		readonlyTools:  readonlyTools,
		planMaxSteps:   planMaxSteps,
		wsRoot:         wsRoot,
	}
	// V10.36: create persistent planner Agent with compaction so the planner
	// accumulates history across turns without unbounded growth.
	// V10.58: StrictEvidence is explicitly false — the planner uses read-only
	// tools and never calls complete_step, so strict verification is meaningless.
	if readonlyTools != nil {
		plannerSink := event.FuncSink(func(e event.Event) {
			// Suppress TurnStarted from the planner agent — Hermes
			// already started the turn (line 150). A redundant
			// TurnStarted would reset perTurnPlannerUsage in the
			// frontend, zeroing out the planner's cost stats.
			if e.Kind == event.TurnStarted {
				return
			}
			if e.Kind == event.Usage {
				if e.UsageSource == "" || e.UsageSource == event.UsageSourceExecutor {
					e.UsageSource = event.UsageSourcePlanner
				}
			}
			sink.Emit(e)
		})
		h.plannerAgent = New(hermesProvider, readonlyTools, hermesSession, Options{
			MaxSteps:       planMaxSteps,
			Temperature:    temperature,
			Pricing:        hermesPricing,
			Gate:           &autoGate{},
			DisableVerify:  true,
			PlannerMode:    true,
			StrictEvidence: false,
			ContextWindow:  contextWindow,
			Compaction:     CompactionConfig{ArchiveDir: archiveDir, Window: contextWindow},
		}, plannerSink)
	}
	return h
}

func sessionSystemPrompt(s *Session) string {
	if s == nil {
		return ""
	}
	for _, m := range s.Snapshot() {
		if m.Role == provider.RoleSystem {
			return m.Content
		}
	}
	return ""
}

// ResetSession discards turn-local planner history when switching
// executor sessions. Carrying the old Hermes transcript across sessions
// can make the next plan reuse unrelated tasks.
func (h *Hermes) ResetSession() {
	if h == nil {
		return
	}
	system := h.hermesSystem
	if system == "" {
		system = sessionSystemPrompt(h.hermesSess)
	}
	h.hermesSess = NewSession(system)
	// Also update the plannerAgent's session pointer so it uses the fresh
	// session. Without this, plannerAgent still writes to the old session,
	// leaking stale history into future plans across session switches.
	if h.plannerAgent != nil {
		h.plannerAgent.SetSession(h.hermesSess)
	}
}

// PlannerContext returns the planner agent's last usage and context window,
// for the status bar's per-model context gauge.
func (h *Hermes) PlannerContext() (used int, window int) {
	if h == nil || h.plannerAgent == nil {
		return 0, 0
	}
	u := h.plannerAgent.LastUsage()
	if u == nil {
		return 0, h.plannerAgent.ContextWindow()
	}
	return u.PromptTokens, h.plannerAgent.ContextWindow()
}

// SetAsker installs the interactive asker for plan confirmation (V10.34).
// nil means headless mode — plans auto-confirm without user approval.
// Also wires the asker into the plannerAgent so it can ask clarifying questions
// during planning (scope negotiation, detail gathering).
func (h *Hermes) SetAsker(a Asker) {
	h.asker = a
	if h.plannerAgent != nil {
		h.plannerAgent.SetAsker(a)
	}
}

// ── Run (top-level orchestration) ───────────────────────────────────────

// Run plans with the planner model, then hands the plan to the executor.
// Returns a merged TurnResult combining the planner's and executor's outcomes.
func (h *Hermes) Run(ctx context.Context, input string) (*TurnResult, error) {
	h.sink.Emit(event.Event{Kind: event.TurnStarted})

	// V10.31: fast path — skip planner for simple/quick tasks ("!" prefix).
	if result, err := h.runFastPath(ctx, input); result != nil || err != nil {
		return result, err
	}

	// Normal path: plan → confirm → execute.
	h.sink.Emit(event.Event{Kind: event.Phase, Text: h.hermesProvider.Name() + " · hermes"})
	prePlanLen := len(h.hermesSess.Messages)
	h.injectProjectMap()

	plan, err := h.planWithConfirmation(ctx, input, prePlanLen)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		// Hermes answered directly or user chose chat-only — the planner's
		// text has already been streamed; no executor dispatch needed.
		// Summary is empty because callers discard TurnResult for this path.
		return &TurnResult{Plan: input, Success: true}, nil
	}

	return h.executePlan(ctx, input, *plan)
}

// ── Run sub-steps ───────────────────────────────────────────────────────

// runFastPath handles the "!" prefix fast path that skips the planner entirely.
// Returns (nil, nil) when input is not a fast-path candidate.
func (h *Hermes) runFastPath(ctx context.Context, input string) (*TurnResult, error) {
	task, ok := shouldSkipPlanner(input)
	if !ok {
		return nil, nil
	}
	h.sink.Emit(event.Event{Kind: event.Phase, Text: h.hephaestus.ProvName() + " · 快速执行"})
	// Suppress the executor's TurnStarted — Hermes already started the turn.
	execSink := h.hephaestus.Sink()
	h.hephaestus.SetSink(event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnStarted {
			return
		}
		execSink.Emit(e)
	}))
	defer h.hephaestus.SetSink(execSink)
	return h.hephaestus.Run(ctx, formatHandoff(task, task, ""))
}

// injectProjectMap adds a structural overview to the planner session when the
// workspace has changed since the last injection.
func (h *Hermes) injectProjectMap() {
	if h.wsRoot == "" {
		return
	}
	pm := codegraph.Analyze(h.wsRoot)
	pmHash := pm.Hash()
	if pmHash != h.lastProjectHash {
		h.hermesSess.Add(provider.Message{
			Role:    provider.RoleUser,
			Content: "## 项目代码图谱\n\n" + pm.Format() + "\n\n以上是当前项目的代码结构概览，规划时可直接引用其中的路径和类型名。",
		})
		h.lastProjectHash = pmHash
	}
}

// planWithNote bundles a confirmed plan with any user note from confirmation.
type planWithNote struct {
	text     string
	userNote string
}

// planWithConfirmation runs the planner in a replan loop: plan → confirm →
// repeat on revise. Returns the confirmed plan, or nil when Hermes answered
// directly (no code changes needed) or the user chose chat-only.
func (h *Hermes) planWithConfirmation(ctx context.Context, input string, prePlanLen int) (*planWithNote, error) {
	for {
		plan, err := h.plan(ctx, input)
		if err != nil {
			h.hermesSess.Truncate(prePlanLen)
			return nil, fmt.Errorf("hermes: %w", err)
		}
		if isAnswerNotAction(plan) {
			return nil, nil // Hermes answered directly
		}

		// Keep the full planner output — preamble (analysis/reasoning) is valuable
		// context for the executor. Previously only the <!--plan--> portion was kept.

		userNote, chatOnly, revise, err := h.confirmPlan(ctx, input, plan)
		if err != nil {
			h.hermesSess.Truncate(prePlanLen)
			return nil, err
		}
		if chatOnly {
			return nil, nil
		}
		if revise {
			if userNote != "" {
				// V10.58: append to current input (may already contain prior
				// feedback) instead of resetting to origInput — or the previous
				// round's feedback would be silently lost.
				input = input + "\n\n—— User feedback on previous plan ——\n" + userNote
			}
			// V10.58: keep the original prePlanLen as the rollback baseline;
			// advancing it would leave abandoned plan messages in the session
			// if the re-plan also fails.
			continue
		}
		return &planWithNote{text: plan, userNote: userNote}, nil
	}
}

// executePlan dispatches the executor with the confirmed plan, feeds results
// back to the planner session, and emits TurnResultEvent for the frontend.
func (h *Hermes) executePlan(ctx context.Context, origInput string, p planWithNote) (*TurnResult, error) {
	plan := p.text
	userNote := p.userNote

	h.sink.Emit(event.Event{Kind: event.Phase, Text: h.hephaestus.ProvName() + " · Hephaestus"})
	// Suppress the executor's TurnStarted — Hermes already started the turn.
	execSink := h.hephaestus.Sink()
	h.hephaestus.SetSink(event.FuncSink(func(e event.Event) {
		if e.Kind == event.TurnStarted {
			return
		}
		execSink.Emit(e)
	}))
	defer h.hephaestus.SetSink(execSink)

	// V10.49: pre-inject the original Chinese input before the handoff prompt.
	h.hephaestus.Session().Add(provider.Message{Role: provider.RoleUser, Content: origInput})
	execResult, execErr := h.hephaestus.Run(ctx, formatHandoff(origInput, plan, userNote))

	// Attach plan to TurnResult so the returned value is self-contained.
	if execResult != nil {
		execResult.Plan = plan
	}

	// Feed results back to the planner and emit TurnResultEvent.
	if execResult != nil {
		h.feedResultToPlanner(execResult)
		h.sink.Emit(event.Event{
			Kind: event.TurnResultEvent,
			PlanResult: &event.PlanResult{
				Plan:          execResult.Plan,
				FilesCreated:  execResult.FilesCreated,
				FilesModified: execResult.FilesModified,
				Success:       execResult.Success,
				Errors:        execResult.Errors,
				Summary:       execResult.Summary,
			},
		})
	} else if execErr != nil {
		synth := &TurnResult{
			Plan:    plan,
			Success: false,
			Errors:  []string{execErr.Error()},
			Summary: "execution terminated before producing output",
		}
		h.hermesSess.Add(provider.Message{
			Role:    provider.RoleUser,
			Content: formatExecutionFeedback(synth),
		})
		h.sink.Emit(event.Event{
			Kind: event.TurnResultEvent,
			PlanResult: &event.PlanResult{
				Plan:    synth.Plan,
				Success: false,
				Errors:  synth.Errors,
				Summary: synth.Summary,
			},
		})
	}
	return execResult, execErr
}

// feedResultToPlanner injects execution feedback into the planner's session
// and invalidates the project map cache on structural changes.
func (h *Hermes) feedResultToPlanner(r *TurnResult) {
	hasContent := r.Summary != "" || len(r.Errors) > 0 ||
		len(r.FilesCreated) > 0 || len(r.FilesModified) > 0
	if hasContent {
		h.hermesSess.Add(provider.Message{
			Role:    provider.RoleUser,
			Content: formatExecutionFeedback(r),
		})
	}
	if h.wsRoot != "" && hasStructuralChange(r.FilesCreated, r.FilesModified) {
		h.lastProjectHash = ""
	}
}

// ── Formatting helpers ──────────────────────────────────────────────────

// formatExecutionFeedback converts a TurnResult into a structured summary
// for injection into the planner's session so the planner knows what happened.
func formatExecutionFeedback(r *TurnResult) string {
	status := "success"
	if !r.Success {
		status = "errors"
	}

	created := quoteFilePaths(r.FilesCreated)
	modified := quoteFilePaths(r.FilesModified)

	errors := "(none)"
	if len(r.Errors) > 0 {
		errors = strings.Join(r.Errors, "; ")
	}

	summary := r.Summary
	if summary == "" {
		summary = "(execution produced no summary — check Errors for details)"
	}

	conclusion := ""
	if r.Success && len(r.Errors) == 0 {
		conclusion = "\n- ✅ 任务已完成（Success=true, Errors 为空）"
	} else {
		conclusion = "\n- ⚠️ 任务未完成，请检查 Errors 并修正"
	}

	return fmt.Sprintf("[上一轮执行结果] %s\n- Created: %s\n- Modified: %s\n- Errors: %s\n- Summary: %s%s\n", status, created, modified, errors, summary, conclusion)
}

// hasStructuralChange checks whether any file path indicates a structural change
// that would invalidate the cached ProjectMap (go.mod, package.json, or internal/ paths).
func hasStructuralChange(created, modified []string) bool {
	check := func(paths []string) bool {
		for _, p := range paths {
			if p == "go.mod" || p == "package.json" || p == "Cargo.toml" {
				return true
			}
			if strings.HasPrefix(p, "internal/") && strings.HasSuffix(p, ".go") {
				return true
			}
		}
		return false
	}
	return check(created) || check(modified)
}

// ── Plan implementation ──────────────────────────────────────────────────

// plan runs Hermes as an AgentRunner with read-only tools so it can investigate
// the codebase before proposing a plan. Falls back to planStream (zero-tool stream)
// when readonlyTools is nil — e.g. in tests or when no read-only registry is wired.
func (h *Hermes) plan(ctx context.Context, input string) (string, error) {
	if h.readonlyTools != nil {
		return h.planWithTools(ctx, input)
	}
	return h.planStream(ctx, input)
}

// planStream is the backward-compatible zero-tool stream fallback, used when
// Hermes is constructed without a read-only tool registry (e.g. in tests).
func (h *Hermes) planStream(ctx context.Context, input string) (string, error) {
	msgs := make([]provider.Message, len(h.hermesSess.Messages)+1)
	copy(msgs, h.hermesSess.Messages)
	msgs[len(msgs)-1] = provider.Message{Role: provider.RoleUser, Content: input}

	ch, err := h.hermesProvider.Stream(ctx, provider.Request{
		Messages:    msgs,
		Temperature: h.temperature,
	})
	if err != nil {
		return "", err
	}

	var text strings.Builder
	var usage *provider.Usage
	for chunk := range ch {
		switch chunk.Type {
		case provider.ChunkText:
			text.WriteString(chunk.Text)
			h.sink.Emit(event.Event{Kind: event.Text, Text: chunk.Text})
		case provider.ChunkUsage:
			usage = chunk.Usage
		case provider.ChunkError:
			return "", chunk.Err
		}
	}
		h.sink.Emit(event.Event{Kind: event.Usage, Usage: usage, Pricing: h.hermesPricing, UsageSource: event.UsageSourcePlanner})

	plan := text.String()
	// Persist this turn into the session so the planner accumulates history
	// across turns, matching the planWithTools path.
	h.hermesSess.Add(provider.Message{Role: provider.RoleUser, Content: input})
	h.hermesSess.Add(provider.Message{Role: provider.RoleAssistant, Content: plan})
	return plan, nil
}

// planWithTools runs the persistent planner Agent with read-only tools.
// V10.36: uses the persistent plannerAgent (created in NewHermes) instead of
// building a temporary AgentRunner each turn. The planner's session accumulates
// planning history + execution results across turns; compaction keeps it bounded.
func (h *Hermes) planWithTools(ctx context.Context, input string) (string, error) {
	if h.plannerAgent == nil {
		return "", fmt.Errorf("hermes: planner agent not initialized (no read-only tools)")
	}
	// Re-propagate asker to plannerAgent before each planning run,
	// ensuring the ask tool can interact with the user even if
	// SetAsker was called before plannerAgent was created.
	if h.asker != nil {
		h.plannerAgent.SetAsker(h.asker)
	}
	turnResult, err := h.plannerAgent.Run(ctx, input)
	if err != nil {
		return "", fmt.Errorf("hermes: %w", err)
	}
	if turnResult != nil && turnResult.Summary != "" {
		slog.Info("hermes: planner run summary", "summary", turnResult.Summary)
	}

	// Extract the plan from the planner's persistent session (last assistant message).
	var plan string
	msgs := h.hermesSess.Messages
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleAssistant {
			plan = msgs[i].Content
			break
		}
	}
	if plan == "" {
		return "", fmt.Errorf("hermes: planner produced no output")
	}
	return plan, nil
}

// autoGate approves every tool call — safe for read-only planners.
type autoGate struct{}

func (g *autoGate) Check(_ context.Context, _ string, _ json.RawMessage, _ bool) (bool, string, error) {
	return true, "", nil
}

// ── Plan helpers ─────────────────────────────────────────────────────

// shouldSkipPlanner detects tasks that are simple enough to execute directly,
// V10.34: only the explicit "!" marker skips the planner.
func shouldSkipPlanner(input string) (string, bool) {
	if stripped, ok := strings.CutPrefix(input, "!"); ok {
		return strings.TrimSpace(stripped), true
	}
	return "", false
}

// isAnswerNotAction checks whether the planner's output is a direct answer
// that needs no executor. The planner self-marks executable plans with
// <!--plan--> — if absent, Hermes answered directly.
func isAnswerNotAction(plan string) bool {
	trimmed := strings.TrimSpace(plan)
	return !strings.Contains(trimmed, "<!--plan-->")
}

func formatHandoff(task, plan, userNote string) string {
	note := ""
	if userNote != "" {
		note = fmt.Sprintf("\n\n📌 User note (written during plan confirmation):\n%s\n", userNote)
	}
	return fmt.Sprintf(`# %s

Original task:
%s

Hermes output:
%s%s`, hephaestusHandoffMarker, task, plan, note)
}

// HandoffTask returns the original user task embedded in an executor handoff
// message, or s unchanged when it is not one. Session previews and auto-titles
// use it so dual-model sessions surface the user's words, not the handoff
// boilerplate.
func HandoffTask(s string) string {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "# "+hephaestusHandoffMarker) {
		return s
	}
	const header = "Original task:\n"
	i := strings.Index(trimmed, header)
	if i < 0 {
		return s
	}
	rest := trimmed[i+len(header):]
	if j := strings.Index(rest, "\n\nHermes output:"); j >= 0 {
		rest = rest[:j]
	}
	if task := strings.TrimSpace(rest); task != "" {
		return task
	}
	return s
}
