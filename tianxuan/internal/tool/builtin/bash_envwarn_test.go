package builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDotEnv 写入一个项目 .env(KEY=VALUE 简单格式)。
func writeDotEnv(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestEnvWarningPrismaMismatch: 命令涉及数据库且用户级 DATABASE_URL 与项目
// .env 不一致时,执行结果必须带预警(主动检查项,不依赖记忆兜底)。
func TestEnvWarningPrismaMismatch(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns real npx; skipped under -short")
	}
	dir := t.TempDir()
	writeDotEnv(t, dir, "DATABASE_URL=\"postgresql://proj@proj-host/db\"\n")
	t.Setenv("DATABASE_URL", "postgresql://user@user-host/db")

	b := bash{workDir: dir}
	args := argsJSON(t, map[string]any{"command": "npx prisma migrate dev"})
	out, err := b.Execute(t.Context(), args)
	// 命令本身会失败(无 npx/环境),但预警必须出现在输出中。
	_ = err
	if !strings.Contains(out, "env-warning") || !strings.Contains(out, "DATABASE_URL") {
		t.Errorf("prisma mismatch output missing warning:\n%s", out)
	}
}

// TestEnvWarningPrismaMatch: 用户级与项目 .env 一致时不预警。
func TestEnvWarningPrismaMatch(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns real npx; skipped under -short")
	}
	dir := t.TempDir()
	writeDotEnv(t, dir, "DATABASE_URL=\"postgresql://proj@proj-host/db\"\n")
	t.Setenv("DATABASE_URL", "postgresql://proj@proj-host/db")

	b := bash{workDir: dir}
	args := argsJSON(t, map[string]any{"command": "npx prisma migrate dev"})
	out, _ := b.Execute(t.Context(), args)
	if strings.Contains(out, "env-warning") {
		t.Errorf("matching DATABASE_URL should not warn:\n%s", out)
	}
}

// TestEnvWarningIrrelevantCommand: 无关命令即使有环境差异也不预警(避免噪音)。
func TestEnvWarningIrrelevantCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns real npx; skipped under -short")
	}
	dir := t.TempDir()
	writeDotEnv(t, dir, "DATABASE_URL=\"postgresql://proj@proj-host/db\"\n")
	t.Setenv("DATABASE_URL", "postgresql://user@user-host/db")

	b := bash{workDir: dir}
	args := argsJSON(t, map[string]any{"command": "go test ./..."})
	out, _ := b.Execute(t.Context(), args)
	if strings.Contains(out, "env-warning") {
		t.Errorf("irrelevant command should not warn:\n%s", out)
	}
}

// TestEnvWarningNoUserVar: 用户级未设置时不预警(无对比对象)。
func TestEnvWarningNoUserVar(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns real npx; skipped under -short")
	}
	dir := t.TempDir()
	writeDotEnv(t, dir, "DATABASE_URL=\"postgresql://proj@proj-host/db\"\n")
	t.Setenv("DATABASE_URL", "")

	b := bash{workDir: dir}
	args := argsJSON(t, map[string]any{"command": "npx prisma migrate dev"})
	out, _ := b.Execute(t.Context(), args)
	if strings.Contains(out, "env-warning") {
		t.Errorf("no user-level DATABASE_URL should not warn:\n%s", out)
	}
}

// TestEnvWarningJsonMode: JSON 输出模式下预警进入 warning 字段。
func TestEnvWarningJsonMode(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns real npx; skipped under -short")
	}
	dir := t.TempDir()
	writeDotEnv(t, dir, "DATABASE_URL=\"postgresql://proj@proj-host/db\"\n")
	t.Setenv("DATABASE_URL", "postgresql://user@user-host/db")

	b := bash{workDir: dir}
	args := argsJSON(t, map[string]any{"command": "npx prisma migrate dev", "output_format": "json"})
	out, _ := b.Execute(t.Context(), args)
	if !strings.Contains(out, "warning") || !strings.Contains(out, "DATABASE_URL") {
		t.Errorf("json output missing warning field:\n%s", out)
	}
}
