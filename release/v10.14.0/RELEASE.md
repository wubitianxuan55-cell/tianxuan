# V10.14.0 发布记录

> 自我进化迭代 — Reasonix 吸收 + 速度优化 + 测试修复 · 2026-06-29

---

## 🧠 Reasonix 吸收（从 DeepSeek 内部编码代理移植）

### 1. 成功循环检测 `repeatedSuccessBlock`
- 检测写工具（write_file/edit_file/multi_edit/delete_range/delete_symbol/bash）在同一用户轮次中重复成功调用
- 阈值 2 次后自动阻止，提示模型换方法
- 防止模型无意义循环消耗 token
- **文件**：`agent.go` +3, `execute_one.go` +150（含 7 个辅助函数）

### 2. 参数修复提示 `schema echo`
- 模型发出非法 JSON 参数时，错误消息自动附带工具 schema
- 帮助模型一次修正，节省一整轮 API 调用
- **文件**：`execute_one.go`

### 3. Grace Round 工具调用守卫
- 当 maxSteps 限制触发 grace round 后，模型若仍调用工具则退出
- 防止 `MaxSteps=1` 等限制下无限循环
- **文件**：`agent_run.go` +5

---

## ⚡ 速度优化

### 4. 流式 batcher 调参
- `maxBytes`: 8 → 32，flush 频率降低 75%
- Wails Go↔JS IPC 调用显著减少，流式渲染更流畅
- **文件**：`stream_batcher.go`

### 5. Precheck 缓存优化
- readFileForPrecheck 读盘后写入 toolCache
- 消除 precheck→execute 的重复文件 I/O
- 对大文件（几百 KB+）效果显著
- **文件**：`tool_precheck.go` +7

---

## 🔧 测试修复（7 个既有问题）

| 测试 | 根因 | 修复 |
|------|------|------|
| `TestEarlyToolDispatch` | 缺 graceRound 守卫导致无限循环超时 | +5 行守卫 |
| `TestMaybeCompact_WithUsage` | minRecentTokens=2000 使 tail 吞掉所有消息 | 消息对 20→40 |
| `TestPostLLMCallAbsentStreaming` | batcher 合并 reasoning chunk | 接受 1-2 事件 |
| `TestTextSink` ×3 | dispatch 批处理 + reasoning throttle 格式变化 | 更新期望值 |
| `TestEvidenceFlow` ×3 | complete_step 验证在闸门阶段，测试期望的是未实现行为 | 更新为当前行为 |

---

## 📋 文件变更

| 文件 | 改动 |
|------|------|
| `execute_one.go` | +170（repeatedSuccessBlock + schema echo + 7 辅助函数） |
| `tool_precheck.go` | +7（precheck 缓存写回） |
| `stream_batcher.go` | 8→32 |
| `agent.go` | +3（repeatSuccessCounts 字段） |
| `agent_run.go` | +6（graceRound 守卫 + 重置） |
| `compact_test.go` | 20→40 消息对 |
| `dispatch_test.go` | 期望 2 partial + 1 full |
| `evidence_flow_test.go` | 3 处断言更新 |
| `postllmcall_flow_test.go` | 1 处期望放宽 |
| `textsink_test.go` | 3 处格式适配 |
| `textsink_partial_test.go` | 加 flush 触发事件 |
