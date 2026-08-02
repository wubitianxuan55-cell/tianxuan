---
name: v10-150-release-notes
title: V10.150.0 发布记录
description: V10.150.0 发布记录 — OpenClaw Model Failover 蒸馏（turn-local 模型回退链）+ Reasonix Auto Failure Guard + 桌面端发布
---

## V10.150.0 发布记录

| 项目 | 值 |
|------|-----|
| 版本号 | V10.150.0 |
| 发布日期 | 2026-08-02 |
| 代码变更 | 14 文件（agent/provider/boot/config + 新包 failover + 测试），+~340 行 |
| 主题 | 后端蒸馏两轮（Auto Failure Guard + Model Failover）+ 桌面端发布 |
| 构建产物 | `release/v10.150.0/tianxuan-desktop.exe` |
| 文件大小 | 24,997,888 bytes (~23.8 MB) |
| SHA256 | `c9e787b049a044a52c96b3b42a4d216a662ce29c1939267edfa50028e58b00e4` |

### 本次发布包含的改动
| 模块 | 主题 |
|------|------|
| 失败升级（V10.149，Reasonix 蒸馏） | Auto Failure Guard：宿主侧失败升级决策 — 精确操作指纹（工具+参数 SHA-256）、同操作失败 3 次宿主停操作、回合累计 6 次失败停变更/验证只留只读诊断、成功变更清零回合预算、只读/blocked 失败不累计 |
| 模型回退（V10.150，OpenClaw 蒸馏） | Model Failover：turn-local 模型回退链 — 429/408/5xx/断网时按序切备用模型（同 key 不同 model）、整链纯过载指数退避重试、FallbackSummaryError 逐候选细节、OnSwitch 通知；参数错误/auth/取消/流中断不切候选 |
| 基础设施 | provider.HTTPStatusCoder 接口（AuthError + httpStatusError 实现），供 failover/诊断复用 |
| 配置 | `[[providers]] fallbacks = [...]`；空/重复自动跳过；无 fallback 零开销（行为不变） |
| 桌面端 | 重新构建发布（前端 128 测试 + 后端全量测试通过） |

### 关键决策（5 级追溯）

- 蒸馏选型：对比 Reasonix/Kun/OpenClaw 后端后，先做"宿主侧失败升级"（不再只靠 nudge 提示模型自改），再做"跨模型回退"（补齐同 provider 重试之外的缺口）——都直接提升可靠性，且不触碰 L1-L4 缓存前缀
- Model Failover 缓存安全：回退 turn-local（只当前回合生效，下回合回到 primary），正常路径缓存前缀完全不变
- Chain 实现 provider.Provider 接口：agent 层零改动，boot 构造时透明包装
- auth 失败不切候选：同一 key 下切换模型无法修复鉴权，直接呈现 AuthError 避免烧备用候选

### 验证

- `go test ./...` 全绿（failover 11 例 + boot 接线 3 例 + 全量回归）
- 前端 `vitest run` 128/128 通过；`tsc --noEmit` 0 错误（wails build 内跑通）
- `wails build -ldflags "-X main.version=v10.150.0"` EXIT 0（25s）
- SHA256 归档前后一致
