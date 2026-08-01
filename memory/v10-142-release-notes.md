---
name: v10-142-release-notes
title: V10.142.0 发布记录
description: V10.142.0 发布记录 — 记忆系统重构（项目归一/自动提取/跨会话记忆/自主进化）+ 桌面端面板重排与交互修复
---

## V10.142.0 发布记录

| 项目 | 值 |
|------|-----|
| 版本号 | V10.142.0 |
| 发布日期 | 2026-08-01 |
| 构建产物 | `tianxuan-desktop.exe` |
| 文件大小 | 21,244,928 bytes (~20.3 MB) |
| SHA256 | `65534973E430E10C9CF60F4694E411563709516F89774932C286524E90EE6065` |
| 代码变更 | 5 提交，37 文件，+2922/-542 |

### 本发布包含的提交

| 提交 | 主题 |
|------|------|
| 21860cc | 记忆系统重构 — 项目归一存储 + 自动提取 + 召回增强 + Dream 自动化 + @session 引用 + 画像/强化 |
| b768cc7 | maybeAutoRecall 改为会话级一次性召回，符合四域缓存前缀不变性 |
| 9c78b25 | 记忆面板按新架构重排 — 待确认入口 + 项目/跨项目区分 + 画像卡 + 强化计数 |
| c7f7c90 | 文件预览改为弹窗，不再内嵌占用面板 |
| 592148a | 恢复运行中发送排队，纠偏改为显式按钮 |

### 架构要点（本发布）

- **项目基准归一（P1）**：`StoreFor` 按 git-root slug 分目录，同仓库子目录共享
  记忆；`GlobalDir=<userDir>/memories` 跨项目共享 user/feedback 事实；旧 cwd-slug
  存储 Load 时一次性迁移
- **自动提取（P2）**：每轮后扫 user+assistant 消息 → extract-cursor 游标防重复
  → 候选暂存 pending/，**用户确认后落盘**；过滤 transient 控制块、与已有记忆去重
- **召回增强（P3）**：SearchMatch 携带 Body+Mtime，注入正文截断 + 新鲜度提示；
  `maybeAutoRecall` 改为**会话级一次性**（首轮注入，前缀稳定优先）
- **Dream 自动化（P4）**：24h/5 会话/same_session 门控 + 机械整合跨会话主题为
  候选；顺带 90 天 TTL 弱记忆归档 + 项目画像重建
- **@session 引用（P5）**：确定性只读摘要（保留文本、工具结果压单行、8k 预算
  保最新），无 LLM 调用、失败不中断回合
- **画像与强化（P6）**：strength.json 强化计数 + TTL 归档（可恢复）+
  profile.json 画像（top concepts/类型分布/common errors）
- **桌面端**：记忆面板「待确认」入口/项目-跨项目区分/画像卡/召回 badge；
  文件预览弹窗化；运行中发送排队恢复 + 显式纠偏按钮

### 缓存前缀不变性

所有记忆机制遵守四域缓存约束：记忆变更一律 turn-tail 注入、下一 session 才折入
前缀；`maybeAutoRecall` 每轮注入改为首轮一次性（避免动态块落在缓存未命中区）；
未触碰 L1/L2/tools 列表/compaction/compress/diff 等敏感路径。

### 构建环境（2026-08-01）

| 组件 | 版本/路径 |
|------|-----------|
| Go | 1.26.5（`D:\AI\tianxuanX\tools\go\bin`） |
| Wails CLI | v2.13.0（`C:\Users\Administrator\go\bin`） |
| Node | v26.5.1（`D:\AI\tianxuanX\tools\node`） |
| pnpm | v11.9.0（tools/node） |

### 构建命令

```
cd D:\AI\tianxuanX && build-desktop.bat
```

（V10.138 起脚本自动补齐 PATH，无需手动设置环境。）
