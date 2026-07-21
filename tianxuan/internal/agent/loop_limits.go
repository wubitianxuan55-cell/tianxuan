// Package agent
// ── 循环控制常量 ──────────────────────────────────────────────────────────
//
// 所有 agent loop 的硬编码阈值集中在此文件，方便全局审计和调优。
// 分为三类：
//  1. Retry caps — 同一纠正动作的最大重试次数
//  2. Detection thresholds — 触发防护措施所需的连续出现次数
//  3. Verify gate — 单次触发（布尔守卫）
//
// 这些值经过 V5-V10 迭代验证，调整需谨慎。

package agent

// ── Retry caps ───────────────────────────────────────────────────────────

// OutputLenNudgeCap 输出截断 nudge 最大注入次数。
// 防止模型反复触及输出 token 限制却不产生工具调用。
const OutputLenNudgeCap = 5

// InvalidOutputCap 无效输出 nudge 最大注入次数。
// 防止模型仅输出 reasoning 而不产生文字或工具调用。
const InvalidOutputCap = 3

// MaxStreamRecoveries 流中断恢复最大次数。
// 连接断开时可尝试恢复流的次数上限。
const MaxStreamRecoveries = 3

// MaxEmptyFinalBlocks 空最终答案最大阻止次数。
// 模型声明完成但未输出可见文字时的重试上限。
const MaxEmptyFinalBlocks = 3

// MaxFinalReadinessBlocks 最终就绪检查最大阻止次数。
// 模型声称完成但缺少 complete_step 证据时的重试上限。
const MaxFinalReadinessBlocks = 3

// TaskGateReentryLimit 任务门重入次数上限。
// taskGate 最多注入几次未完成任务提醒。
const TaskGateReentryLimit = 3

// GoalGateReentryLimit 目标门重入次数上限。
// goalGate 最多注入几次目标检查提醒。
const GoalGateReentryLimit = 3

// ── Detection thresholds ──────────────────────────────────────────────────

// StormBreakThreshold 同工具同错误连续失败触发风暴断路器的次数。
const StormBreakThreshold = 3

// RepeatedStepThreshold 连续相同步骤触发重复检测的次数。
const RepeatedStepThreshold = 3

// RepeatSuccessAllowed 同写工具允许连续成功的次数（不含）。
// 设为 2 表示第 3 次同参数成功调用将被阻止。
const RepeatSuccessAllowed = 2

// BgStartKillStreakThreshold 后台启停循环检测阈值。
// 连续启动→立即杀死后台任务且不读取输出的次数。
const BgStartKillStreakThreshold = 3
