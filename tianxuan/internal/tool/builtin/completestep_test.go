package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"tianxuan/internal/evidence"
)

func TestCompleteStepRejectsMissingEvidence(t *testing.T) {
	_, err := completeStep{}.Execute(context.Background(),
		json.RawMessage(`{"step":"Add the parser","result":"parser added","evidence":[]}`))
	if err == nil {
		t.Fatal("completion with empty evidence should be rejected")
	}
	if !strings.Contains(err.Error(), "evidence") {
		t.Fatalf("error should mention evidence, got %v", err)
	}
}

func TestCompleteStepRequiresStepAndResult(t *testing.T) {
	cases := []string{
		`{"step":"","result":"x","evidence":[{"kind":"manual","summary":"checked"}]}`,
		`{"step":"x","result":"","evidence":[{"kind":"manual","summary":"checked"}]}`,
	}
	for _, c := range cases {
		if _, err := (completeStep{}).Execute(context.Background(), json.RawMessage(c)); err == nil {
			t.Fatalf("expected rejection for %s", c)
		}
	}
}

func TestCompleteStepRejectsBadEvidenceKind(t *testing.T) {
	_, err := completeStep{}.Execute(context.Background(),
		json.RawMessage(`{"step":"x","result":"y","evidence":[{"kind":"vibes","summary":"trust me"}]}`))
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("bad evidence kind should be rejected, got %v", err)
	}
}

func TestCompleteStepRejectsEmptyEvidenceSummary(t *testing.T) {
	_, err := completeStep{}.Execute(context.Background(),
		json.RawMessage(`{"step":"x","result":"y","evidence":[{"kind":"verification","summary":""}]}`))
	if err == nil || !strings.Contains(err.Error(), "summary") {
		t.Fatalf("empty evidence summary should be rejected, got %v", err)
	}
}

func TestCompleteStepAccepts(t *testing.T) {
	out, err := completeStep{}.Execute(context.Background(), json.RawMessage(`{
		"step":"Add the parser",
		"result":"parser added and wired into the loop",
		"evidence":[
			{"kind":"verification","summary":"all tests pass","command":"go test ./..."},
			{"kind":"diff","summary":"new parser.go + call site","paths":["parser.go","loop.go"]}
		]}`))
	if err != nil {
		t.Fatalf("valid completion rejected: %v", err)
	}
	for _, want := range []string{"Add the parser", "2 evidence", "verification", "diff"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ack %q missing %q", out, want)
		}
	}
}

func TestCompleteStepVerifiesHostReceipts(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.Record(evidence.Receipt{
		ToolName: "bash",
		Success:  true,
		Command:  "go test ./internal/...",
	})
	ledger.Record(evidence.Receipt{
		ToolName: "write_file",
		Success:  true,
		Paths:    []string{"internal/evidence/evidence.go"},
		Write:    true,
	})
	ledger.Record(evidence.Receipt{
		ToolName: "read_file",
		Success:  true,
		Paths:    []string{"internal/tool/builtin/completestep.go"},
		Read:     true,
	})
	ctx := evidence.WithLedger(context.Background(), ledger)

	out, err := completeStep{}.Execute(ctx, json.RawMessage(`{
		"step":"Verify receipts",
		"result":"complete_step checks host receipts",
		"evidence":[
			{"kind":"verification","summary":"tests passed","command":"go test ./internal/..."},
			{"kind":"diff","summary":"ledger package added","paths":["internal/evidence/evidence.go"]},
			{"kind":"files","summary":"complete_step implementation inspected","paths":["internal/tool/builtin/completestep.go"]}
		]}`))
	if err != nil {
		t.Fatalf("host-verified evidence rejected: %v", err)
	}
	if !strings.Contains(out, "host-verified 3") {
		t.Fatalf("ack should report host verification, got %q", out)
	}
}

// TestCompleteStepRejectsUnverifiedHostEvidence tests that strict mode (Plan Mode)
// rejects evidence that cannot be verified against host receipts.
func TestCompleteStepRejectsUnverifiedHostEvidence(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.SetStrictVerification(true) // V10.8: 严格验证模式
	ledger.Record(evidence.Receipt{ToolName: "bash", Success: false, Command: "go test ./..."})
	ledger.Record(evidence.Receipt{ToolName: "write_file", Success: true, Paths: []string{"changed.go"}, Write: true})
	ctx := evidence.WithLedger(context.Background(), ledger)

	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "failed verification command",
			body: `{"step":"x","result":"y","evidence":[{"kind":"verification","summary":"claimed tests","command":"go test ./..."}]}`,
			want: "successful bash receipt",
		},
		{
			name: "missing diff writer",
			body: `{"step":"x","result":"y","evidence":[{"kind":"diff","summary":"claimed diff","paths":["other.go"]}]}`,
			want: "successful writer receipt",
		},
		{
			name: "missing file receipt",
			body: `{"step":"x","result":"y","evidence":[{"kind":"files","summary":"claimed file","paths":["other.go"]}]}`,
			want: "successful read/write receipt",
		},
		{
			name: "diff without path",
			body: `{"step":"x","result":"y","evidence":[{"kind":"diff","summary":"claimed diff"}]}`,
			want: "paths",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := completeStep{}.Execute(ctx, json.RawMessage(tc.body))
			if err == nil {
				t.Fatal("unverified host evidence should be rejected")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q missing %q", err, tc.want)
			}
		})
	}
}

