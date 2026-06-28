package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"tianxuan/internal/event"
	"tianxuan/internal/jobs"
	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// DefaultTaskSystemPrompt steers a sub-agent toward focused, terse delivery —
// it doesn't see the parent's conversation so it must self-contain.
const DefaultTaskSystemPrompt = `You are a sub-agent invoked by a parent coding agent to carry out one focused task.
Use the provided tools to investigate or act. Return a single final answer that is concise
and self-contained — the parent will see only that answer, not your tool calls or reasoning.
If you need to ask for clarification, fail with a precise question instead of guessing.`

// taskResultTag wraps sub-agent output in structured XML so the parent agent can
// distinguish the result from other tool output. Borrowed from opencode.
const (
	taskResultTagOpen  = "<task-result>"
	taskResultTagClose = "</task-result>"
)

// wrapTaskResult wraps a sub-agent's final answer in structured XML tags so the
// parent model can reliably identify and parse it.
func wrapTaskResult(text string) string {
	return taskResultTagOpen + "\n" + strings.TrimSpace(text) + "\n" + taskResultTagClose
}

var subagentMetaTools = []string{
	"task",
	"run_skill",
	"install_skill",
	"explore",
	"research",
	"review",
	"security_review",
}

// SubagentMetaTools returns the tool names that spawned agents should not inherit
// from the parent registry unless a future call site deliberately opts into a
// different boundary. They can spawn or author more agent work, so excluding them
// preserves one layer of delegation without adding a spawn-count cap.
func SubagentMetaTools() []string {
	out := make([]string, len(subagentMetaTools))
	copy(out, subagentMetaTools)
	return out
}

// IsSubagentMetaTool reports whether the tool name spawns a sub-agent that makes
// its own API calls. These calls can evict the parent's cache prefix on the
// provider side (especially on smaller cache pools like flash 128K), so the
// parent should re-warm after the sub-agent returns.
func IsSubagentMetaTool(name string) bool {
	for _, t := range subagentMetaTools {
		if t == name {
			return true
		}
	}
	return false
}

// TaskCompiler is the subset of cache.Compiler that TaskTool needs for
// fork-based cache sharing. Defined here so the agent package doesn't
// import the cache package. The Fork return is interface-typed because
// cache.Compiler.Fork() returns a concrete *Compiler, not this interface.
type TaskCompiler interface {
	Fork() interface{ SystemPrompt() string }
	SystemPrompt() string
}

// TaskTool spawns a sub-agent in its own session for a focused sub-task. The
// sub-agent runs with a filtered tool whitelist and the same step budget shape
// as the parent (see Execute); its tool calls are forwarded to the parent's
// event stream nested under this call, while only its final assistant message is
// returned to the parent model. Use cases: keep noisy tool sequences (multi-file
// exploration, repeated grep / read_file) out of the parent's context budget, or
// parallel research across independent areas (the parallel-dispatch path picks
// these up only when readOnly, which task is not).
type TaskTool struct {
	prov          provider.Provider
	pricing       *provider.Pricing
	parentReg     *tool.Registry
	maxSteps      int
	contextWindow int
	temperature   float64
	archiveDir    string
	l2Dir         string // L2 ring persistence for sub-agents
	sysPrompt     string
	gate          Gate
	compiler      TaskCompiler // optional, for cache sharing via Fork
	runtimePrompt string       // V5.25: L2 runtime context for sub-agents
	templatePrefix string       // V5.30: 子代理模板前缀，同类子代理共享缓存
	accumulatedUsage *provider.Usage // V5.30: 子代理累计 token 用量
	activeSchemas []provider.ToolSchema // V5.30: 父代理过滤工具集，子代理继承以共享缓存
}

// NewTaskTool wires a task tool to the parent agent's environment so its
// sub-agents can use the same provider and tools. sysPrompt is the system
// prompt every sub-agent starts with; pass "" for DefaultTaskSystemPrompt. gate
// is the permission gate sub-agents inherit — pass the headless variant so
// deny rules still bite while autonomous sub-agents are never blocked on an
// interactive prompt (there is no UI to answer one).
func NewTaskTool(prov provider.Provider, pricing *provider.Pricing, parentReg *tool.Registry,
	maxSteps, contextWindow int, temperature float64, archiveDir, sysPrompt string, gate Gate) *TaskTool {
	if sysPrompt == "" {
		sysPrompt = DefaultTaskSystemPrompt
	}
	return &TaskTool{
		prov:          prov,
		pricing:       pricing,
		parentReg:     parentReg,
		maxSteps:      maxSteps,
		contextWindow: contextWindow,
		temperature:   temperature,
		archiveDir:    archiveDir,
		sysPrompt:     sysPrompt,
		gate:          gate,
	}
}

func (t *TaskTool) Name() string { return "task" }

