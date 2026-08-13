# 下一阶段优化规划（2026-08-13）

承接 V10.178.0 工程治理轮次，剩余结构性债务按风险递增排序。

## P1 低风险快见效

### 1. lint 进 CI
- .golangci.yml 已配置 staticcheck/unused/errcheck/ineffassign，new-from-rev 已修正为 main，但 ci.yml 无 lint 步骤
- 方案：ci.yml 加 golangci-lint 步骤（new-from-rev: main 只报新问题）
- 风险：低

### 2. 前端测试进 CI
- vitest 187 例全绿但只本地跑，desktop 独立 Go module 也有测试
- 方案：ci.yml 加 node 22 + pnpm + pnpm test，desktop module go test 也加入
- 风险：低

### 3. 测试时长优化分层
- builtin 单包 47s、boot 7.5s、hook 6.5s，全量约 1 分钟
- 方案：拆短测试（默认跑）和慢测试（tag/夜间跑）
- 风险：低

## P2 中风险需逐块理解

### 4. chat_tui.go 剩余拆分（1824 行）
- 已拆 render.go，待拆 paste.go（约 288 行）、slash.go（约 220 行）、statusline.go（约 100 行）
- 难点：类型定义与函数交织，需按函数边界切分
- 风险：中

### 5. 800+ 行文件拆分
- serve_handlers.go(969)、boot.go(960)、agent.go(872)、controller.go(851)、config.go(831)、cli.go(830)
- 逐个理解边界再拆，禁止机械移动
- 风险：中高

## P3 高风险涉及缓存铁律

### 6. 6 个 alias 兼容层清理
- toolguard/cache/render/session/budget/textutils 的 re-export
- 核心约束：移动符号若影响工具 schema 序列化会破坏 DeepSeek 前缀缓存 L3，清理前必须先做符号级依赖梳理 + 缓存命中率回归
- 风险：高

### 7. 缓存命中率基准自动化
- benchmarks 数据停在 V4.0/V5.x，README 的 90%+ 无法证伪
- 方案：抽最小 cache-hit benchmark，每次发布记录趋势
- 风险：中

## P4 收尾杂项

### 8. ISSUE_TEMPLATE v1/v2 彻底清理
- version-line dropdown 仍留 v1 选项，删 dropdown + 删 label workflow 的 v1 分支

### 9. 剩余静默吞错保持现状
- 62 处已修 2 处真实 bug，其余为有意尽力而为（os.Remove 清理/杀进程/探测解析），建议不改

## 顺序建议

先 P1（lint + 前端测试 + 测试分层，纯 CI 配置无缓存风险），再 P2（拆文件纯移动），最后 P3（alias + 缓存基准需回归验证）。
