---
name: v10-67-0-release-notes
title: V10.67.0 发布记录
description: V10.67.0 发布记录 — 从 Reasonix 蒸馏补齐设置面板
metadata:
  type: reference
---

## V10.67.0 发布记录

### 新功能

1. **ModelPicker 组件** — 从 Reasonix 蒸馏的搜索式模型选择器。支持按提供者分组显示、搜索过滤、键盘导航。替换 SettingsModels 中的 ModelSwitcher

2. **StepLimitControl 组件** — 预设按钮组 + 自定义数字输入框的组合步数控件。规划器步数预设 [6/12/25/∞]，执行器步数预设 [10/25/50/∞]

3. **SettingsGeneral 大幅增强** — 新增桌面布局风格（经典/工作台/创作）、关闭行为（退出/最小化到托盘）、显示模式（标准/紧凑）、工具审批模式（询问/自动/YOLO）、声音提示音配置、状态栏样式（图标/文字）和状态栏项目复选框。保留原有的规划/子代理/推理/上下文/Memory 区块

4. **SettingsShortcuts 录制模式** — 点击快捷键按钮进入录制模式，按新组合键即可修改。冲突检测（冲突时显示告警）、重置按钮

5. **SettingsHooks 管理 UI** — 可编辑的钩子列表（事件/匹配/命令/描述/超时），JSON 导入/导出，添加/删除功能

6. **SettingsSandbox Shell 选择器** — 新增 Shell 类型下拉（自动检测/Bash/PowerShell/PWSh Core）

7. **后端 API** — 新增 `SetStatusBarStyle` + `SetStatusBarItems` 两个 setter

### 文件变更

| 文件 | 变更 |
|------|------|
| `ModelPicker.tsx` **[新]** | 搜索式模型选择器（+6496 bytes） |
| `StepLimitControl.tsx` **[新]** | 预设+自定义步数控件（+2454 bytes） |
| `SettingsGeneral.tsx` | 增强（+7178 bytes） |
| `SettingsShortcuts.tsx` | 录制模式重写（+5998 bytes） |
| `SettingsHooks.tsx` | 管理 UI 重写（+7579 bytes） |
| `SettingsModels.tsx` | ModelPicker + StepLimitControl 集成 |
| `SettingsSandbox.tsx` | +Shell 选择器 |
| `settings_app.go` | +SetStatusBarStyle, SetStatusBarItems |
| `edit.go` | +SetStatusBarStyle, SetStatusBarItems |
| `bridge.ts` + `mock.ts` | 桥接声明补齐 |

### 验证

- `go test ./... -count=1` — 全部通过
- `go vet ./...` — 零警告
- `go build ./desktop/...` — 编译通过
- `npx tsc --noEmit` — 零错误
