package builtin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"tianxuan/internal/event"
	"tianxuan/internal/jobs"
	"tianxuan/internal/sandbox"
)

// nodePath 定位 node 可执行文件：PATH 优先，其次项目自带工具链
// (<repo>/tools/node/node.exe，测试 cwd 为 internal/tool/builtin)。
func nodePath() string {
	for _, n := range []string{"node", "node.exe"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	rel := filepath.Join("..", "..", "..", "..", "tools", "node", "node.exe")
	if abs, err := filepath.Abs(rel); err == nil {
		if _, statErr := os.Stat(abs); statErr == nil {
			return filepath.ToSlash(abs)
		}
	}
	return ""
}

func newTestJobManager() *jobs.Manager { return jobs.NewManager(event.Discard) }

func withTestJobs(jm *jobs.Manager) context.Context {
	return jobs.WithManager(context.Background(), jm)
}

func resolvedTestShell(t *testing.T) sandbox.Shell {
	t.Helper()
	return (bash{}).resolved()
}

var bashJobIDRe = regexp.MustCompile(`job "([^"]+)"`)

func extractJobID(t *testing.T, out string) string {
	t.Helper()
	m := bashJobIDRe.FindStringSubmatch(out)
	if len(m) != 2 {
		t.Fatalf("no job id in output %q", out)
	}
	return m[1]
}

// TestIsServiceCommand locks the service-command detector used to auto-background
// long-running servers: they must never block a foreground turn (which can deadlock
// the whole process when the server keeps the output pipe open past the timeout).
func TestIsServiceCommand(t *testing.T) {
	services := []string{
		"npm run dev",
		"npm start",
		"pnpm dev",
		"yarn start",
		"python -m http.server 8000",
		"uvicorn app:app --reload",
		"docker compose up",
		"node server.js",
		"go run ./cmd/server",
		"tail -f app.log",
		"ngrok http 3000",
		"cargo run",
	}
	for _, c := range services {
		if !isServiceCommand(c) {
			t.Errorf("isServiceCommand(%q) = false, want true", c)
		}
	}
	normal := []string{
		"echo hello",
		"go test ./...",
		"git status",
		"ls -la",
		"grep -r foo .",
		"rm -f tmp.txt",
		"npm install",
		"python script.py --once",
	}
	for _, c := range normal {
		if isServiceCommand(c) {
			t.Errorf("isServiceCommand(%q) = true, want false", c)
		}
	}
}

// TestBashServiceCommandAutoBackground: 服务类命令未设 run_in_background 时
// 自动转后台——Execute 立即返回 job 信息（不阻塞 turn），且可用 kill_shell 停止。
func TestBashServiceCommandAutoBackground(t *testing.T) {
	nodeBin := nodePath()
	if nodeBin == "" {
		t.Skip("node not available")
	}
	jm := newTestJobManager()
	ctx := withTestJobs(jm)
	sh := resolvedTestShell(t)
	b := bash{shell: sh}
	// " server" 命中 isServiceCommand：node 服务命令本身没有自然关键词，
	// 注释里注明用途（createServer 的 Server 前无空格，不匹配 " server"）。
	nodeCmd := fmt.Sprintf(`"%s" -e "/* server */ require('http').createServer((q,s)=>s.end()).listen(0)"`, nodeBin)
	if sh.Kind == sandbox.ShellPowerShell {
		nodeCmd = fmt.Sprintf(`& %s`, nodeCmd) // PowerShell 需要 & 调用运算符
	}
	out, err := b.Execute(ctx, argsJSON(t, map[string]any{
		"command": nodeCmd,
	}))
	if err != nil {
		t.Fatalf("service command should auto-background without error: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "Started background job") {
		t.Fatalf("expected background job notice, got %q", out)
	}
	// 清理后台 job（kill 进程树），避免测试残留 node 进程。
	id := extractJobID(t, out)
	if !jm.Kill(id) {
		t.Errorf("kill job %q failed", id)
	}
}
