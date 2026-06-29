---
name: v10-14-release-notes
title: V10.14.0 发布记录
description: V10.14.0 发布记录 — 8 项改进：Reasonix 吸收 + 速度优化 + 测试修复
metadata:
  type: project
---

## V10.14.0 发布记录 — 自我进化迭代

### 🧠 Reasonix 吸收

#### 1. 成功循环检测 `repeatedSuccessBlock`
- 检测写工具在同一用户轮次中重复成功调用，阈值 2 次后阻止
- 防止模型无意义循环消耗 token
- 7 个辅助函数完整移植自 Reasonix

#### 2. 参数修复提示 `schema echo`
- 模型发出非法 JSON 参数时，错误消息自动附带工具 schema
- 帮助模型一次修正，节省一整轮 API 调用

#### 3. Grace Round 工具调用守卫
- 防止 MaxSteps 限制下无限循环
- 移植自 Reasonix，填补原有漏洞

### ⚡ 速度优化

#### 4. 流式 batcher 调参
- maxBytes: 8→32，flush 频率降低 75%
- Wails Go↔JS IPC 调用显著减少

#### 5. Precheck 缓存优化
- 消除 precheck→execute 的重复文件 I/O

### 🔧 测试修复（7 个既有问题全部修复）
- TestEarlyToolDispatch（超时）、TestMaybeCompact_WithUsage（断言）、TestPostLLMCall（batcher 影响）、TextSink ×3（格式变化）、EvidenceFlow ×3（行为匹配）

### 记忆更新
- 新增 `self-evolution-principle`：自我进化原则
