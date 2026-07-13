# tianxuan 设置面板长期补齐规划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从 DeepSeek-Reasonix 蒸馏设置面板功能，补齐 tianxuan 缺失的 Tab、参数和布局，同时保留 tianxuan 自身进化优势（按技能子代理模型、独立温度/effort、神话命名、8色配色方案）。

**Architecture:** 保持 tianxuan 现有的右侧可拖拽抽屉 + 主从视图布局；扩展 SettingsPanel 从 7 个 Tab 到 14+ 个 Tab；左侧导航引入分组和搜索；右侧内容区采用统一 SettingsSection/SettingsField 组件体系；后端通过 config/edit.go mutation API 安全写入。

**Tech Stack:** React 18 + TypeScript + Tailwind CSS + lucide-react (前端)；Go (后端)；TOML 配置

---

## 全局约束

1. **不照抄**：结合 tianxuan 自身进化补齐，保留差异化优势
2. **保留 tianxuan 优势**：按技能子代理模型、独立 plannerTemperature/subagentTemperature、三段 effort 按钮、神话命名 (Hephaestus/Hermes)
3. **布局对齐**：完全对齐 Reasonix 设置面板布局（分组导航 + 搜索 + SettingsSection/SettingsField 组件体系）
4. **前后端一起补**：所有新增参数都需要后端 API 支持
5. **跳过 Bots**：QQ/飞书/微信机器人模块不纳入
6. **TDD 铁律**：无失败测试不写产品代码
7. **手术级变更**：每次改动只包含任务要求内容

---

## 第一阶段 (P1)：布局重构 + Agent 核心参数补齐

**目标**：设置面板从 7 Tab 扩展到 14 Tab 的布局框架 + 对 Agent 行为影响最大的 9 个核心参数

### 布局重构

| 变更项 | 当前 | 目标 |
|--------|------|------|
| Tab 数量 | 7 个 | 14 个（新增 general/network/mcp/skills/subagents/plugins/memory/hooks）|
| 左侧导航 | 平铺列表 | 分组（核心/能力/高级）+ 搜索框 |
| 右侧渲染 | `tab === "xxx" &&` 链 | `Record<SettingsTab, Component>` 映射表 |
| Tab 组件接口 | 不一致（5 个用 SectionProps，2 个不用） | 统一 SectionProps |
| 内容包装 | 无统一包装 | SettingsPageShell（标题 + 描述） |

### Agent 核心参数补齐（General tab 内）

| 新参数 | 类型 | 来源 | 用途 |
|--------|------|------|------|
| `subagentDepth` | `number` (1-3) | Reasonix | 子代理递归深度上限 |
| `plannerMaxSteps` | `number` (6/12/25/0=自动) | Reasonix | 规划器独立步数限制 |
| `executorMaxSteps` | `number` (10/25/50/0=自动) | Reasonix | 执行器步数预设按钮（现有手动输入改为预设） |
| `reasoningLanguage` | `string` (auto/zh/en) | Reasonix | 推理文本语言偏好 |
| `coldResumePrune` | `boolean` | Reasonix | 冷恢复时修剪过期上下文 |
| `autoPlan` | `string` (off/ask/on) | Reasonix | 自动规划模式 |
| `memoryCompilerEnabled` | `boolean` | Reasonix | Memory v5 编译器开关 |
| `defaultToolApprovalMode` | `string` (ask/allow/deny) | Reasonix | 新会话默认审批模式 |
| `outputStyle` | `string` (default/concise/explanatory) | Reasonix | 输出风格 |

### 涉及文件

