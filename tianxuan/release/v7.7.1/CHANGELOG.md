## [7.7.1] — 2026-06-18

### 架构优化

- **双重 JSON Repair 消除** (`agent.go`): 提取 `repairArguments` 辅助函数，executeBatch 开头统一 repair，executeOne 不再重复 — 每轮节省 N×2 次 JSON marshal/unmarshal
- **toolcache O(1) 路径失效** (`toolcache.go`): `invalidatePath` 从线性扫描改为路径索引 `pathKeys map[string]map[string]struct{}`，消除写入密集型工作流 O(n²) 退化

### Bug 修复

| Bug | 级别 | 文件 | 修复 |
|-----|:----:|------|------|
| serve_test nil deref | 🔴 | `serve_test.go` | `http.Post` 失败时 resp2 为 nil，加 `err2` 检查 |
| serve_test 端口耗尽 | 🔴 | `csrf_test.go` | `http.DefaultClient` → `srv.Client()`（Windows TCP TIME_WAIT） |
| debug skill 未注册 | 🟡 | `builtins.go` | `builtinDebugBody` 已定义但未注册到 `builtinSkills()` |

### 测试覆盖

- `skill_test.go`: 新增 `TestBuiltinDebugIsInlineSkill`（debug 技能存在性 + 属性校验）

### 发布

- CLI: `tianxuan.exe` (13.3 MB)
- 桌面端: `tianxuan-desktop.exe` (16.7 MB, Wails v2.12.0)

---

## [7.7.0] — 2026-06-17

### 内置技能全面升级

从零散的 Agent 技能集 → 8 个深度整合 tianxuan 独特能力的技能。

| 技能 | 变化 | 关键提升 |
|------|:--:|----------|
| **`explore`** | 升级 | 融合 CodeGraph 工具选择表 + 7 个 codegraph 工具 + 3 个 LSP 查询工具 |
| **`research`** | 升级 | 同 explore + web_search/web_fetch |
| **`review`** | 升级 | 用 `git_status/git_diff/git_log` 替换 `bash git`，加入 `codegraph_impact` 影响分析 + `lsp_diagnostics` 编译检查 |
| **`security-review`** | 升级 | 同 review + `codegraph_trace` 追踪输入路径 |
| **`tdd`** | 重写 | 吸收 debug 的隔离阶段：RED → GREEN → REFACTOR |
| **`lsp`** | 新增 | 诊断→理解→修复→验证完整工作流 |
| **`debug`** | 新增 | 4 阶段系统化调试：Reproduce → Isolate → Fix → Prevent |
| ~~`karpathy-guidelines`~~ | 移除 | 内容已在系统 prompt，冗余 |
| ~~`test`~~ | 移除 | 升级为 `tdd` |

### 清理

- 删除 9 个 `superpowers-*` + 1 个 `karpathy.md` 冗余用户级技能
- `skill_test.go` 同步更新工具白名单

### 发布

- CLI: `tianxuan.exe` (13 MB)
- 桌面端: `tianxuan-desktop.exe` (16 MB, Wails v2.12.0)
