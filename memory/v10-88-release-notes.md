---
name: v10-88-release-notes
title: V10.88.0 发布记录
description: V10.88.0 发布记录 — 双模型 Loop 工作流深度优化 + 三重门恢复，13 项改进
---

## V10.88.0 发布记录

| 项目 | 值 |
|------|-----|
| 版本号 | V10.88.0 |
| 发布日期 | 2026-07-15 |
| 构建产物 | `release/v10.88.0/tianxuan-desktop.exe` |
| 文件大小 | 17,511,424 bytes (~16.7 MB) |
| SHA256 | `0c9033dffdf6f9b9a1bf4ae4bcaa3482a3c3a80388b35285be225a43976ebcb8` |
| 代码变更 | 7 文件, +379/-80 行 |

### 核心变更

#### 三重门恢复（solo 模式）
- **taskGate**：检查 canonical todo 列表，未完成项注入中文 nudge（重入上限 3）
- **goalGate**：检查 SetGoal 设置的目标，要求模型自检是否达成（重入上限 3）
- **verifyGate**：保持不变（运行测试验证）
- 三闸门依次触发：taskGate → goalGate → verifyGate，plannerMode 下全部跳过
- 新增 `taskGateReentry int` 和 `goalGateReentry int` 字段到 AgentRunner

#### planFix 深度反思
- 新增 `fixAttempt` 结构体，记录每轮修正的计划和执行反馈
- Round 2：保持最小修补策略（"仅修复 ❌ 步骤"）
- Round 3：切换为反思模式——prompt 包含完整修正履历，指令"前两轮针对性修补均未完全解决。请重新审视整体方案"
- `executePlanWithRetry` 维护 `fixHistory` 切片，传递给 `planFix`

#### TurnResultEvent 去重
- `executePlan` 新增 `suppressResultEvent bool` 参数
- retry 循环中 Round 1 正常 emit，Round 2/3 抑制
- `executePlanWithRetry` 循环结束后统一 emit 一次最终结果
- 前端不再看到多张重复结果卡片

#### formatSummary 改名
- `synthesizeFinalOutput` → `formatSummary`
- 注释更新为"纯字符串格式化——不调用 LLM，不消耗 token"
- 消除"军师总结工匠"的命名误导

#### P0 Bug 修复
- **ResetSession 项目图谱丢失**：末尾添加 `h.lastProjectHash = ""`，确保新会话首次 injectProjectMap 重新注入
- **CompactNow 错误静默**：`_ = CompactNow(...)` 改为接收返回值 + `slog.Warn`

#### 安全防御
- **fast path 空输入防御**：`runFastPath` 中 task 为空时返回 nil/nil，走正常规划路径（不影响 `shouldSkipPlanner` 测试）
- **wrapExecutorSink 嵌套防护**：新增 `executorSinkWrapped bool` 字段，重复调用返回空 restore

#### 细节改进
- **shouldAutoConfirm 英文支持**：`isStepLine` 辅助函数同时匹配中文"步骤 N"和英文"Step N"
- **Tool error 截断计数**：`turnToolErrorsTruncated` 计数器，超 5 个时第 5 位置追加"（还有 N 个额外错误被截断）"
- **planStream 纯流 compaction**：plannerAgent==nil 时消息 >200 保留系统提示 + 最近 80 条
- **executePlanWithRetry 诊断日志**：修正成功/3 轮耗尽/退出无解 三处 slog.Info
- **executePlan execErr 追加**：execResult 和 execErr 同时非空时错误不丢失
- **project map 自动恢复**：planStream compaction 后重置 lastProjectHash
- **V10.XX→V10.87**：全部 10 处版本号占位符替换

### 架构约束

| 约束 | 说明 |
|------|------|
| 缓存前缀不变性 | 所有改动不改变双模型各自的 session prefix |
| 三重门仅 solo | plannerMode 下三闸门全部跳过，Hermes 自行管理任务追踪 |
| TurnResultEvent 单次 | retry 循环前端只收到一张结果卡片 |

### 构建信息

```
构建命令: cd tianxuan/desktop && wails build
产物位置: tianxuan/desktop/build/bin/tianxuan-desktop.exe
Wails CLI: v2.13.0
构建时间: 49.8s
```