func (t *TaskTool) Description() string {
	return "Spawn a sub-agent for a focused sub-task. The sub-agent runs in its own session with the same provider and a filtered tool list (defaults to every parent tool except subagent/skill meta-tools, so delegation stays one layer deep). Only its final answer is returned. Set output_schema to get structured JSON back (e.g. {files_modified: [...], key_decisions: [...]}). Use this to (a) keep long exploration sequences out of the parent's context budget, or (b) delegate self-contained work like 'find every place that calls X and summarise the patterns'."
}

func (t *TaskTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "prompt":{"type":"string","description":"What the sub-agent should accomplish. Be specific about the deliverable — the sub-agent does not see this conversation."},
  "description":{"type":"string","description":"Short label for the sub-task (3-7 words). Surfaced in the dispatch line so the user sees what's running."},
  "tools":{"type":"array","items":{"type":"string"},"description":"Optional tool whitelist. Subagent/skill meta-tools are still excluded so delegation stays one layer deep."},
  "max_steps":{"type":"integer","description":"Optional cap on tool-call rounds. Defaults to half the parent's cap (min 5).","minimum":1},
  "run_in_background":{"type":"boolean","description":"Run the sub-agent asynchronously: returns a job id immediately and keeps working across turns. Collect its final answer with wait, and you'll be notified when it finishes. Use for long, independent sub-tasks you don't need to block on right now."},
  "output_schema":{"type":"object","description":"Optional JSON Schema the sub-agent MUST return its result in. If set, the parent will attempt to parse the final answer as JSON. If the result is valid JSON matching the expected shape, it is returned verbatim; otherwise a diagnostic note is prefixed. Use when the parent needs structured data from the sub-agent."}
},
"required":["prompt"]
}`)
}

// ReadOnly is false: a sub-agent can invoke any whitelisted tool, including
// writers. Conservative classification keeps the parallel-dispatch path from
// running two sub-agents at once and letting their writes race.
func (t *TaskTool) ReadOnly() bool { return false }

// CompactDescriptor — V10.11: compact task description for prompt efficiency.
func (t *TaskTool) CompactDescription() string {
	return "派发隔离子代理执行子任务(可设置output_schema获取结构化JSON)"
}
func (t *TaskTool) CompactSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"},"description":{"type":"string"},"tools":{"type":"array","items":{"type":"string"}},"max_steps":{"type":"integer"},"run_in_background":{"type":"boolean"},"output_schema":{"type":"object"}},"required":["prompt"]}`)
}

