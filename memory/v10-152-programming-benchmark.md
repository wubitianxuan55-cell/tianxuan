---
name: v10-152-programming-benchmark
title: V10.152.0 编程能力实测基线
description: V10.152.0 编程能力实测基线 — 同任务对比 tianxuan/Codex，轮次/耗时/成本/缓存/行为
---

## V10.152.0 编程能力实测基线

本文档记录 V10.152.0 的编程能力实测结果，作为后续版本的对比基准。实测背景：用户反馈“相同模型下 tianxuan 比 Codex 差一大截”，经 7 阶段优化（提示词重构 -79%、回合减负、compact 工具集、描述英文化、单模型解除 complete_step 强制、multi_edit 恢复、测试先行规则确定化）后的验证。

### 实测方法

- **工具**：`tianxuan-bench.exe run "<task>"`（headless，DeepSeek deepseek-pro，max_steps=25）
- **环境**：Windows 下 Git Bash shell；项目目录 D:\AI\bench_compare（临时）
- **对比**：同任务在 D:\AI\bench_codex 用 Codex 完成（记录工具步数）

### 实测一：从零实现（CSV 行解析器）

任务：实现 ParseCSVLine（引号转义/引号内逗号/未闭合报错）+ 表驱动测试 + gofmt/test 验证。

| 指标 | tianxuan | Codex |
|------|---------|-------|
| 工具调用轮次 | 5-6 轮 | 4 步 |
| 总耗时 | 71 秒 | ~2 分钟 |
| 完成质量 | 6 测试 PASS + gofmt/vet/build | 6 测试 PASS + gofmt |
| 成本 | 约 ¥0.02 | — |
| 缓存命中率 | 首轮 6%，后续 97-99% | — |

### 实测二：修改存量代码/修 bug（引号后尾随字符）

任务：修复 ParseCSVLine 对 `"a"x` 静默忽略尾随字符的 bug，要求 test-first（先写失败测试→红灯→修复→绿灯）。

| 指标 | 值 |
|------|-----|
| 工具调用轮次 | ~9 轮（含调查/run_skill/验证） |
| 总耗时 | 203 秒 |
| 成本 | 约 ¥0.005（末轮 17436 tok） |
| 缓存命中率 | 98%（16768/17055） |
| 测试先行遵守 | ✅ 先加失败测试→确认红灯→修复→绿灯 |
| 质量 | 修复正确 + 7 组边界验证无回归 |

### 关键行为观察

1. **轮次与 Codex 基本持平**：从零实现 5-6 轮 vs Codex 4 步，约等；多出的 1-2 轮来自验证强迫（go vet/build）
2. **测试先行规则生效**：修改存量代码时严格“先写失败测试→红灯→修复→绿灯”；从零实现时合理豁免
3. **方法论按需加载**：修 bug 场景主动调用 `run_skill systematic-debugging`，而非每轮强制注入
4. **成本优势实打实**：完成一个真实编程任务 ≤ ¥0.02，缓存命中 97%+

### 复跑方法

```powershell
# 从零实现基准（之后可重复）
go build -o D:\AI\tianxuan-bench.exe ./cmd/tianxuan
$env:DEEPSEEK_API_KEY = '<key>'   # 或依赖 .env
cd D:\AI\bench_compare
D:\AI\tianxuan-bench.exe run (Get-Content task.txt -Raw)
# 修 bug 基准
D:\AI\tianxuan-bench.exe run (Get-Content task2.txt -Raw)
```

### 对比基准说明

- 后续版本变更后，在相同目录重跑上述两个任务，对比：轮次、耗时、成本、缓存命中率、测试先行遵守率
- 基准目录：D:\AI\bench_compare（任务文本 task.txt / task2.txt）；Codex 对比目录 D:\AI\bench_codex
- 注意：缓存命中率受 session 新旧影响，首轮必然 miss，对比后续轮次
