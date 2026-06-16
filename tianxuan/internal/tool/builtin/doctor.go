package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"tianxuan/internal/doctor"
	"tianxuan/internal/tool"
)

func init() { tool.RegisterBuiltin(doctorTool{}) }

type doctorTool struct{}

func (doctorTool) Name() string { return "doctor" }

func (doctorTool) Description() string {
	return "运行系统诊断：检查 Go 环境、项目结构、操作系统等。返回结构化报告。在排查环境问题或确认工具链可用性时使用。（完整的配置/API/缓存检查请使用 CLI 命令 `tianxuan doctor`）"
}

func (doctorTool) Schema() json.RawMessage {
	return json.RawMessage(`{
"type":"object",
"properties":{},
"required":[]
}`)
}

func (doctorTool) ReadOnly() bool { return true }

func (doctorTool) CompactDescription() string { return compactDesc["doctor"] }
func (doctorTool) CompactSchema() json.RawMessage   { return compactSchema["doctor"] }

func (doctorTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	cwd, _ := os.Getwd()
	_, goModErr := os.Stat(cwd + string(os.PathSeparator) + "go.mod")
	goModExists := goModErr == nil

	deps := doctor.Deps{
		CWD:        cwd,
		GoModExists: goModExists,
	}

	report := doctor.Run(ctx, deps)

	var out strings.Builder
	out.WriteString("doctor:\n")
	for _, it := range report.Items {
		if it.Status == doctor.Skip {
			continue // 跳过未配置的检查项
		}
		icon := ""
		switch it.Status {
		case doctor.Pass:
			icon = "PASS"
		case doctor.Warn:
			icon = "WARN"
		case doctor.Fail:
			icon = "FAIL"
		default:
			continue
		}
		fmt.Fprintf(&out, "[%s] %s: %s\n", icon, it.Name, it.Detail)
		if it.Advice != "" {
			fmt.Fprintf(&out, "      → %s\n", it.Advice)
		}
	}
	fmt.Fprintf(&out, "\nSUMMARY: %s\n", report.Summary())
	return out.String(), nil
}
