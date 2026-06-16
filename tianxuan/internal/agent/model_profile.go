package agent

// ─── V5.17: ModelContextProfile (Kun model-context-profile.ts 移植) ──────

// ModelProfile 定义不同模型的上下文窗口和压缩阈值。
type ModelProfile struct {
	Model        string  // 模型名称（如 "deepseek-v4-flash"）
	ContextWindow int    // 上下文窗口 token 数
	SoftRatio    float64 // soft 阈值比例（默认 0.8）
}

// DefaultModelProfiles 返回内置模型配置。
// 数值来自 DeepSeek 官方文档和 Kun 实测。
func DefaultModelProfiles() []ModelProfile {
	return []ModelProfile{
		{Model: "deepseek-v4-flash", ContextWindow: 128_000, SoftRatio: 0.8},
		{Model: "deepseek-v4-pro", ContextWindow: 1_000_000, SoftRatio: 0.8},
		{Model: "deepseek-v3", ContextWindow: 128_000, SoftRatio: 0.8},
		{Model: "deepseek-r1", ContextWindow: 128_000, SoftRatio: 0.8},
		{Model: "deepseek-chat", ContextWindow: 128_000, SoftRatio: 0.8},
		{Model: "deepseek-reasoner", ContextWindow: 128_000, SoftRatio: 0.8},
	}
}

// LookupModelProfile 根据模型名称查找配置。
// 返回 nil 表示未找到，使用默认 CompactionConfig。
func LookupModelProfile(model string, profiles []ModelProfile) *ModelProfile {
	for i := range profiles {
		if profiles[i].Model == model {
			return &profiles[i]
		}
	}
	return nil
}

// ApplyModelProfile 将模型配置应用到 CompactionConfig。
// 仅当 profile 的 ContextWindow > 0 时才覆盖 Window。
// SoftRatio > 0 时覆盖 Ratio。
func ApplyModelProfile(comp *CompactionConfig, profile *ModelProfile) {
	if profile == nil {
		return
	}
	if profile.ContextWindow > 0 {
		comp.Window = profile.ContextWindow
	}
	if profile.SoftRatio > 0 {
		comp.Ratio = profile.SoftRatio
	}
}
