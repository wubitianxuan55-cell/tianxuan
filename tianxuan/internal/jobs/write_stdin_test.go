package jobs

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"tianxuan/internal/event"
)

// TestWriteStdinDeliversToCommand delivers data to a job's stdin pipe and the
// job's command reads it. This is the foundation of the interactive-process
// capability (distilled from codex CLI's write_stdin): a background job started
// with an stdin pipe can receive input mid-run instead of only at launch.
func TestWriteStdinDeliversToCommand(t *testing.T) {
	jm := NewManager(event.Discard)
	job := jm.Start("bash", "cat-interactive", func(ctx context.Context, out io.Writer) (string, error) {
		r, w := io.Pipe()
		jm.SetStdin(jobIDFromRun(ctx), w)
		go func() {
			buf := make([]byte, 1024)
			n, _ := r.Read(buf)
			out.Write([]byte("got:" + string(buf[:n])))
			r.Close()
		}()
		<-ctx.Done()
		return "", ctx.Err()
	})

	// Wait for the pipe to be registered, then write.
	deadline := time.Now().Add(2 * time.Second)
	var writeErr error
	for time.Now().Before(deadline) {
		writeErr = jm.WriteStdin(job.ID, "hello-stdin")
		if writeErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if writeErr != nil {
		t.Fatalf("WriteStdin failed: %v", writeErr)
	}

	// Poll output for the echoed payload.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		text, _, _ := jm.Output(job.ID)
		if strings.Contains(text, "got:hello-stdin") {
			jm.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	text, _, _ := jm.Output(job.ID)
	jm.Close()
	t.Fatalf("job never echoed stdin payload; output = %q", text)
}

// TestWriteStdinUnknownJobErrors: writing to a job that doesn't exist must
// fail loudly rather than silently no-op.
func TestWriteStdinUnknownJobErrors(t *testing.T) {
	jm := NewManager(event.Discard)
	defer jm.Close()
	if err := jm.WriteStdin("no-such-job", "x"); err == nil {
		t.Fatal("WriteStdin on unknown job should error")
	}
}

// TestWriteStdinAfterDoneErrors: writing to a finished job must fail loudly.
func TestWriteStdinAfterDoneErrors(t *testing.T) {
	jm := NewManager(event.Discard)
	job := jm.Start("bash", "quick", func(ctx context.Context, out io.Writer) (string, error) {
		out.Write([]byte("done"))
		return "ok", nil
	})
	defer jm.Close()
	// Wait for the job to finish.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, status, _ := jm.Output(job.ID); status != Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := jm.WriteStdin(job.ID, "x"); err == nil {
		t.Fatal("WriteStdin on finished job should error")
	}
}

func jobIDFromRun(ctx context.Context) string {
	id, _ := JobIDFromContext(ctx)
	return id
}