| 文件 | 操作 | 变更内容 |
|------|------|---------|
| `desktop/frontend/src/components/SettingsShared.tsx` | 修改 | 扩展 SettingsTab 类型、TAB_GROUPS 分组定义、搜索过滤、统一 meta/label |
| `desktop/frontend/src/components/SettingsPanel.tsx` | 修改 | 布局重构：分组导航 + 搜索 + 映射表渲染 + SettingsPageShell |
| `desktop/frontend/src/components/SettingsGeneral.tsx` | **新建** | General tab：Agent 运行时参数（9 个新参数）+ 桌面偏好 |
| `desktop/frontend/src/components/SettingsNetwork.tsx` | **新建** | Network tab：代理设置（Phase 2 实现内容，Phase 1 仅占位） |
| `desktop/frontend/src/components/SettingsMcp.tsx` | **新建** | MCP tab 占位 |
| `desktop/frontend/src/components/SettingsSkills.tsx` | **新建** | Skills tab 占位 |
| `desktop/frontend/src/components/SettingsSubagents.tsx` | **新建** | Subagents tab 占位 |
| `desktop/frontend/src/components/SettingsPlugins.tsx` | **新建** | Plugins tab 占位 |
| `desktop/frontend/src/components/SettingsMemory.tsx` | **新建** | Memory tab 占位 |
| `desktop/frontend/src/components/SettingsHooks.tsx` | **新建** | Hooks tab 占位 |
| `desktop/frontend/src/lib/types.ts` | 修改 | SettingsView 新增 9 个字段 + AgentView 扩展 |
| `desktop/frontend/src/lib/bridge.ts` | 修改 | 新增 bridge 方法 |
| `desktop/frontend/src/locales/zh.ts` | 修改 | 新增 settings.* i18n 键 |
| `desktop/frontend/src/locales/en.ts` | 修改 | 新增 settings.* i18n 键 |
| `desktop/settings_app.go` | 修改 | SettingsView 新增字段 + 9 个新 setter 方法 |
| `internal/config/config.go` | 修改 | AgentConfig 新增字段 |
| `internal/config/edit.go` | 修改 | 新增 mutation 方法 |

---

## 第二阶段 (P2)：新核心模块补齐

**目标**：补齐直接影响用户体验的核心模块

### Network 代理设置
- 代理模式：off / system / custom 三段按钮
- 自定义代理 URL 输入
- NoProxy 列表（逗号分隔）
- 代理认证（用户名/密码）

### Desktop 外观增强
- 布局风格：classic / workbench / creation
- 显示模式：standard / compact
- 关闭行为：background / quit
- 状态栏样式：icon / text
- 状态栏项目：可拖拽排序列表
- 字体大小：5 档预设
- 显示缩放：百分比滑块
- 自定义字体输入

### General 重组
- 将 appearance 中的语言设置移入 General
- 将 updates 中的自动检查/遥测/指标移入 General
- 工具审批模式
- 自动规划
- Memory 编译器

---

## 第三阶段 (P3)：能力管理面板

**目标**：补齐 MCP/Skills/Plugins/Subagents/Memory/Hooks 的管理 UI

### MCP 服务器管理
- 服务器列表（名称、命令、状态）
- 添加/编辑/删除服务器
- 工具发现状态
- 连接测试

### Skills 管理
- 技能根目录配置
- 技能列表与启用/禁用
- 排除路径

### Plugins 管理
- 插件列表（名称、路径、状态）
- 启用/禁用
- 技能/Hooks/MCP 统计

### Subagents 管理
- 子代理列表（名称、模型、effort、允许工具）
- 添加/编辑/删除
- 颜色标记

### Memory 管理
- Memory 编译器开关
- 详细度控制
- 记忆条目列表

### Hooks 管理
- 事件列表（类型、命令、超时）
- 添加/编辑/删除
- 作用域（global/project）
- 信任状态

---

## 第四阶段 (P4)：体验增强

**目标**：Provider 高级字段 + 交互升级 + 诊断/快捷键/更新增强

### Provider 增强
- 内置预设提供者（DeepSeek/OpenAI/Anthropic 等一键添加）
- Fetch Models API 自动发现
- 视觉模型独立标记
- reasoningProtocol / thinking 模式
- supportedEfforts / defaultEffort
- modelOverrides 按模型能力覆盖

### ModelPicker 升级
- 搜索过滤
- 按 provider 分组
- Key 状态徽章
- 模型不可用警告

### Diagnostics
- Skills/Commands/Hooks/Plugins 健康检查
- 配置错误报告
- 网络诊断
- 一键跳转修复

### Shortcuts
- 键盘快捷键列表
- 自定义/重置
- 冲突检测

### Updates 增强
- 自动检查开关
- 遥测开关
- 指标收集开关
- Channel 标签

---

## 执行策略

推荐使用 **subagent-driven-development**：
1. 按阶段执行，每阶段一个 session
2. 每个 Task 派发独立 subagent
3. 两阶段审查（规格合规 → 代码质量）
4. 每阶段结束时 git commit + tag

---

*规划文档版本: v1.0 · 2026-07-13*
