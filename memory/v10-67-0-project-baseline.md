---
name: v10-67-0-project-baseline
title: V10.67.0 项目基准
description: V10.67.0 项目基准 — 当前版本、构建命令、核心变更
metadata:
  type: project
---

# V10.67.0 项目基准

## 版本

- **版本号**: V10.67.0
- **分支**: master
- **发布时间**: 2026-07-14

## 构建

```bash
cd tianxuan/desktop && wails build
```

产物: `C:\Users\吴比\AppData\Roaming\tianxuan\bin\tianxuan.exe`

## 核心变更

### 从 Reasonix 蒸馏补齐设置面板

1. **ModelPicker** — 搜索式模型选择器（Reasonix SettingsPanel.tsx:4222）
2. **StepLimitControl** — 预设+自定义步数控件（Reasonix :1976）
3. **SettingsGeneral 增强** — 桌面布局/关闭行为/显示模式/声音/状态栏/工具审批
4. **SettingsShortcuts 录制模式** — 交互式快捷键录制+冲突检测（Reasonix :564）
5. **SettingsHooks 管理 UI** — 钩子编辑/JSON导入导出（Reasonix :6255）
6. **SettingsSandbox Shell 选择器** — auto/bash/powershell/pwsh
7. **后端 API** — SetStatusBarStyle + SetStatusBarItems

### 技术栈

- Go 后端 + Wails v2.12 桌面框架
- React 18 + TypeScript 前端
- DeepSeek API (双模型：planner + executor)
