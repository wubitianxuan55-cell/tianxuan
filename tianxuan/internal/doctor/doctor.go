// Package doctor provides a diagnostic engine that checks tianxuan's health:
// config validity, API connectivity, cache stability, environment, MCP status,
// token usage, Go toolchain, and project structure. It powers both the
// `tianxuan doctor` CLI command and the `doctor` model-facing tool.
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"tianxuan/internal/billing"
	"tianxuan/internal/config"
)

// Status is a check outcome.
type Status string

const (
	Pass Status = "pass"
	Warn Status = "warn"
	Fail Status = "fail"
	Skip Status = "skip"
)

// Item is one diagnostic check result.
type Item struct {
	Name    string `json:"name"`
	Status  Status `json:"status"`
	Detail  string `json:"detail"`
	Advice  string `json:"advice,omitempty"`
}

// Report is the full diagnostic output.
type Report struct {
	Items   []Item `json:"items"`
	Passed  int    `json:"passed"`
	Warned  int    `json:"warned"`
	Failed  int    `json:"failed"`
	Skipped int    `json:"skipped"`
}

// Deps are the external dependencies the checker needs. All are optional —
// a nil value skips the corresponding check.
type Deps struct {
	Cfg        *config.Config
	ProviderName string // e.g. "deepseek-v4-pro"
	APIKey       string // bearer token for connectivity check
	BalanceURL   string // e.g. "https://api.deepseek.com/user/balance"

	// Cache-related callbacks (from agent if available).
	CachePrefixLocked  func() bool   // verifyPrefix has locked
	CacheVolatileFound func() string // volatile token scan result ("" = clean)
	ToolDrift          func() string // tool catalog drift ("" = stable)

	// MCP plugin list.
	MCPPlugins []string

	// Token stats for historical sessions.
	TokenStats func() (hitTokens, missTokens int64, ok bool)

	CWD         string
	GoModExists bool   // go.mod found in CWD?
}

// Run executes all checks and returns a report.
func Run(ctx context.Context, deps Deps) *Report {
	r := &Report{}

	r.add(checkConfig(deps))
	r.add(checkConnectivity(ctx, deps))
	r.add(checkCache(deps))
	r.add(checkEnvironment(deps))
	r.add(checkMCP(deps))
	r.add(checkTokens(deps))
	r.add(checkGoEnv(ctx))
	r.add(checkProject(deps))

	for _, it := range r.Items {
		switch it.Status {
		case Pass:
			r.Passed++
		case Warn:
			r.Warned++
		case Fail:
			r.Failed++
		case Skip:
			r.Skipped++
		}
	}
	return r
}

func (r *Report) add(it Item) { r.Items = append(r.Items, it) }

// Summary returns a one-line verdict.
func (r *Report) Summary() string {
	if r.Failed > 0 {
		return fmt.Sprintf("%d/%d 通过, %d 失败, %d 警告", r.Passed, r.Passed+r.Warned+r.Failed, r.Failed, r.Warned)
	}
	if r.Warned > 0 {
		return fmt.Sprintf("%d/%d 通过, %d 警告", r.Passed, r.Passed+r.Warned, r.Warned)
	}
	return fmt.Sprintf("全部 %d 项通过", r.Passed)
}

// ─── individual checks ──────────────────────────────────────────

func checkConfig(deps Deps) Item {
	if deps.Cfg == nil {
		return Item{Name: "配置", Status: Skip, Detail: "配置未加载"}
	}
	if deps.ProviderName == "" {
		deps.ProviderName = deps.Cfg.DefaultModel
	}
	if deps.ProviderName == "" {
		return Item{Name: "配置", Status: Fail, Detail: "未设置默认模型", Advice: "运行 tianxuan setup 配置模型"}
	}
	if err := deps.Cfg.Validate(deps.ProviderName); err != nil {
		return Item{Name: "配置", Status: Fail, Detail: err.Error(), Advice: "检查 config.toml 中的 providers 配置"}
	}
	// Resolve to show the actual provider
	pe, ok := deps.Cfg.ResolveModel(deps.ProviderName)
	if !ok {
		return Item{Name: "配置", Status: Warn, Detail: fmt.Sprintf("模型 %s 已配置", deps.ProviderName), Advice: "验证 base URL 是否正确"}
	}
	detail := fmt.Sprintf("模型 %s · 类型 %s · key=%s · %s", deps.ProviderName, pe.Kind, maskKeyEnv(pe.APIKeyEnv), pe.BaseURL)
	return Item{Name: "配置", Status: Pass, Detail: detail}
}

func checkConnectivity(ctx context.Context, deps Deps) Item {
	if deps.APIKey == "" || deps.BalanceURL == "" {
		return Item{Name: "连通性", Status: Skip, Detail: "API key 或余额 URL 未配置"}
	}
	bal, err := billing.Fetch(ctx, deps.BalanceURL, deps.APIKey)
	if err != nil {
		return Item{Name: "连通性", Status: Fail, Detail: fmt.Sprintf("API 不可达: %v", err), Advice: "检查网络连接和 API key 是否有效"}
	}
	if bal == nil {
		return Item{Name: "连通性", Status: Skip, Detail: "余额查询不支持"}
	}
	balanceStr := "未知"
	if len(bal.Infos) > 0 {
		balanceStr = bal.Infos[0].TotalBalance
		if bal.Infos[0].GrantedBalance != "" && bal.Infos[0].GrantedBalance != "0" {
			balanceStr = bal.Infos[0].GrantedBalance
		}
	}
	return Item{Name: "连通性", Status: Pass, Detail: fmt.Sprintf("API 可达 · 余额 ¥%s", balanceStr)}
}