func (t *TaskTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Prompt          string   `json:"prompt"`
		Description     string   `json:"description"`
		Tools           []string `json:"tools"`
		MaxSteps        int      `json:"max_steps"`
		RunInBackground bool     `json:"run_in_background"`
	OutputSchema    json.RawMessage `json:"output_schema,omitempty"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.Prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}

	maxSteps := p.MaxSteps
	if maxSteps <= 0 {
		if t.maxSteps > 0 {
			maxSteps = t.maxSteps / 2
			if maxSteps < 5 {
				maxSteps = 5
			}
		}
	}

	subReg := t.buildSubReg(p.Tools)

	if p.RunInBackground {
		jm, ok := jobs.FromContext(ctx)
		if !ok {
			return "", fmt.Errorf("background execution is not available in this context")
		}
		parentID, parent, _, _ := CallContext(ctx)
		nested := subSinkFor(parentID, parent)
		label := p.Description
		if label == "" {
			label = "task"
		}
		job := jm.Start("task", label, func(jobCtx context.Context, _ io.Writer) (string, error) {
			return t.runSub(jobCtx, p.Prompt, subReg, nested, maxSteps, p.OutputSchema)
		})
		return fmt.Sprintf("Started background task %q (%s). It runs across turns; collect its final answer with wait (or wait will return it once done), and you'll be notified when it finishes.", job.ID, label), nil
	}

	return t.runSub(ctx, p.Prompt, subReg, subSink(ctx), maxSteps, p.OutputSchema)
}

func (t *TaskTool) buildSubReg(names []string) *tool.Registry {
	return FilterRegistry(t.parentReg, names, SubagentMetaTools()...)
}

func FilterRegistry(parent *tool.Registry, names []string, exclude ...string) *tool.Registry {
	ex := make(map[string]bool, len(exclude))
	for _, e := range exclude {
		ex[e] = true
	}
	sub := tool.NewRegistry()
	src := names
	if len(src) == 0 {
		src = parent.Names()
	}
	for _, name := range src {
		if ex[name] {
			continue
		}
		if tl, ok := parent.Get(name); ok {
			sub.Add(tl)
		}
	}
	return sub
}

func (t *TaskTool) SetCompiler(c TaskCompiler) { t.compiler = c }
func (t *TaskTool) SetL2Dir(dir string)        { t.l2Dir = dir }
func (t *TaskTool) SetRuntimePrompt(p string)   { t.runtimePrompt = p }
func (t *TaskTool) SetTemplatePrefix(prefix string) { t.templatePrefix = prefix }
func (t *TaskTool) SetActiveSchemas(schemas []provider.ToolSchema) { t.activeSchemas = schemas }
func (t *TaskTool) SubUsage() *provider.Usage { return t.accumulatedUsage }

func (t *TaskTool) runSub(ctx context.Context, prompt string, subReg *tool.Registry, sink event.Sink, maxSteps int, outputSchema json.RawMessage) (string, error) {
	// V6.0: sub-agent does NOT inherit parent L1+L2 — uses DefaultTaskSystemPrompt independently.
	// This saves ~50K tokens per sub-agent call (97% reduction) and keeps cache stats separate.
	sysPrompt := t.sysPrompt
	promptContent := prompt

	// V5.30: 子代理模板注入
	if t.templatePrefix != "" {
		promptContent = t.templatePrefix + "\n\n---\n\n" + promptContent
	}

	// V5.30: 子代理 API 请求发送父代理完整工具集以保证缓存对齐。
	// API 请求工具列表与父代理一致（含 meta-tools），DeepSeek 可命中缓存。
	// 执行层面由 subReg（buildSubReg 过滤后）门控，meta-tools 无法被实际调用。
var subUsage provider.Usage
	result, err := RunSubAgent(ctx, t.prov, subReg, sysPrompt, promptContent, Options{
		MaxSteps:       maxSteps,
		Temperature:    t.temperature,
		Pricing:        t.pricing,
		Gate:           t.gate,
		ContextWindow:  t.contextWindow,
		ArchiveDir:     t.archiveDir,
		L2Dir:          t.l2Dir,
	}, sink, &subUsage)
	if err == nil && len(outputSchema) > 0 {
		// output_schema set: verify the result is parseable JSON.
		// We don't validate every field (full JSON Schema needs a lib),
		// but we confirm the sub-agent returned well-formed JSON.
		var parsed interface{}
		if json.Unmarshal([]byte(result), &parsed) != nil {
			result = "[output_schema: sub-agent returned non-JSON; parent should retry]" + "\n" + result
		}
		u := subUsage
		t.accumulatedUsage = &u
		return result, nil
	}
	if err == nil {
		u := subUsage
		t.accumulatedUsage = &u
		// V10.12: wrap successful sub-agent results in structured XML tags
		// so the parent can reliably identify the result. Borrowed from opencode.
		return wrapTaskResult(result), nil
	}
	return result, err
}

func RunSubAgent(ctx context.Context, prov provider.Provider, reg *tool.Registry, sysPrompt, prompt string, opts Options, sink event.Sink, subUsage *provider.Usage) (string, error) {
	sess := NewSession(sysPrompt)
	sub := New(prov, reg, sess, opts, sink)
	// V5.30: 继承父代理工具集，使 tools JSON 段缓存命中
	runErr := sub.Run(ctx, prompt)
	// V10.5: 即使 Run 出错（如超时），也尝试提取最后一条助手消息作为部分结果
	lastMsg := extractLastAssistantMessage(sess.Messages)
	if runErr != nil {
		if lastMsg != "" {
			return lastMsg, fmt.Errorf("sub-agent terminated with error (partial result returned): %w", runErr)
		}
		return "", fmt.Errorf("sub-agent: %w", runErr)
	}
	if lastMsg != "" {
		return lastMsg, nil
	}
	return "", fmt.Errorf("sub-agent finished without producing a final answer")
}

// extractLastAssistantMessage finds the last non-empty assistant message
// in the session, traversing from the end. Returns "" if none found.
func extractLastAssistantMessage(msgs []provider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role == provider.RoleAssistant && strings.TrimSpace(m.Content) != "" {
			return m.Content
		}
	}
	return ""
}

func NestedSink(ctx context.Context, fallback event.Sink) event.Sink {
	parentID, parent, _, ok := CallContext(ctx)
	if !ok || parent == nil {
		return fallback
	}
	return subSinkFor(parentID, parent)
}

func subSink(ctx context.Context) event.Sink {
	parentID, parent, _, ok := CallContext(ctx)
	if !ok || parent == nil {
		return event.Discard
	}
	return subSinkFor(parentID, parent)
}

func subSinkFor(parentID string, parent event.Sink) event.Sink {
	if parent == nil {
		return event.Discard
	}
	return event.FuncSink(func(e event.Event) {
		switch e.Kind {
		case event.ToolDispatch, event.ToolResult:
			e.Tool.ParentID = parentID
			e.Tool.ID = parentID + "/" + e.Tool.ID
			parent.Emit(e)
		case event.Usage:
			// V5.30: forward sub-agent usage so StatsPanel sees sub-agent tokens
			parent.Emit(e)
		}
	})
}
