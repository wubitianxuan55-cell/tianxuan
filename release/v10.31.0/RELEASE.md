# V10.31.0 发布记录

> 双模型弹性降级 + 统计面板规划/执行拆分 + 子代理冷启动优化 · 2026-07-04

---

## 🚀 双模型弹性降级

### 快速路径
- `!` 前缀显式标记：`!fix typo in line 42` 直接跳过规划执行
- 启发式自动检测：<120字符 + 简单操作关键词自动跳过
- 关键词：fix typo, rename variable, update comment, add comment, format code, delete line
- Phase 显示 `模型名 · 快速执行`
- **文件**：`coordinator.go`

---

## 📊 统计面板重设计

### 规划/执行/子代理三组拆分
- 后端 Meta 添加 `plannerLabel` 字段
- store 拆分 `perTurnMainUsage` → `perTurnPlannerUsage` + `perTurnExecutorUsage`
- StatsTable 4 列表格：规划 | 执行 | 子代理
- 汇总行移至表格下方独立展示
- 命中率大字高亮（对齐当前步样式）
- 规划/执行/子代理分别使用各自的 modelPrice
- 子代理价格修正为使用 subagentModel
- **文件**：`app.go`, `app_meta.go`, `store.ts`, `StatsPanel.tsx`, `App.tsx`, `types.ts`

### 布局优化
- 汇总行移至表格下方，紧凑单行展示
- 缓存命中率 text-xl font-bold 大字 + 命中/未命中明细
- 3 条命中率趋势线（规划 accent / 执行 blue / 子代理 warn）

---

## ⚡ 子代理冷启动优化

### 工具直用优先
- AGENTS.md 新增规则：简单查询直接用底层工具
  - 符号定位 → `lsp_definition` / `codegraph_search`
  - 函数调用关系 → `codegraph_node` / `codegraph_trace`
- explore 子代理 body 新增 Fast Path 指令
- **文件**：`AGENTS.md`, `builtins.go`

---

## 🧹 记忆债务清理

- 4 个旧基准标记为"已归档（当前 V10.30.0）"
- version-history.md 补充 V10.22-V10.30 版本记录
- **文件**：`memory/v10-*-project-baseline.md`, `memory/version-history.md`

---

## 📦 构建信息

- **产物**：`release/v10.31.0/tianxuan-desktop.exe`
- **SHA256**：`a688bb0b3744cdb61c48da48b4556b1047466b88a5cd81a9fcbebae79fa79d91`
- **构建命令**：`cd tianxuan/desktop && wails build`
- **变更**：11 文件，~180 行新增
