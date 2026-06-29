---
name: v10-14-project-baseline
title: V10.14.0 项目基准
description: V10.14.0 项目基准 — 旧版本（已归档，当前 V10.15.0）
metadata:
  type: project
---

# V10.14.0 项目基准（已归档）

> **当前版本**: V10.15.0 — 参见 [[v10-15-project-baseline]]
metadata:
  type: project
---

## 当前版本
- **版本号**: V10.14.0
- **发布日期**: 2026-06-29
- **构建产物**: `release/v10.14.0/tianxuan-desktop.exe`
- **SHA256**: `a02321f19f0dc963e3baf24fa696272c7ebc4773d9b53c10da84d82c4d4d4ea1`
- **构建命令**: `cd tianxuan/desktop && wails build`
- **发布位置**: `release/vX.Y.Z/tianxuan-desktop.exe`

## 核心变更 — 自我进化迭代

8 项改进（Reasonix 移植 + 速度优化 + 测试修复）：

### 🧠 Reasonix 吸收
1. **成功循环检测** — 写工具重复成功 ≥2 次自动阻止（移植自 Reasonix repeatedSuccessBlock）
2. **参数修复提示** — 非法 JSON 参数时附带工具 schema（移植自 Reasonix schema echo）
3. **Grace Round 守卫** — 防止 MaxSteps 限制下无限工具调用循环

### ⚡ 速度优化
4. **流式 batcher 调参** — maxBytes 8→32，Wails IPC 减少 75%
5. **Precheck 缓存** — 读盘后写入 toolCache，消除重复 I/O

### 🔧 测试修复
6. 7 个既有测试修复（2 超时 + 5 断言失败）

## 构建位置固定规则
- `wails build` **不加 `-o`**，依赖 `wails.json` 的 `outputfilename` 决定输出 → `desktop/build/bin/tianxuan-desktop.exe`
- 发布时拷贝：`cp desktop/build/bin/tianxuan-desktop.exe release/vX.Y.Z/tianxuan-desktop.exe`

## 不变
TCCA 四层架构、事件驱动管线、缓存前缀不变性约束、自我进化原则均不变。

**How to apply:** `cd tianxuan/desktop && wails build`
