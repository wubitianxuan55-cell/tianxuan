package builtin

import (
	"context"
	"strings"
	"testing"

	"tianxuan/internal/sandbox"
)

// TestPlainModeFailureHeader verifies the V10.169 change: a failed foreground
// command (non-zero exit) in plain mode returns a codex-style structured
// header — "Exit code: N", "Wall time: X.X seconds", and the output under an
// "Output:" line — so the model can read the exit status without parsing the
// raw error string. Success stays raw (no header) to preserve downstream
// parsing.
func TestPlainModeFailureHeader(t *testing.T) {
	sh := resolvedTestShell(t)
	b := bash{shell: sh}

	// "false" always exits 1 and prints nothing.
	out, err := b.Execute(context.Background(), argsJSON(t, map[string]any{
		"command": "false",
	}))
	if err == nil {
		t.Fatal("expected non-zero exit error from `false`, got nil")
	}
	if !strings.Contains(out, "Exit code: 1") {
		t.Fatalf("output = %q, want it to contain 'Exit code: 1'", out)
	}
	if !strings.Contains(out, "Wall time:") {
		t.Fatalf("output = %q, want it to contain 'Wall time:'", out)
	}
	if !strings.Contains(out, "Output:") {
		t.Fatalf("output = %q, want an 'Output:' section marker", out)
	}
}

// TestPlainModeFailureWithOutputKeepsContent verifies the failure header wraps
// (not replaces) the command's own output: the model still sees stdout/stderr.
func TestPlainModeFailureWithOutputKeepsContent(t *testing.T) {
	sh := resolvedTestShell(t)
	b := bash{shell: sh}

	// Print a marker line, then exit 3.
	cmd := `printf 'boom-marker\n' && exit 3`
	if sh.Kind == sandbox.ShellPowerShell {
		cmd = `Write-Output 'boom-marker'; exit 3`
	}
	out, err := b.Execute(context.Background(), argsJSON(t, map[string]any{
		"command": cmd,
	}))
	if err == nil {
		t.Fatal("expected non-zero exit error, got nil")
	}
	if !strings.Contains(out, "Exit code: 3") {
		t.Fatalf("output = %q, want 'Exit code: 3'", out)
	}
	if !strings.Contains(out, "boom-marker") {
		t.Fatalf("output = %q, want the command's own output preserved", out)
	}
}

// TestPlainModeSuccessStaysRaw locks the no-regression contract: a successful
// plain-mode command returns exactly its merged output with no header prefix,
// so existing parsers and downstream consumers are unaffected.
func TestPlainModeSuccessStaysRaw(t *testing.T) {
	sh := resolvedTestShell(t)
	b := bash{shell: sh}

	out, err := b.Execute(context.Background(), argsJSON(t, map[string]any{
		"command": `printf 'ok-raw-output\n'`,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "Exit code:") {
		t.Fatalf("success output = %q, must NOT contain an 'Exit code:' header", out)
	}
	if !strings.Contains(out, "ok-raw-output") {
		t.Fatalf("output = %q, want the command's raw output", out)
	}
}

// TestJSONModeUnchanged verifies json output_format still returns the
// structured object and is not affected by the plain-mode header change.
// JSON mode encodes failure in the payload (ok:false, exit_code) and returns
// a nil error — that contract is unchanged.
func TestJSONModeUnchanged(t *testing.T) {
	sh := resolvedTestShell(t)
	b := bash{shell: sh}

	out, err := b.Execute(context.Background(), argsJSON(t, map[string]any{
		"command":     "false",
		"output_format": "json",
	}))
	if err != nil {
		t.Fatalf("json mode should return nil error (failure encoded in payload), got %v", err)
	}
	if !strings.Contains(out, `"exit_code":1`) && !strings.Contains(out, `"exit_code": 1`) {
		t.Fatalf("json output = %q, want exit_code field", out)
	}
	if !strings.Contains(out, `"ok":false`) {
		t.Fatalf("json output = %q, want ok:false for failing command", out)
	}
	if strings.Contains(out, "Exit code: 1") {
		t.Fatalf("json output = %q, must NOT contain plain-mode header", out)
	}
}

// TestTruncatedFailureReportsTotalLines verifies the "Total output lines" line
// appears only when the failure output was truncated (mirroring codex: report
// the count only when the model can't see every line). A 1KB+ failing command
// output triggers plain-mode truncation (48KB cap is generous, so we emit
// enough lines to exceed it and assert the line appears).
func TestTruncatedFailureReportsTotalLines(t *testing.T) {
	sh := resolvedTestShell(t)
	b := bash{shell: sh}

	// 3000 行 × ~40 字节 ≈ 120KB > 48KB cap → truncation fires, while the
	// command line itself stays tiny (avoids OS command-length limits).
	cmd := `seq 1 3000 | awk '{print $1 " xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}' && exit 9`
	if sh.Kind == sandbox.ShellPowerShell {
		cmd = `1..3000 | ForEach-Object { "$_ xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" }; exit 9`
	}
	out, err := b.Execute(context.Background(), argsJSON(t, map[string]any{
		"command": cmd,
	}))
	if err == nil {
		t.Fatal("expected non-zero exit error, got nil")
	}
	if !strings.Contains(out, "Exit code: 9") {
		t.Fatalf("output = %q, want 'Exit code: 9'", out)
	}
	if !strings.Contains(out, "Total output lines:") {
		t.Fatalf("output = %q, want 'Total output lines:' (output was truncated)", out)
	}
}

// TestSmallFailureOmitsTotalLines locks the no-noise contract: a small failing
// output (no truncation) must NOT carry a "Total output lines" line — the
// model sees every line, so the count is redundant.
func TestSmallFailureOmitsTotalLines(t *testing.T) {
	sh := resolvedTestShell(t)
	b := bash{shell: sh}

	out, err := b.Execute(context.Background(), argsJSON(t, map[string]any{
		"command": "printf 'tiny\\n' && exit 2",
	}))
	if err == nil {
		t.Fatal("expected non-zero exit error, got nil")
	}
	if strings.Contains(out, "Total output lines:") {
		t.Fatalf("output = %q, must NOT contain 'Total output lines:' for small output", out)
	}
	if !strings.Contains(out, "Exit code: 2") {
		t.Fatalf("output = %q, want 'Exit code: 2'", out)
	}
}