func TestCompleteStepRejectsManualOnly(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.SetStrictVerification(true) // V10.8: 严格验证模式
	ctx := evidence.WithLedger(context.Background(), ledger)
	_, err := completeStep{}.Execute(ctx, json.RawMessage(`{
		"step":"Manual check",
		"result":"operator confirmed behavior",
		"evidence":[{"kind":"manual","summary":"checked the visible output"}]}`))
	if err == nil {
		t.Fatal("manual-only evidence should be rejected")
	}
	if !strings.Contains(err.Error(), "all evidence is manual") {
		t.Fatalf("error should mention manual, got %v", err)
	}
}


func TestCompleteStepMatchesTodoReceipt(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.SetStrictVerification(true) // V10.8: 严格验证模式
	ledger.Record(evidence.Receipt{
		ToolName: "bash",
		Success:  true,
		Command:  "go test ./...",
	})
	ledger.Record(evidence.Receipt{
		ToolName: "todo_write",
		Success:  true,
		Todos: []evidence.TodoItem{
			{Content: "Add parser", Status: "in_progress", ActiveForm: "Adding parser"},
			{Content: "Wire parser", Status: "completed"},
		},
	})
	ctx := evidence.WithLedger(context.Background(), ledger)

	for _, step := range []string{"Add parser", "Adding parser", "2"} {
		t.Run(step, func(t *testing.T) {
			out, err := completeStep{}.Execute(ctx, json.RawMessage(`{
				"step":"`+step+`",
				"result":"step is complete",
				"evidence":[{"kind":"verification","summary":"tests pass","command":"go test ./..."}]}`))
			if err != nil {
				t.Fatalf("todo-backed step rejected: %v", err)
			}
			if !strings.Contains(out, "todo-matched") {
				t.Fatalf("ack should mention todo match, got %q", out)
			}
		})
	}
}

func TestCompleteStepRejectsTodoMismatchAndPending(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.SetStrictVerification(true) // V10.8: 严格验证模式
	ledger.Record(evidence.Receipt{
		ToolName: "todo_write",
		Success:  true,
		Todos: []evidence.TodoItem{
			{Content: "Add parser", Status: "in_progress"},
			{Content: "Document parser", Status: "pending"},
		},
	})
	ctx := evidence.WithLedger(context.Background(), ledger)

	cases := []struct {
		name string
		step string
		want string
	}{
		{name: "missing", step: "Ship parser", want: "matching todo_write item"},
	}
	// pending status is now accepted — complete_step no longer rejects it.
	// The test below verifies pending completes successfully with valid evidence.
	t.Run("pending_accepted", func(t *testing.T) {
		// First, record a successful bash command as evidence
		ledger2 := evidence.NewLedger()
		ledger2.SetStrictVerification(true) // V10.8: 严格验证模式
		ledger2.Record(evidence.Receipt{
			ToolName: "bash", Success: true, Command: "echo done",
		})
		ledger2.Record(evidence.Receipt{
			ToolName: "todo_write", Success: true,
			Todos: []evidence.TodoItem{
				{Content: "Add parser", Status: "in_progress"},
				{Content: "Document parser", Status: "pending"},
			},
		})
		ctx2 := evidence.WithLedger(context.Background(), ledger2)
		_, err := completeStep{}.Execute(ctx2, json.RawMessage(`{
			"step":"Document parser",
			"result":"step is complete",
			"evidence":[{"kind":"verification","summary":"ok","command":"echo done"}]}`))
		if err != nil {
			t.Fatalf("pending step should be accepted, got: %v", err)
		}
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := completeStep{}.Execute(ctx, json.RawMessage(`{
				"step":"`+tc.step+`",
				"result":"step is complete",
				"evidence":[{"kind":"manual","summary":"checked manually"}]}`))
			if err == nil {
				t.Fatal("todo-backed mismatch should be rejected")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q missing %q", err, tc.want)
			}
		})
	}
}

