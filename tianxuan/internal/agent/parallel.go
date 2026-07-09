package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"tianxuan/internal/provider"
	"tianxuan/internal/tool"
)

// ParallelTasksTool dispatches multiple independent sub-agent tasks concurrently
// and returns a merged summary of all results. It is the task-level counterpart
// to parallel_skills — instead of dispatching named skills it dispatches
// arbitrary sub-agent prompts, each running in its own isolated session.
type ParallelTasksTool struct {
	prov          provider.Provider
	pricing       *provider.Pricing
	parentReg     *tool.Registry
	maxSteps      int
	contextWindow int
	temperature   float64
	archiveDir    string
	sysPrompt     string
	gate          Gate
	compiler      TaskCompiler
	runtimePrompt string
	activeSchemas []provider.ToolSchema
}

// NewParallelTasksTool creates a parallel_tasks tool wired to the parent agent's
// environment. Sub-agents reuse the same provider and inherit a filtered tool
// registry (minus subagent/skill meta-tools, so delegation stays one layer deep).
func NewParallelTasksTool(
	prov provider.Provider,
	pricing *provider.Pricing,
	parentReg *tool.Registry,
	maxSteps int,
	contextWindow int,
	temperature float64,
	archiveDir string,
	sysPrompt string,
	gate Gate,
) *ParallelTasksTool {
	if sysPrompt == "" {
		sysPrompt = DefaultTaskSystemPrompt
	}
	return &ParallelTasksTool{
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

func (t *ParallelTasksTool) Name() string { return "parallel_tasks" }

func (t *ParallelTasksTool) ReadOnly() bool { return false }

func (t *ParallelTasksTool) Description() string {
	return "Dispatch multiple read-only sub-agent tasks concurrently and collect their results. Each task runs in its own isolated session with read-only tools. Use for 2+ independent investigations that share no state — e.g. 'find all uses of X in the frontend' + 'find all uses of X in the backend'. Blocks until all complete."
}

func (t *ParallelTasksTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "tasks":{
    "type":"array",
    "items":{
      "type":"object",
      "properties":{
        "prompt":{"type":"string","description":"The task prompt for the sub-agent."},
        "description":{"type":"string","description":"Optional short label (3-7 words) shown in the job list."},
        "tools":{"type":"array","items":{"type":"string"},"description":"Optional tool whitelist for the sub-agent."},
        "max_steps":{"type":"integer","description":"Optional max tool-call rounds. Defaults to half the parent's cap (min 5).","minimum":1}
      },
      "required":["prompt"]
    },
    "description":"Array of sub-tasks to run in parallel. Each gets its own isolated session."
  }
},
"required":["tasks"]
}`)
}

func (t *ParallelTasksTool) CompactDescription() string {
	return "Run multiple read-only sub-agent tasks concurrently, collect results."
}

func (t *ParallelTasksTool) CompactSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{"prompt":{"type":"string"},"description":{"type":"string"},"tools":{"type":"array","items":{"type":"string"}},"max_steps":{"type":"integer"}},"required":["prompt"]}}},"required":["tasks"]}`)
}

func (t *ParallelTasksTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Tasks []struct {
			Prompt      string   `json:"prompt"`
			Description string   `json:"description"`
			Tools       []string `json:"tools"`
			MaxSteps    int      `json:"max_steps"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("parallel_tasks: invalid args: %w", err)
	}
	if len(p.Tasks) == 0 {
		return "", fmt.Errorf("parallel_tasks: tasks must not be empty")
	}

	type taskResult struct {
		idx     int
		desc    string
		content string
		err     error
	}

	results := make([]taskResult, len(p.Tasks))
	var wg sync.WaitGroup

	for i, task := range p.Tasks {
		wg.Add(1)
		go func(idx int, prompt, desc string, tools []string, maxSteps int) {
			defer wg.Done()

			subCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
			defer cancel()

			if maxSteps <= 0 {
				if t.maxSteps > 0 {
					maxSteps = t.maxSteps / 2
					if maxSteps < 5 {
						maxSteps = 5
					}
				}
			}

			subReg := FilterRegistry(t.parentReg, tools, SubagentMetaTools()...)
			sysPrompt := t.sysPrompt
			if t.compiler != nil {
				forked := t.compiler.Fork()
				sysPrompt = forked.SystemPrompt()
			}

			opts := Options{
				MaxSteps:      maxSteps,
				Temperature:   t.temperature,
				Pricing:       t.pricing,
				Gate:          t.gate,
				ContextWindow: t.contextWindow,
				Compaction:    CompactionConfig{ArchiveDir: t.archiveDir, Window: t.contextWindow},
				RuntimePrompt: t.runtimePrompt,
				ActiveSchemas: t.activeSchemas,
			}

			content, err := RunSubAgent(subCtx, t.prov, subReg, sysPrompt, prompt, opts, NestedSink(ctx, nil), nil)

			results[idx] = taskResult{
				idx:     idx,
				desc:    desc,
				content: content,
				err:     err,
			}
		}(i, task.Prompt, task.Description, task.Tools, task.MaxSteps)
	}

	wg.Wait()

	var b strings.Builder
	b.WriteString("<parallel-tasks-result>\n")
	successes := 0
	failures := 0
	for _, r := range results {
		label := r.desc
		if label == "" {
			label = fmt.Sprintf("Task %d", r.idx+1)
		}
		if r.err != nil {
			b.WriteString(fmt.Sprintf("## %s — ERROR\n%s\n\n", label, r.err))
			failures++
		} else {
			b.WriteString(fmt.Sprintf("## %s — OK\n%s\n\n", label, strings.TrimSpace(r.content)))
			successes++
		}
	}
	b.WriteString(fmt.Sprintf("---\n%d succeeded, %d failed\n</parallel-tasks-result>", successes, failures))
	return b.String(), nil
}

func (t *ParallelTasksTool) SetCompiler(c TaskCompiler) { t.compiler = c }

func (t *ParallelTasksTool) SetRuntimePrompt(p string) { t.runtimePrompt = p }

func (t *ParallelTasksTool) SetActiveSchemas(s []provider.ToolSchema) { t.activeSchemas = s }
