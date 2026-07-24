package agent

import (
	"context"

	"tianxuan/internal/planmode"
	"tianxuan/internal/provider"
)

// Planner 是规划者的抽象接口。
// DeepSeek 规划者使用 AgentRunner（含 TCCA 四域缓存），
// XAI 规划者使用 XAIPlanner（独立上下文管理）。
// Hermes 通过此接口统一编排规划→执行流程。
type Planner interface {
	// Run 执行一轮规划，返回规划结果。
	Run(ctx context.Context, input string) (*TurnResult, error)

	// SetSession 替换规划者的对话 session。
	SetSession(s *Session)

	// SetAsker 安装交互式问答器（plan 确认等）。
	SetAsker(a Asker)

	// SetPlanMode 切换只读计划模式。
	SetPlanMode(v bool)

	// SetPlanModePolicy 安装计划模式安全策略。
	SetPlanModePolicy(p planmode.Policy)

	// CompactNow 立即触发上下文压缩。
	CompactNow(ctx context.Context, reason string) error

	// LastUsage 返回最近一次 API 调用的 token 用量。
	LastUsage() *provider.Usage

	// ContextWindow 返回上下文窗口大小（tokens）。
	ContextWindow() int

	// ProvName 返回规划者使用的 provider 名称。
	ProvName() string
}