func TestCompleteStepIgnoresFailedTodoReceipt(t *testing.T) {
	ledger := evidence.NewLedger()
	ledger.Record(evidence.Receipt{
		ToolName: "bash",
		Success:  true,
		Command:  "go test ./...",
	})
	ledger.Record(evidence.Receipt{
		ToolName: "todo_write",
		Success:  false,
		Todos:    []evidence.TodoItem{{Content: "Add parser", Status: "in_progress"}},
	})
	ctx := evidence.WithLedger(context.Background(), ledger)

	if _, err := (completeStep{}).Execute(ctx, json.RawMessage(`{
		"step":"Anything",
		"result":"step is complete",
		"evidence":[{"kind":"verification","summary":"tests pass","command":"go test ./..."}]}`)); err != nil {
		t.Fatalf("failed todo_write receipt should not constrain step: %v", err)
	}
}

func TestCompleteStepReadOnly(t *testing.T) {
	if !(completeStep{}).ReadOnly() {
		t.Fatal("complete_step must be ReadOnly so it stays available and needs no approval")
	}
}
// ── V10.8: 非严格模式测试 ──────────────────────────────────────────

// TestCompleteStepAcceptsUnverifiedEvidenceInNonStrictMode verifies that
// complete_step accepts evidence without host receipt matching when
// strict verification is disabled (the production default).
func TestCompleteStepAcceptsUnverifiedEvidenceInNonStrictMode(t *testing.T) {
	ledger := evidence.NewLedger()
	// strictVerify defaults to false — no SetStrictVerification call
	// ledger contains receipts that do NOT match the evidence
	ledger.Record(evidence.Receipt{ToolName: "bash", Success: false, Command: "not-this-command"})
	ctx := evidence.WithLedger(context.Background(), ledger)

	cases := []struct {
		name string
		args string
	}{
		{
			name: "verification without matching bash receipt",
			args: `{"step":"x","result":"y","evidence":[{"kind":"verification","summary":"all tests pass","command":"go test ./..."}]}`,
		},
		{
			name: "diff without matching writer receipt",
			args: `{"step":"x","result":"y","evidence":[{"kind":"diff","summary":"changed files","paths":["nonexistent.go"]}]}`,
		},
		{
			name: "files without matching read/write receipt",
			args: `{"step":"x","result":"y","evidence":[{"kind":"files","summary":"inspected files","paths":["nonexistent.go"]}]}`,
		},
		{
			name: "mix of verified and unverified kinds",
			args: `{"step":"x","result":"y","evidence":[{"kind":"verification","summary":"tests pass","command":"go test ./..."},{"kind":"diff","summary":"changed files","paths":["nonexistent.go"]}]}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := completeStep{}.Execute(ctx, json.RawMessage(tc.args))
			if err != nil {
				t.Fatalf("non-strict mode should accept unverified evidence: %v", err)
			}
			if !strings.Contains(out, "signed off") {
				t.Fatalf("ack missing 'signed off', got %q", out)
			}
		})
	}
}

// TestCompleteStepAcceptsManualOnlyInNonStrictMode verifies that non-strict
// mode allows pure manual evidence (no verification/diff/files required).
func TestCompleteStepAcceptsManualOnlyInNonStrictMode(t *testing.T) {
	ledger := evidence.NewLedger()
	ctx := evidence.WithLedger(context.Background(), ledger)

	out, err := completeStep{}.Execute(ctx, json.RawMessage(`{
		"step":"Manual check",
		"result":"visual inspection confirmed correct",
		"evidence":[{"kind":"manual","summary":"checked the output visually"}]}`))
	if err != nil {
		t.Fatalf("non-strict mode should accept manual-only evidence: %v", err)
	}
	if !strings.Contains(out, "signed off") {
		t.Fatalf("ack missing 'signed off', got %q", out)
	}
}

// TestCompleteStepSkipsTodoMatchInNonStrictMode verifies that non-strict
// mode does not require a matching todo_write item — the step can be any
// text without triggering an error.
func TestCompleteStepSkipsTodoMatchInNonStrictMode(t *testing.T) {
	ledger := evidence.NewLedger()
	// Record a todo_write with completely different items
	ledger.Record(evidence.Receipt{
		ToolName: "todo_write",
		Success:  true,
		Todos: []evidence.TodoItem{
			{Content: "Unrelated task", Status: "in_progress"},
		},
	})
	ctx := evidence.WithLedger(context.Background(), ledger)

	out, err := completeStep{}.Execute(ctx, json.RawMessage(`{
		"step":"Step not in todo list",
		"result":"step is complete",
		"evidence":[{"kind":"verification","summary":"tests pass","command":"go test ./..."}]}`))
	if err != nil {
		t.Fatalf("non-strict mode should skip todo matching: %v", err)
	}
	if !strings.Contains(out, "signed off") {
		t.Fatalf("ack missing 'signed off', got %q", out)
	}
}
