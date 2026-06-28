# Tianxuan 版本变更日志

## V10.9.0 (2026-06-28) — 🧠 记忆建议引擎 + 多标签页骨架 + UI 增强

### 记忆建议引擎（借鉴 DeepSeek-Reasonix V1.13）
- **自动检测记忆候选**: 16 个中英文关键词（记住/always/偏好/约定 等）从用户消息中自动提取，纯本地运算不消耗 token
- **工作流技能自动生成**: 3 个模板（code-review/refactor/bug-fix）从历史检测重复模式→自动生成 SKILL.md
- **一键采纳**: AcceptMemorySuggestion / AcceptSkillSuggestion，记忆→Store.Save，技能→skill.CreateWithContent
- **归档记忆列表**: ListArchived() + ArchivedMemory 类型，store.go +80 行
- **[[wiki-link]] 内联渲染**: 记忆正文中 [[name]] 渲染为可点击跳转链接，死链接灰色删除线提示

### 多标签页系统（骨架）
- **WorkspaceTab 类型**: 独立 ID/Scope/WorkspaceRoot/SessionPath/Ctrl，为多标签准备
- **App.tabs map**: 所有绑定方法统一改用 ctrlByTabID("") 路由（20+ 方法重构），完全向后兼容
- **tabEventSink + toWireTab**: 事件注入 tabId 供前端路由，全局 eventSink 自动注入活跃 tabID
- **TabBar 前端组件 + 持久化**: desktop-tabs.json 保存恢复，SelectTab/TabMeta API

### UI 增强
- **PromptShelf 组件**: 共享架子（头部+进度条+折叠体+按钮），TodoPanel 重构使用
- **快速添加路径提示**: MemoryPanel 显示"保存至: ~/.tianxuan/..."路径
- **FactCard 增强**: wiki-link 内联渲染、编辑/删除/确认删除交互

### 借鉴来源
- DeepSeek-Reasonix V1.13.0 桌面端代码深度分析
- 记忆建议引擎 (memory_suggestions.go, 440行) ← Reasonix
- 多标签页骨架 (tabs.go + 路由重构) ← Reasonix
- PromptShelf ← Reasonix

## V10.8.0 (2026-06-28) — 🔵 智能化

- **compact 保留 todo**: 压缩前读取 .tianxuan/progress.md 注入指令，防止进度丢失
- **增强 commit message**: autoCommitMessage 包含文件名摘要（≤3 列出名字）
- **grep 相关性排序**: sort_by=relevance 按匹配密度排序

## V10.7.0 (2026-06-28) — 🟢 工作流支持

- **git_worktree 工具**: 新增 add/remove/list 操作，支持隔离并行开发
- **计划进度持久化**: todo_write 同步写入 .tianxuan/progress.md（Markdown 表格）
- **main/master 分支检测**: git_commit 在主分支上拒绝提交

## V10.6.0 (2026-06-28) — 🟡 可靠性增强

- **web_fetch 自动重试**: retries 参数 + 指数退避 1s→2s→4s + isTransientError 智能判断
- **bash stdout/stderr 分离**: json 模式返回独立 stdout/stderr 字段
- **子代理超时部分结果**: extractLastAssistantMessage + "(partial result returned)" 标签

## V10.5.0 (2026-06-28) — 🔴 编辑体验革命

- **edit_file 自动行尾适配**: CRLF/LF 自动检测和转换，multi_edit 同步适配
- **edit_lines 工具**: 按行号编辑（1-based），自动保留原始行尾格式
- **read_file 无行号输出**: line_numbers=false 输出纯文本

## V10.4.0 (2026-06-26) — Superpowers 融合 + 工具精简

- AGENTS.md: 7 条编码铁律 + 8 条推辞识别表
- 技能 10→4: 仅保留 explore/research/review/security-review（子代理）
- 工具 28→24: 移除 doctor/time/verify/design_session
- bash 超时 2→5min + output_format=json
- grep 200→500 + max_matches 参数
- edit_file: old_string not found 诊断增强
- 前端: 记忆面板中文化 + Transcript 滚动修复 + web 工具摘要

## V10.3.0 (2026-06-26)

- 统计面板合并: 子代理和主模型统计统一
- MessageNavigator: 右侧面板第5标签，消息列表+跳转
- 外观重设计: 9 主题配色 + 字体设置
- Plan Mode: explore/research/review/security_review 在只读模式下可用

## V10.2.0 (2026-06-24~25)

- UI 优化 + app.go 拆分为 5 个子模块 + 空间清理

## V10.1.0 (2026-06-26)

- 全量蒸馏: 13 commits, 40+ files, 2500+ lines
- Go 后端 6 机制 + React 前端 11 组件 + CSS 4 组 token

---

## 构建产物索引

| 版本 | 路径 | 大小 | SHA256 |
|------|------|------|--------|
| V10.2.0 | release/v10.2.0/tianxuan-desktop.exe | 16 MB | — |
| V10.4.0 | release/v10.4.0/tianxuan-desktop.exe | 16 MB | — |
| V10.5.0 | release/v10.5.0/tianxuan-desktop.exe | 16 MB | — |
| V10.6.0 | release/v10.6.0/tianxuan-desktop.exe | 16 MB | — |
| V10.7.0 | release/v10.7.0/tianxuan-desktop.exe | 16 MB | — |
| V10.8.0 | release/v10.8.0/tianxuan-desktop.exe | 16 MB | `b9671ae8f408…` |
