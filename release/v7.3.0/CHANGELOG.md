## [7.3.0] — 2026-06-14

### 桌面端修复

- **统计面板布局恢复**: 按 V7.2 规格重排 StatsPanel，顺序：上下文→会话→本轮→当前步→命中率趋势→Token累计图→工具统计
- **Token 累计图修复**: 数据源从按步改为按轮累计，正确反映轮次间 token 增长
- **App.tsx TCCA 残留清除**: 移除已删除 TCCA 面板的 `tcca={state.tcca}` prop 传递，修复 tsc 编译错误导致 wails build 使用旧 dist 的问题

### 核心清理

- **删除「前缀挥发性检测」模块** (`prefix_volatility.go`): 只检测不修复，纯诊断噪音，零生产价值（V5.11 引入，V7.3 移除）
- **删除「压力冲刷」模块** (`pressure_flush.go`): 70% 压力注入存储提示，与 V7.1 轮内压缩策略矛盾（V6.0 引入，V7.3 移除）
- **删除「检查点重建」模块** (`rebuild.go`): `buildCheckpointRebuild` 整函数已成死代码（V7.0 引入，V7.3 移除）

### DSR 收敛

- **交换压缩 fallback 优先级**: Legacy Truncate 优先（保留 `[L1+prefix+summary+tail]` 结构，缓存只断一次），Budgeted Rebuild 降为后备
- **compactStuck 降级路径**: 不再静默返回，改为 force 模式纯截断，不注入 checkpoint/memory/tasks
- **删除 compaction digest marker**: 在摘要中写 SHA256 标记的唯一作用是证明"确实破坏了缓存"——移除

### 发布

- CLI: `bin/tianxuan.exe` (12.5 MB)
- 桌面端: `desktop/build/bin/tianxuan-desktop.exe` (15.7 MB, Wails v2.12.0)
