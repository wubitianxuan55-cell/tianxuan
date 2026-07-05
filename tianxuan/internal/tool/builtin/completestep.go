package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"tianxuan/internal/evidence"
	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(completeStep{}) }

// completeStep records an evidence-backed completion of one step of an approved
// plan. Like todo_write it has no host side effects — the claim and its evidence
// live in the call's args, which a frontend renders as a signed-off step. Its
// reason for existing is the enforcement in Execute: a completion with no evidence
// is rejected, so the model can't flip a step to "done" without showing why it is
// done (the verification it ran, the diff/files it changed, or a manual check).
// It complements todo_write — todo_write keeps the list moving (one item
// in_progress), complete_step is the formal sign-off of a finished step.
type completeStep struct{}

type stepEvidence struct {
	Kind    string   `json:"kind"`
	Summary string   `json:"summary"`
	Command string   `json:"command,omitempty"`
	Paths   []string `json:"paths,omitempty"`
}

// validEvidenceKinds are the evidence forms a completion may cite. "checkpoint"
// (main's fourth kind) is omitted — v2 has no checkpoint system.
var validEvidenceKinds = map[string]bool{
	"verification": true, // a command/test was run; cite it and its outcome
	"diff":         true, // a concrete code change; cite what changed
	"files":        true, // files created/edited/inspected; cite the paths
	"manual":       true, // a manual check; cite what was confirmed and how
}

func (completeStep) Name() string { return "complete_step" }

func (completeStep) Description() string {
	return `Sign off ONE plan step with PROOF it is done. At least one evidence item must be verification (run a command via bash and cite its output), diff (show what changed), or files (list paths touched). Manual evidence alone is NOT accepted — combine it with at least one verifiable kind. Fields: step (title/number matching todo_write), result (what changed), evidence (≥1 item, at least one non-manual), optional notes.`
}

func (completeStep) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{
  "step":{"type":"string","description":"Which plan step this completes — its title or number, matching the task list."},
  "step_index":{"type":"integer","minimum":1,"description":"Optional 1-based task-list item number. Prefer this when the step title is long or easy to mistype."},
  "result":{"type":"string","description":"What is now true or changed as a result of finishing this step."},
  "evidence":{
    "type":"array",
    "minItems":1,
    "description":"Proof the step is done. At least one item is required.",
    "items":{
      "type":"object",
      "properties":{
        "kind":{"type":"string","enum":["verification","diff","files","manual"],"description":"verification = a command/test was run; diff = a concrete code change; files = files created/edited/inspected; manual = a manual check."},
        "summary":{"type":"string","description":"The evidence itself: the test result, what the diff does, or what was confirmed."},
        "command":{"type":"string","description":"The command run, for verification evidence (e.g. \"go test ./...\")."},
        "paths":{"type":"array","items":{"type":"string"},"description":"Files this evidence refers to."}
      },
      "required":["kind","summary"]
    }
  },
  "notes":{"type":"string","description":"Optional caveats, follow-ups, or anything deferred."}
},
"required":["step","result","evidence"]
}`)
}

// ReadOnly is true: complete_step only records a claim (no filesystem or process
// effect), so it never needs approval and stays available alongside todo_write.
func (completeStep) ReadOnly() bool { return true }

func (completeStep) CompactDescription() string { return compactDesc["complete_step"] }
func (completeStep) CompactSchema() json.RawMessage   { return compactSchema["complete_step"] }

func (completeStep) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Step      string         `json:"step"`
		StepIndex *int           `json:"step_index,omitempty"`
		Result    string         `json:"result"`
		Evidence  []stepEvidence `json:"evidence"`
		Notes     string         `json:"notes"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if strings.TrimSpace(p.Step) == "" && p.StepIndex == nil {
		return "", fmt.Errorf("step or step_index is required — name the plan step you are completing")
	}
	if strings.TrimSpace(p.Result) == "" {
		return "", fmt.Errorf("result is required — state what is now true after finishing this step")
	}
	if len(p.Evidence) == 0 {
		return "", fmt.Errorf("at least one evidence item is required — don't mark a step complete without showing why it's done (run a check, cite the diff, or confirm manually)")
	}
	kinds := make([]string, 0, len(p.Evidence))
	for i, e := range p.Evidence {
		if !validEvidenceKinds[e.Kind] {
			return "", fmt.Errorf("evidence %d: invalid kind %q (want verification|diff|files|manual)", i+1, e.Kind)
		}
		if strings.TrimSpace(e.Summary) == "" {
			return "", fmt.Errorf("evidence %d: summary is required — the evidence is the summary, not just its kind", i+1)
		}
		kinds = append(kinds, e.Kind)
	}

	hostVerified, manualUnverified, err := verifyStepEvidence(ctx, p.Evidence)
	if err != nil {
		return "", err
	}
	todoMatch, hasTodo, err := verifyTodoStep(ctx, p.Step)
	if err != nil {
		return "", err
	}
	// 编码铁律: manual 证据不能单独作为签退依据——至少需要一条可验证证据
	if hostVerified == 0 && manualUnverified > 0 {
		return "", fmt.Errorf("all evidence is manual (%d item(s)) — at least one verification, diff, or files evidence is required; run a check with bash and cite its output, or cite a concrete code change", manualUnverified)
	}
	hostStatus := ""
	if _, ok := evidence.FromContext(ctx); ok {
		hostStatus = fmt.Sprintf(" Host evidence: host-verified %d, manual/unverified %d.", hostVerified, manualUnverified)
	}
	todoStatus := ""
	if hasTodo {
		todoStatus = fmt.Sprintf(" Todo step: todo-matched %d.", todoMatch.Index)
	}
	return fmt.Sprintf("Step %q signed off with %d evidence item(s) [%s].%s The host advances the task list for you — it marks this step completed and moves the next to in_progress, so you don't need another todo_write to mark completions.",
		p.Step, len(p.Evidence), strings.Join(kinds, ", "), hostStatus+todoStatus), nil
}

