---
name: v10-20-project-baseline
title: V10.20.0 项目基准
description: V10.20.0 项目基准 — 旧版本（已归档，当前 V10.30.0）
metadata:
  type: project
---

## 当前版本
- **版本号**: V10.20.0
- **发布日期**: 2026-07-03
- **构建产物**: `release/v10.20.0/tianxuan-desktop.exe`
- **SHA256**: `fde38adb2259d1eee69c41841916b2c8fe4f49866ae462a0a55f879c3cb2fc3b`
- **构建命令**: `cd tianxuan/desktop && wails build`

## 核心变更 — 记忆升降级 + 阻塞修复 + Bug修复 + 清理
74 文件变更，+1991/-1786 行，Go build ✅，tsc ✅。

### 阻塞修复
- controller_approval: Lock 后补回 Unlock()，消除死锁
- controller: 添加 permLevel:"ask"，修复出厂即 YOLO 安全漏洞

### 新功能
- 记忆 Type 升降级: Store.ChangeType → Controller.ChangeFactType → App Wails 绑定
- 前端 FactCard 展开视图 👤用户/📋项目/💬反馈 按钮组
- i18n 三语言 5 个新 key

### Bug 修复
- StatsPanel: skipWriteRef 守卫防止新会话旧数据覆盖
- Transcript: turnEls 清理 + items 重置清空，修复消息面板跳转

### 清理
- websearch: DDG 死代码 ~150 行
- StatusBar: agentMode/yolo → permLevel badge
- serve_handlers: "bypass" → "autoApprove"
- builtin_test: DDG 测试删除

## 不变
TCCA 四层架构、事件驱动管线、缓存前缀不变性约束、自我进化原则均不变。
