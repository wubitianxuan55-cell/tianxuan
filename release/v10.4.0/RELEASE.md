# V10.4.0 发布记录

**发布日期**: 2026-06-27
**构建产物**: `release/v10.4.0/tianxuan-desktop.exe` (16 MB)
**前端版本**: `package.json` version = "10.4.0"

## 核心变更

### Superpowers 方法论全面融合
- **AGENTS.md 重写**: "技能优先"→"工具优先"原则，7 条编码铁律（设计优先/TDD/验证强制/无根因不修复/无占位符/持续执行/拒绝谄媚）
- **8 条常见推辞识别表**: 模型理性化借口的对照表，自动生效

### 工具集优化
- **移除 4 个低价值工具**: doctor, time, verify(功能融入bash), design_session
- **bash 增强**: 超时 2→5分钟，增加 `output_format=json` 返回结构化验证结果
- **grep 增强**: 默认限制 200→500，增加 `max_matches` 参数（最大2000）
- **edit_file/multi_edit 诊断增强**: `old_string not found` 时报告行尾类型(CRLF/LF)+内容预览
- **路径规范化**: `resolveIn` 增加 `filepath.Clean()` 自动解析 `..`/`.`
- **complete_step**: manual 证据可接受，描述更新
- **git_commit/git_status/git_diff**: 描述嵌入 TDD 铁律

### 技能精简
- 内置技能 10→4: 移除 inline 技能(init/tdd/lsp/context7/debug/receiving-code-review)
- 保留 4 个子代理技能: explore, research, review, security-review
- debug/tdd 方法论已融入 AGENTS.md 宪法和工具描述

### 前端优化
- **记忆面板全面中文化**: MemoryPanel/FactCard/DocEditor 接入 i18n
- **滚动修复**: Transcript 工具输出完成不再强制跳到底部
- **工具图标清理**: 移除已删除工具的图标映射
- **RuntimePanel 分类更新**: 移除空的"系统"分类

### 网页工具改进
- web_fetch 摘要不再显示 JSON 信封
- web_search 增加 subjectOf(搜索词) + summarize(结果条数)
- SmartCompress 覆盖 web_fetch/web_search

### 文件变更
- 27 个文件修改，+395 / -307 行
- 删除: doctor.go, time.go
- 未跟踪新增: design_session.go(已删), verify.go(已删)

## Why
V10.4.0 是工具集重大精简和 Superpowers 方法论深度融合版本。基于 agent 自身数万次工具调用的实际体验，优化了最高频痛点（edit_file 匹配失败、bash 超时、滚动劫持、技能调用率低等）。工具从 28→24，技能从 10→4，代码更精简，方法论更系统。

## How to apply
后续版本号在此基础上递增；构建时使用:
```sh
cd tianxuan/desktop
pnpm --dir frontend build
wails build
```
