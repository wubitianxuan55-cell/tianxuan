## [8.0.6] — 2026-06-18

### 最后一轮清理

- **`2>>` stderr 重定向修复** (`plan_bash.go`): hasShellRedirect 正确处理 `2>> file.log`（append stderr），不误拦
- **steer 全面测试覆盖** (`steer_test.go` 新文件): 4 个测试覆盖全 blocked / 真失败 / 混合场景 / 单失败

### 全量审计结论

6 项深度检查全部通过：
- dedupHashes 跨 batch 复用正确
- dispatcher + inline fallback 三条门禁路径等价
- planBashAllowed → Permission gate 可达
- FilteredSchemas 非死代码（被 context.Manager 使用）

### 发布

- CLI: `tianxuan.exe` (13.4 MB)
- 桌面端: `tianxuan-desktop.exe` (16.7 MB, Wails v2.12.0)

---

## [8.0.5] — 2026-06-18

### shouldMidTurnSteer 计划模式误判修复

- 将 `"blocked:"` 结果从失败计数中分离（plan mode 下 blocked 是正常行为）
- 全部 blocked → steerCount 重置为 0，不触发误导性 steer
- 只有非 blocked 的真正失败才计入 steer 判断

---

## [8.0.4] — 2026-06-18

### Plan bash 安全修复

- **`2>&1` 误拦修复**: 从 shellMetachars 移除 `&`（`&&` 单独检测），`2>` 正确处理
- **`go test` 白名单**: 新增到安全命令列表
- **危险参数检测**: `find -delete/-exec` 和 `go -fix/-mod` 在安全命令中仍被拦截
- **`>>`/`<<`/`<`/`>`**: 从 shellMetachars 移除，重定向统一由 hasShellRedirect 精确处理

---

## [8.0.3] — 2026-06-18

### Plan 模式 bash 安全白名单

- `plan_bash.go` 新文件: planBashCheck + hasShellRedirect + 20+ 安全命令
- Shell 元字符检测: `&&`, `||`, `$()`, `` ` ``, `;`, `|`
- 文件重定向检测
- 接入 dispatcher 和 inline fallback 两条路径

---

## [8.0.2] — 2026-06-18

### 致命修复: filteredSchemas 破坏前缀缓存不变性

**根因**: filteredSchemas 按输入动态切换 tools 列表，与 `verifyPrefix` 的"前缀永远不变"设计冲突。不同轮次 tools 变化会导致 SHA256 指纹不匹配 → panic。

**修复**: runDirect 中改回 `activeSchemas = nil`，filteredSchemas 保留为公开方法供首轮一次性调用。

**教训**: DeepSeek 前缀缓存要求 tools 列表整个会话不变，任何 per-turn 工具过滤都会破坏缓存。

---

## [8.0.1] — 2026-06-18

### 4 Bug 修复

- filteredSchemas 死代码 → 现在会被 runDirect 调用
- steerCount 跨 turn 泄漏 → turn 开始时重置
- MatchesTool 遗漏 PermissionRequest → 加入事件列表
- PermissionRequest 未接入执行路径 → dispatcher + inline fallback 都加了

---

## [8.0.0] — 2026-06-18

### 8 项新特性

| 特性 | 层级 | 说明 |
|------|:--:|------|
| 工具按需过滤 | P0 | FilteredSchemas 按上下文动态精简工具列表 |
| 确定性结果剪枝 | P0 | dedupHashes 相同调用结果不重复发 token |
| Mid-turn Steer | P0 | 检测全失败 batch，注入纠偏提示 |
| Plan 智能澄清 | P1 | 模糊输入主动追问 |
| read_skill 工具 | P1 | Agent 按需读取技能 body |
| Context7 MCP | P2 | 环境变量自动启用库文档 MCP |
| /goal 命令 | P2 | 大目标分解为子任务 |
| PermissionRequest Hook | P2 | 新 hook 事件 + 接口 + Runner 实现 |

12 文件，~350 行 Go。
