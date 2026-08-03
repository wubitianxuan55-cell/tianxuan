package builtin

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestAdaptPowerShellHeredocCatWriteFile：`cat <<EOF > file` 必须翻译为
// PowerShell 的 WriteAllText（here-string），而不是原样丢给 PowerShell 报错。
func TestAdaptPowerShellHeredocCatWriteFile(t *testing.T) {
	cmd := "cat <<'EOF' > out.txt\nline one\nline two\nEOF"
	got, err := adaptPowerShellCommand(cmd, "")
	if err != nil {
		t.Fatalf("adapt: %v", err)
	}
	for _, want := range []string{"WriteAllText", "out.txt", "line one", "line two"} {
		if !strings.Contains(got, want) {
			t.Errorf("adapted command %q should contain %q", got, want)
		}
	}
	if strings.Contains(got, "<<") {
		t.Errorf("heredoc not translated: %q", got)
	}
}

// TestAdaptPowerShellHeredocAppend：`cat <<EOF >> file` 翻译为 AppendAllText。
func TestAdaptPowerShellHeredocAppend(t *testing.T) {
	cmd := "cat <<EOF >> log.txt\nmore\nEOF"
	got, err := adaptPowerShellCommand(cmd, "")
	if err != nil {
		t.Fatalf("adapt: %v", err)
	}
	if !strings.Contains(got, "AppendAllText") || !strings.Contains(got, "log.txt") {
		t.Errorf("adapted command %q should use AppendAllText on log.txt", got)
	}
}

// TestAdaptPowerShellHeredocStdin：`python - <<EOF` 翻译为 here-string 管道 stdin。
func TestAdaptPowerShellHeredocStdin(t *testing.T) {
	cmd := "python - <<EOF\nprint(1)\nEOF"
	got, err := adaptPowerShellCommand(cmd, "")
	if err != nil {
		t.Fatalf("adapt: %v", err)
	}
	if !strings.Contains(got, "| python -") || !strings.Contains(got, "print(1)") {
		t.Errorf("adapted command %q should pipe the heredoc to python -", got)
	}
	if strings.Contains(got, "<<") {
		t.Errorf("heredoc not translated: %q", got)
	}
}

// TestAdaptPowerShellHeredocNodeStdin：node 场景同上。
func TestAdaptPowerShellHeredocNodeStdin(t *testing.T) {
	cmd := "node - <<EOF\nconsole.log(1)\nEOF"
	got, err := adaptPowerShellCommand(cmd, "")
	if err != nil {
		t.Fatalf("adapt: %v", err)
	}
	if !strings.Contains(got, "| node -") || !strings.Contains(got, "console.log(1)") {
		t.Errorf("adapted command %q should pipe the heredoc to node -", got)
	}
}

// TestAdaptPowerShellHeredocCatNoRedirect：`cat <<EOF` 直接输出 here-string。
func TestAdaptPowerShellHeredocCatNoRedirect(t *testing.T) {
	cmd := "cat <<EOF\nhello\nEOF"
	got, err := adaptPowerShellCommand(cmd, "")
	if err != nil {
		t.Fatalf("adapt: %v", err)
	}
	if !strings.Contains(got, "hello") || strings.Contains(got, "<<") {
		t.Errorf("adapted command %q should carry the heredoc body", got)
	}
}

// TestAdaptPowerShellHeredocUnsupported：无法翻译的 heredoc 形式要大声失败，
// 不能静默地把 << 丢给 PowerShell。
func TestAdaptPowerShellHeredocUnsupported(t *testing.T) {
	cmd := "diff a b <<EOF\nx\nEOF"
	if _, err := adaptPowerShellCommand(cmd, ""); err == nil {
		t.Fatal("unsupported heredoc command should fail loudly")
	}
}

// TestAdaptPowerShellNoHeredocPassthrough：无 heredoc 的普通命令原样返回。
func TestAdaptPowerShellNoHeredocPassthrough(t *testing.T) {
	cmd := "Write-Output 'x'"
	got, err := adaptPowerShellCommand(cmd, "")
	if err != nil {
		t.Fatalf("adapt: %v", err)
	}
	if got != cmd {
		t.Errorf("plain command changed: got %q want %q", got, cmd)
	}
}

// TestAdaptPowerShellNpxShims：裸 npm/npx/pnpm/pnpx 在命令位置必须加 .cmd
// 后缀，绕开 PowerShell 优先解析 .ps1 导致的执行策略拦截；参数位置不动。
func TestAdaptPowerShellNpxShims(t *testing.T) {
	cases := []struct{ in, want string }{
		{"npx tsc --noEmit", "npx.cmd tsc --noEmit"},
		{"npm install", "npm.cmd install"},
		{"; pnpm add x", "; pnpm.cmd add x"},
		{"echo npm", "echo npm"}, // 参数位置不替换
		{"Write-Output 'npx'", "Write-Output 'npx'"},
	}
	for _, c := range cases {
		got, err := adaptPowerShellCommand(c.in, "")
		if err != nil {
			t.Fatalf("adapt %q: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("adapt %q = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestAdaptPowerShellGitInjection：git 不在 PATH 时注入常见 git cmd 目录。
func TestAdaptPowerShellGitInjection(t *testing.T) {
	gitDir := `C:\Program Files\Git\cmd`
	got, err := adaptPowerShellCommand("git status", gitDir)
	if err != nil {
		t.Fatalf("adapt: %v", err)
	}
	// gitCmdDir 被注入为 "dir;" 前缀（PATH 分隔符在引号内是合法 PowerShell）。
	if !strings.Contains(got, "$env:Path='"+gitDir+";'") {
		t.Errorf("adapted command %q should prepend $env:Path with %q", got, gitDir)
	}
	if !strings.Contains(got, "git status") {
		t.Errorf("adapted command %q should keep the original command", got)
	}
}

// TestAdaptPowerShellHeredocThenShims：heredoc 转换后再做 .cmd 后缀替换。
func TestAdaptPowerShellHeredocThenShims(t *testing.T) {
	cmd := "npx tsx - <<EOF\nconsole.log('x')\nEOF"
	got, err := adaptPowerShellCommand(cmd, "")
	if err != nil {
		t.Fatalf("adapt: %v", err)
	}
	if !strings.Contains(got, "| npx.cmd tsx -") {
		t.Errorf("adapted command %q should shim npx after heredoc translation", got)
	}
}

// TestBashPowerShellHeredocWritesFile e2e：翻译后的 heredoc 写文件在真实
// PowerShell 下工作（Windows only）。
func TestBashPowerShellHeredocWritesFile(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("powershell e2e is windows-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	cmd := "cat <<'EOF' > " + path + "\nhello 世界\nEOF"
	if _, err := runPS(t, cmd); err != nil {
		t.Fatalf("heredoc write via powershell: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(b), "hello 世界") {
		t.Errorf("file content = %q, want it to contain heredoc body", string(b))
	}
}
