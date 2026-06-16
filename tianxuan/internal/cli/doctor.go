package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"tianxuan/internal/config"
	"tianxuan/internal/doctor"
)

func doctorCommand(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "输出 JSON 格式")
	fs.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, ansiRed+"✖ 无法加载配置: "+err.Error()+ansiReset)
		return 1
	}

	modelName := cfg.DefaultModel
	apiKey := ""
	balanceURL := ""
	if pe, ok := cfg.ResolveModel(modelName); ok {
		if pe.APIKeyEnv != "" {
			apiKey = os.Getenv(pe.APIKeyEnv)
		}
		if strings.HasPrefix(pe.BaseURL, "http") && !strings.Contains(pe.BaseURL, "openai.com") {
			balanceURL = strings.TrimRight(pe.BaseURL, "/") + "/user/balance"
		}
	}

	mcpPlugins := make([]string, 0, len(cfg.Plugins))
	for _, p := range cfg.Plugins {
		mcpPlugins = append(mcpPlugins, p.Name)
	}

	cwd, _ := os.Getwd()
	_, goModErr := os.Stat(cwd + string(os.PathSeparator) + "go.mod")
	goModExists := goModErr == nil

	ctx := context.Background()
	report := doctor.Run(ctx, doctor.Deps{
		Cfg:          cfg,
		ProviderName: modelName,
		APIKey:       apiKey,
		BalanceURL:   balanceURL,
		MCPPlugins:   mcpPlugins,
		CWD:          cwd,
		GoModExists:  goModExists,
	})

	if *jsonOut {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		if report.Failed > 0 {
			return 1
		}
		return 0
	}

	fmt.Println(ansiBold + "🔍 tianxuan doctor" + ansiReset)
	fmt.Println()

	for _, it := range report.Items {
		icon, color := statusIcon(it.Status)
		fmt.Printf("  %s %-6s %s\n", color+icon, it.Name, it.Detail+ansiReset)
		if it.Advice != "" {
			fmt.Printf("     %s %s\n", ansiDim+"→", it.Advice+ansiReset)
		}
	}
	fmt.Println()
	fmt.Printf("  %s\n", ansiBold+report.Summary()+ansiReset)

	if report.Failed > 0 {
		return 1
	}
	return 0
}

func statusIcon(s doctor.Status) (string, string) {
	switch s {
	case doctor.Pass:
		return "[✓]", ansiGreen
	case doctor.Warn:
		return "[!]", ansiYellow
	case doctor.Fail:
		return "[✖]", ansiRed
	default:
		return "[ ]", ansiDim
	}
}
