---
name: finish-development-branch
description: 实现完成、测试全绿后收尾开发分支 — 结构化决策（本地合并/推送建 PR/保留/丢弃）并安全清理 worktree。蒸馏自 superpowers v5.1.0 finishing-a-development-branch。
metadata:
  author: tianxuan (distilled from obra/superpowers v5.1.0)
  version: "1.0.0"
---

# 完成开发分支（finish-development-branch）

## 何时使用

- 计划全部完成、测试全部通过，准备把开发分支合入主线的收尾阶段
- worktree / 功能分支开发结束后的决策与清理

## 核心原则

**验证测试 → 检测环境 → 呈现选项 → 执行选择 → 归属式清理。**

测试未全绿禁止合并；未获得用户明确确认禁止删除任何工作；只清理自己创建的 worktree。

## 流程

### 1. 验证测试（前置门禁）

先用 verify_gate 或 bash 运行项目测试套件：

- Go：`go test ./...`
- Rust：`cargo test`
- Node/TS：`npm test`（或项目等效命令）

有失败 → **停下修复，禁止进入下一步**；修复后重新验证全绿才可继续。

### 2. 检测环境

用 `git_status` / `git_worktree list` 确认：

- 当前分支与基线分支（main / master / 远程默认分支）
- 是否位于 worktree 中、worktree 路径（归属判断决定清理权）
- 未提交改动、未推送提交

### 3. 呈现选项（结构化，不加废话）

常规仓库与具名分支的 worktree：

```
实现完成。接下来怎么处理？
1. 本地合并回 <基线分支>
2. 推送并创建 Pull Request
3. 保留分支现状（稍后自行处理）
4. 丢弃本次工作
```

detached HEAD（外部托管工作区）：只呈现 3 个选项（去掉本地合并），并说明工作区由外部托管。

### 4. 执行选择

- **本地合并**：先 cd 到主仓库根 → checkout 基线分支 → merge 开发分支 → **在合并结果上重跑测试** → 通过后才清理 worktree → `git branch -d` 删除分支
- **推送建 PR**：推送分支并创建 PR（gh pr create 或仓库托管流程）→ **保留 worktree**（用户要基于 PR 反馈继续迭代）
- **保留现状**：报告分支名与 worktree 路径 → 不清理
- **丢弃**：先明确告知将删除分支、全部提交与 worktree → 用户输入确认后才执行 → 清理 worktree → `git branch -D` 强制删除分支

### 5. 清理（仅本地合并与丢弃）

- **只清理自己创建的工作区**：worktree 由本 agent 的 `git_worktree add` 创建（或路径位于项目 worktrees 目录下）；外部托管 / 用户自建的工作区一律不删
- **先 cd 到主仓库根，再 `git worktree remove`**——在 worktree 内部执行会失败
- 顺序：先合并成功 → 移除 worktree → 再删分支；完成后 `git worktree prune` 自愈

## 红线

- 测试未全绿 → 禁止合并 / 建 PR
- 合并结果未验证 → 禁止删分支
- 丢弃 → 必须获得用户明确确认（不得直接删除）
- 不清理不是自己创建的 worktree（归属检查）
- 不在 worktree 内部执行 `git worktree remove`
- 禁止在未询问的情况下 force-push
