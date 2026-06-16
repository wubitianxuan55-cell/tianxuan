package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"tianxuan/internal/jobs"
	"tianxuan/internal/tool"
)

// bash_output / kill_shell / wait operate the background jobs registered by
// bash(run_in_background) and task(run_in_background). They reach the session's
// job manager through the call context (jobs.FromContext) — the agent stamps it
// onto every tool call — and degrade to a clear error when it isn't available
// (a headless context with no manager). Together they poll a job's new output,
// terminate a job, and block until jobs finish.

func init() {
	tool.RegisterBuiltin(bashOutput{})
	tool.RegisterBuiltin(killShell{})
	tool.RegisterBuiltin(waitJob{})
}

// --- bash_output: poll a background job's new output (non-blocking) ---

type bashOutput struct{}

func (bashOutput) Name() string { return "bash_output" }

func (bashOutput) Description() string {
	return "Read new output from a background job started with bash(run_in_background=true) or task(run_in_background=true). Returns the output produced since the last bash_output call for that job, plus its status (running/done/failed/killed). Does not block."
}

func (bashOutput) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"job_id":{"type":"string","description":"The background job id (e.g. \"bash-1\") returned when it was started."},"filter":{"type":"string","description":"Optional regular expression; only matching lines of the new output are returned."}},"required":["job_id"]}`)
}

func (bashOutput) ReadOnly() bool { return true }

func (bashOutput) CompactDescription() string { return compactDesc["bash_output"] }
func (bashOutput) CompactSchema() json.RawMessage   { return compactSchema["bash_output"] }

func (bashOutput) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		JobID  string `json:"job_id"`
		Filter string `json:"filter"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.JobID == "" {
		return "", fmt.Errorf("job_id is required")
	}
	jm, ok := jobs.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("background jobs are not available in this context")
	}
	text, status, found := jm.Output(p.JobID)
	if !found {
		return "", fmt.Errorf("no background job %q", p.JobID)
	}
	if p.Filter != "" && text != "" {
		filtered, err := filterLines(text, p.Filter)
		if err != nil {
			return "", err
		}
		text = filtered
	}
	header := fmt.Sprintf("[%s] %s", p.JobID, status)
	if strings.TrimSpace(text) == "" {
		return header + "\n(no new output)", nil
	}
	return header + "\n" + text, nil
}

// filterLines keeps only the lines of s matching the regular expression re.
func filterLines(s, re string) (string, error) {
	rx, err := regexp.Compile(re)
	if err != nil {
		return "", fmt.Errorf("invalid filter regexp: %w", err)
	}
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		if rx.MatchString(line) {
			keep = append(keep, line)
		}
	}
	return strings.Join(keep, "\n"), nil
}

// --- kill_shell: terminate a running background job ---

type killShell struct{}

func (killShell) Name() string { return "kill_shell" }

func (killShell) Description() string {
	return "Terminate a running background job (bash or task) started with run_in_background. A no-op if the job has already finished or the id is unknown."
}

func (killShell) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"job_id":{"type":"string","description":"The background job id to terminate (e.g. \"bash-1\")."}},"required":["job_id"]}`)
}

func (killShell) ReadOnly() bool { return false }

func (killShell) CompactDescription() string { return compactDesc["kill_shell"] }
func (killShell) CompactSchema() json.RawMessage   { return compactSchema["kill_shell"] }

func (killShell) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	if p.JobID == "" {
		return "", fmt.Errorf("job_id is required")
	}
	jm, ok := jobs.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("background jobs are not available in this context")
	}
	// 1. Cancel the job context (kills the immediate process).
	jm.Kill(p.JobID)

	// 2. On Windows, also attempt taskkill /T /F to handle orphaned child processes
	//    (e.g. Python/Node servers started by the background shell).
	killOutput := ""
	if runtime.GOOS == "windows" {
		if pid := jm.Pid(p.JobID); pid > 0 {
			killCmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
			killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			killCmd.Stdout = io.Discard
			killCmd.Stderr = io.Discard
			if err := killCmd.Run(); err == nil {
				killOutput = " process tree terminated."
			}
		}
	}
	return fmt.Sprintf("Killed background job %q.%s", p.JobID, killOutput), nil
}

// --- wait: block until background jobs finish, then return their results ---

type waitJob struct{}

func (waitJob) Name() string { return "wait" }

func (waitJob) Description() string {
	return "Block until background jobs finish, then return each job's status and final output/answer. Use to collect the result of a task(run_in_background) or bash(run_in_background) before continuing. Omit job_ids to wait for every running job."
}

func (waitJob) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"job_ids":{"type":"array","items":{"type":"string"},"description":"Background job ids to wait for. Omit to wait for every currently-running job."},"timeout_seconds":{"type":"integer","description":"Optional maximum seconds to block before returning current progress. Omit to wait until the jobs finish.","minimum":1}}}`)
}

func (waitJob) ReadOnly() bool { return true }

func (waitJob) CompactDescription() string { return compactDesc["wait"] }
func (waitJob) CompactSchema() json.RawMessage   { return compactSchema["wait"] }

func (waitJob) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		JobIDs         []string `json:"job_ids"`
		TimeoutSeconds int      `json:"timeout_seconds"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &p); err != nil {
			return "", fmt.Errorf("invalid args: %w", err)
		}
	}
	jm, ok := jobs.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("background jobs are not available in this context")
	}
	timeout := p.TimeoutSeconds
	if timeout <= 0 {
		timeout = 300 // 默认 5 分钟，防止无限等待
	}
	results := jm.Wait(ctx, p.JobIDs, timeout)
	if len(results) == 0 {
		return "No background jobs to wait for.", nil
	}
	var b strings.Builder
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n\n")
		}
		label := r.ID
		if r.Label != "" {
			label = fmt.Sprintf("%s (%s)", r.ID, r.Label)
		}
		fmt.Fprintf(&b, "[%s] %s", label, r.Status)
		if strings.TrimSpace(r.Output) != "" {
			b.WriteString("\n" + r.Output)
		}
	}
	return b.String(), nil
}
