---
name: v10-13-project-baseline
title: V10.13.0 项目基准
description: V10.13.0 项目基准 — 当前版本、构建命令、核心变更
metadata:
  type: project
---

## 当前版本
- **版本号**: V10.13.0
- **发布日期**: 2026-06-29
- **构建产物**: `desktop/build/bin/tianxuan-desktop.exe`
- **构建命令**: `cd tianxuan/desktop && wails build`
- **发布位置**: `release/vX.Y.Z/tianxuan-desktop.exe`

## 核心变更 — 体验打磨迭代

7 项改进：
1. 删除流式输出闪烁光标
2. 修复流式文本布局抖动（contain: layout style）
3. 修复同一阶段多思考卡（reasoning 同步 dispatch）
4. 清除"计划模式"概念（6 个 Go 文件，模型不再说 plan mode）
5. 底栏模式子状态（探索·只读 / 开发·可写 / 编排·规划中·执行中）
6. 思考卡默认折叠
7. 工具卡空间紧缩（-30~40% 垂直空间）

附带 V10.12.0 未提交变更：CodeViewer 重设计、hljs LRU、22 语言、BEM 层、shimmer、入口动画、虚拟列表移除等。

## 构建位置固定规则
- `wails build` **不加 `-o`**，依赖 `wails.json` 的 `outputfilename` 决定输出 → `desktop/build/bin/tianxuan-desktop.exe`
- 发布时拷贝：`cp desktop/build/bin/tianxuan-desktop.exe release/vX.Y.Z/tianxuan-desktop.exe`

## 不变
TCCA 四层架构、事件驱动管线、缓存前缀不变性约束均不变。

**How to apply:** `cd tianxuan/desktop && wails build`