func verifyStepEvidence(ctx context.Context, items []stepEvidence) (hostVerified int, manualUnverified int, err error) {
	ledger, ok := evidence.FromContext(ctx)
	if !ok {
		return 0, 0, nil
	}
	for i, e := range items {
		switch e.Kind {
		case "verification":
			command := strings.TrimSpace(e.Command)
			if command == "" {
				return 0, 0, fmt.Errorf("evidence %d: verification command is required for host verification", i+1)
			}
			if !ledger.HasSuccessfulCommand(command) {
				hint := commandHints(ledger)
				return 0, 0, fmt.Errorf("evidence %d: verification command %q has no matching successful bash receipt in this turn%s", i+1, command, hint)
			}
			hostVerified++
		case "diff":
			if len(e.Paths) == 0 {
				return 0, 0, fmt.Errorf("evidence %d: diff evidence requires paths for host verification", i+1)
			}
			if !ledger.HasSuccessfulWrite(e.Paths) {
				return 0, 0, fmt.Errorf("evidence %d: diff paths have no matching successful writer receipt in this turn", i+1)
			}
			hostVerified++
		case "files":
			if len(e.Paths) == 0 {
				return 0, 0, fmt.Errorf("evidence %d: files evidence requires paths for host verification", i+1)
			}
			if !ledger.HasSuccessfulReadOrWrite(e.Paths) {
				return 0, 0, fmt.Errorf("evidence %d: file paths have no matching successful read/write receipt in this turn", i+1)
			}
			hostVerified++
		case "manual":
			manualUnverified++
		}
	}
	return hostVerified, manualUnverified, nil
}

func verifyTodoStep(ctx context.Context, step string) (evidence.TodoStepMatch, bool, error) {
	ledger, ok := evidence.FromContext(ctx)
	if !ok {
		return evidence.TodoStepMatch{}, false, nil
	}
	match, hasTodo := ledger.MatchLatestTodoStep(step)
	if !hasTodo {
		return evidence.TodoStepMatch{}, false, nil
	}
	if !match.Found {
		return evidence.TodoStepMatch{}, true, fmt.Errorf("step %q has no matching todo_write item in this turn", step)
	}
	switch match.Status {
	case "in_progress", "completed":
		return match, true, nil
	case "":
		return evidence.TodoStepMatch{}, true, fmt.Errorf("step %q matches todo %d (%q) but its status is pending; complete_step requires in_progress or completed", step, match.Index, match.Content)
	default:
		return evidence.TodoStepMatch{}, true, fmt.Errorf("step %q matches todo %d (%q) but its status is %q; complete_step requires in_progress or completed", step, match.Index, match.Content, match.Status)
	}
}

func commandHints(ledger *evidence.Ledger) string {
	cmds := ledger.SuccessfulCommands(5)
	if len(cmds) == 0 {
		return ""
	}
	for i, c := range cmds {
		if len(c) > 80 {
			cmds[i] = c[:80] + "…"
		}
	}
	return fmt.Sprintf("; successful commands this turn: %s — cite one exactly as it ran", strings.Join(cmds, ", "))
}
