---
name: v10-147-release-notes
title: V10.147.0 发布记录
description: V10.147.0 发布记录 — 技能系统重构（子代理化+工具化+压缩）+ edit_lines 吞行修复 + DeepSeek 峰谷计价 + 记忆增强
---

## V10.147.0 发布记录

| 项目 | 值 |
|------|-----|
| 版本号 | V10.147.0 |
| 发布日期 | 2026-08-02 |
| 代码变更 | ~25 文件（boot/skill/config/control/memory/provider/desktop/前端） |
| 主题 | 技能系统重构 + 修复 + 计价 + 记忆增强 |
| 构建产物 | `tianxuan-desktop.exe` |
| 文件大小 | 21,296,128 bytes (~20.3 MB) |
| SHA256 | `765c3de46869e50b271940e4f7695d9453efc19076c560be679c82323b1d9196` |

### 本次发布包含的改动

| 模块 | 主题 |
|------|------|
| 技能系统 | explore/research/review/security_review 独立工具；5 个设计技能转子代理；ui_styling 工具；design_router 路由工具 |
| 技能系统 | 索引去重（子代理技能不进索引）；bundled 分发源同步；Compact 默认开启（55→46 工具） |
| edit_lines | 修复末尾空行被吞（LF/CRLF 两回归测试） |
| 计价 | DeepSeek 峰谷计价（PeakMultiplier=2，北京时间高峰翻倍）全链路生效 |
| 记忆 | 待确认 >30 条自动提炼为 1 条；面板建议项详情 + 技能中文说明 |

### 关键决策（5 级追溯）

- 根因：DeepSeek 对"先想起技能名 → run_skill"的间接路径不敏感，倾向自己 grep；
  `applyCompactToolset` 还把 explore 等合并进 task，进一步降低调用意愿
- 方案：技能注册为第一等工具（名字即触发词），子代理技能带自包含 Subagent mode；
  父级工具集压缩（隐藏低频 codegraph/LSP），隐藏工具在 explore 子代理内仍可用
- 工具可见性实测：46 个工具 / 3,590 tokens，技能索引 7 行 / 226 tokens，
  相比未优化理论值省 ~900 tokens/请求

### 验证

- `go test` boot/skill/control/agent/tool/cache/config/desktop 全绿；vet 干净
- `tsc --noEmit` 0 错误；vitest 66/66；`go build ./...` EXIT 0
- 真实环境诊断：9 个子代理技能全部可见、白名单工具全部有效
