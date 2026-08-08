package builtin

import (
	"strings"
	"testing"
	"time"

	"tianxuan/internal/event"
	"tianxuan/internal/jobs"
	"tianxuan/internal/sandbox"
)

// TestWriteStdinEndToEnd drives a real interactive process: bash starts a
// background job that reads a line from stdin and echoes it back, write_stdin
// delivers the input, and the echoed payload appears in the job's output.
// This is the interactive-process capability distilled from codex CLI — a
// background job can receive input mid-run, not just at launch.
func TestWriteStdinEndToEnd(t *testing.T) {
	jm := jobs.NewManager(event.Discard)
	ctx := withTestJobs(jm)
	sh := resolvedTestShell(t)
	b := bash{shell: sh}

	// "read line; echo got:$line" — the shell reads one line from stdin and
	// echoes it. Works in both cmd.exe (set /p) and sh (read). Use a small
	// Python one-liner for portability: python reads stdin line and prints.
	interactiveCmd := `python -c "import sys; print('got:'+sys.stdin.readline().strip())"`
	if sh.Kind == sandbox.ShellPowerShell {
		interactiveCmd = `python -c "import sys; print('got:'+sys.stdin.readline().strip())"`
	}
	out, err := b.Execute(ctx, argsJSON(t, map[string]any{
		"command":           interactiveCmd,
		"run_in_background": true,
		"interactive":       true,
	}))
	if err != nil {
		t.Fatalf("start interactive job failed: %v (out=%q)", err, out)
	}
	jobID := extractJobID(t, out)

	// write_stdin delivers the input. The stdin pipe is registered
	// asynchronously by the job's run goroutine, so retry briefly.
	ws := writeStdin{}
	var res string
	deadline := time.Now().Add(2 * time.Second)
	for {
		var err error
		res, err = ws.Execute(ctx, argsJSON(t, map[string]any{
			"job_id": jobID,
			"data":   "hello-stdin\n",
		}))
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			jm.Kill(jobID)
			t.Fatalf("write_stdin never succeeded: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	jm.Kill(jobID)
	if !strings.Contains(res, "got:hello-stdin") {
		t.Errorf("write_stdin result should contain echoed payload, got %q", res)
	}
}

// TestWriteStdinNonInteractiveErrors: writing to a job started without
// interactive=true must fail loudly with a clear hint.
func TestWriteStdinNonInteractiveErrors(t *testing.T) {
	jm := jobs.NewManager(event.Discard)
	ctx := withTestJobs(jm)
	sh := resolvedTestShell(t)
	b := bash{shell: sh}

	out, err := b.Execute(ctx, argsJSON(t, map[string]any{
		"command":           "python -c \"import time; time.sleep(10)\"",
		"run_in_background": true,
	}))
	if err != nil {
		t.Fatalf("start non-interactive job failed: %v", err)
	}
	jobID := extractJobID(t, out)
	defer jm.Kill(jobID)

	ws := writeStdin{}
	res, err := ws.Execute(ctx, argsJSON(t, map[string]any{
		"job_id": jobID,
		"data":   "x",
	}))
	if err == nil {
		t.Fatalf("write_stdin on non-interactive job should error, got %q", res)
	}
	if !strings.Contains(err.Error(), "interactive") {
		t.Errorf("error should hint interactive=true, got %v", err)
	}
}