func checkCache(deps Deps) Item {
	if deps.CachePrefixLocked == nil {
		return Item{Name: "缓存", Status: Skip, Detail: "无缓存状态（非交互模式）"}
	}
	if !deps.CachePrefixLocked() {
		return Item{Name: "缓存", Status: Warn, Detail: "前缀未锁定（首轮前）"}
	}
	var issues []string
	if deps.CacheVolatileFound != nil {
		if v := deps.CacheVolatileFound(); v != "" {
			issues = append(issues, "检测到挥发性 token: "+v)
		}
	}
	if deps.ToolDrift != nil {
		if d := deps.ToolDrift(); d != "" {
			issues = append(issues, "工具目录漂移: "+d)
		}
	}
	if len(issues) > 0 {
		return Item{Name: "缓存", Status: Warn, Detail: strings.Join(issues, "; "), Advice: "重启会话以重建缓存"}
	}
	return Item{Name: "缓存", Status: Pass, Detail: "前缀已锁定 · 无挥发性 token · 工具目录稳定"}
}

func checkEnvironment(deps Deps) Item {
	var parts []string
	shell := detectShell()
	parts = append(parts, "shell="+shell)

	// Check for sandbox (macOS only — check if sandbox-exec exists)
	if _, err := exec.LookPath("sandbox-exec"); err == nil {
		parts = append(parts, "sandbox可用")
	}

	if deps.CWD != "" {
		parts = append(parts, "cwd="+deps.CWD)
	}
	return Item{Name: "环境", Status: Pass, Detail: strings.Join(parts, " · ")}
}

func checkMCP(deps Deps) Item {
	if len(deps.MCPPlugins) == 0 {
		return Item{Name: "MCP", Status: Skip, Detail: "无插件配置"}
	}
	return Item{Name: "MCP", Status: Pass, Detail: fmt.Sprintf("%d 插件: %s", len(deps.MCPPlugins), strings.Join(deps.MCPPlugins, ", "))}
}

func checkTokens(deps Deps) Item {
	if deps.TokenStats == nil {
		return Item{Name: "Token", Status: Skip, Detail: "无统计数据"}
	}
	hit, miss, ok := deps.TokenStats()
	if !ok {
		return Item{Name: "Token", Status: Skip, Detail: "无历史会话"}
	}
	total := hit + miss
	rate := 0.0
	if total > 0 {
		rate = float64(hit) / float64(total) * 100
	}
	return Item{Name: "Token", Status: Pass, Detail: fmt.Sprintf("累计 %d prompt (命中 %.1f%%)", total, rate)}
}

func checkGoEnv(ctx context.Context) Item {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return Item{Name: "Go", Status: Skip, Detail: "go 未安装"}
	}
	ver := goVersion(ctx, goBin)
	goroot := os.Getenv("GOROOT")
	if goroot == "" {
		goroot = filepath.Dir(filepath.Dir(goBin)) // infer from go binary path
	}
	detail := fmt.Sprintf("%s · %s", ver, goroot)
	st := Pass
	if ver == "" {
		st = Warn
		detail = "版本未知"
	}
	return Item{Name: "Go", Status: st, Detail: detail}
}

func checkProject(deps Deps) Item {
	if deps.CWD == "" {
		return Item{Name: "项目", Status: Skip, Detail: "工作目录未知"}
	}
	var parts []string
	if deps.GoModExists {
		mod := readGoModule(deps.CWD)
		if mod != "" {
			parts = append(parts, "module="+mod)
		}
	}
	// Count Go files in top-level
	entries, err := os.ReadDir(deps.CWD)
	if err == nil {
		dirs := 0
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				dirs++
			}
		}
		if dirs > 0 {
			parts = append(parts, fmt.Sprintf("%d 顶级目录", dirs))
		}
	}
	if len(parts) == 0 {
		return Item{Name: "项目", Status: Pass, Detail: "工作目录: " + deps.CWD}
	}
	return Item{Name: "项目", Status: Pass, Detail: strings.Join(parts, " · ")}
}

// ─── helpers ─────────────────────────────────────────────────────

func maskKeyEnv(env string) string {
	if env == "" {
		return "未设"
	}
	if _, ok := os.LookupEnv(env); ok {
		return "已设(" + env + ")"
	}
	return "未设(" + env + ")"
}

func detectShell() string {
	// On Windows, prefer pwsh then powershell
	if _, err := exec.LookPath("pwsh"); err == nil {
		return "pwsh"
	}
	if _, err := exec.LookPath("powershell"); err == nil {
		return "powershell"
	}
	// Unix: prefer bash
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash"
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return filepath.Base(sh)
	}
	return "unknown"
}

func goVersion(ctx context.Context, goBin string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, goBin, "version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// "go version go1.23.4 windows/amd64" → "go1.23.4"
	fields := strings.Fields(string(out))
	if len(fields) >= 3 {
		return fields[2]
	}
	return strings.TrimSpace(string(out))
}

func readGoModule(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimPrefix(line, "module ")
		}
	}
	return ""
}
