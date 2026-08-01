---
name: v10-127-release-notes
title: V10.127.0 发布记录
description: V10.127.0 发布记录 — V10.112~127 累积 16 版本，39 文件 +2164/-191，技能系统重构 + 工具完善 + 服务进程卡死修复
---

## V10.127.0 发布记录

| 项目 | 值 |
|------|-----|
| 版本号 | V10.127.0 |
| 发布日期 | 2026-08-01 |
| 构建产物 | `tianxuan-desktop.exe` |
| 文件大小 | 21,140,992 bytes (~20.2 MB) |
| SHA256 | `757D636D57FAE1AA7D88CDFEE3447CDBF129F5672AC9E88834280B44B8927A32` |
| 代码变更 | 16 版本累积（V10.112~127），39 文件，+2164/-191 |

### 本发布包含的版本

| 版本 | 主题 |
|------|------|
| V10.112 | Hermes 项目地图增量缓存（Analyze 1.8s → Refresh 0ms） |
| V10.113 | 计划 File(s) 锚点交接（执行阶段免重复定位） |
| V10.114 | Refresh 增量检测扩展到 Node/TS 项目 |
| V10.115 | Refresh 增量检测扩展到 Rust 项目 + Rust 模块/类型发现 |
| V10.116 | verify_gate 长输出截断保留头尾（失败详情不再丢失） |
| V10.117 | git 工具长输出截断保护 + verify_gate 截断统一 |
| V10.118 | Rust 项目地图补模块与核心类型 |
| V10.119 | 蒸馏 Aider repo map — CoreTypes 按引用频率排名 |
| V10.120 | 重蒸馏 superpowers v5.1.0 — finish-development-branch 技能 |
| V10.121 | 修复技能几乎不调用 — run_skill 触发指引 + 核心工作流技能化 |
| V10.122 | 技能自动触发层（缓存安全：只改 user 消息、字节确定性） |
| V10.123 | 自动注入缓存加固（会话内去重 + 长度上限 + e2e 前缀稳定） |
| V10.124 | 工具与技能重新分离（explore/review 回归 run_skill 单一入口） |
| V10.125 | 蒸馏 receiving-code-review — 审查反馈接收技能 |
| V10.126 | 全量工具梳理（compact 接线 / Kind 一致化 / 工具集补全） |
| V10.127 | 修复 bash 启动服务类命令堵死进程（自动后台化 + Wait 永不阻塞） |

### 核心变更

#### 技能系统重构（V10.120~125）
- 技能自动触发层：用户输入按确定性规则匹配技能并自动注入正文，不再依赖模型自觉 run_skill；严格适配 DeepSeek 全消息前缀缓存（只改 user 消息、字节确定性、会话内去重、注入长度上限，e2e 测试锁定前缀稳定）
- 内置 6 个核心工作流技能（蒸馏 superpowers v5.1.0）：tdd / systematic-debugging / requesting-code-review / receiving-code-review / finish-development-branch，全部带"必须"触发规则
- 工具与技能重新分离：explore/research/review/security-review 从顶层工具回归 run_skill 单一入口（工具 = 原子操作，技能 = 工作流编排）

#### 项目地图（V10.112~119）
- Hermes 项目地图增量缓存 + Node/Rust 增量检测 + Rust 模块/核心类型
- CoreTypes 按全库引用频率排名（蒸馏 Aider repo map）

#### 工具完善（V10.116~117, 126）
- verify_gate / git_diff / git_log 长输出头尾保留截断
- code_index / move_file 接线 CompactDescriptor（每轮省 ~75% token）
- verify_gate Kind 一致化、coding 工具集补全 5 个工具

#### 关键修复（V10.127）
- bash 启动服务类命令堵死进程：服务命令自动转后台（kill_shell 可关闭）+ 前台 Wait 永不阻塞

### 构建命令

```
cd D:\AI\tianxuanX && build-desktop.bat
```

环境要求：PATH 需含 `tools\node`（node/npm/pnpm）、`C:\Users\Administrator\go\bin`（wails.exe）、`tools\go\bin`（go）。
