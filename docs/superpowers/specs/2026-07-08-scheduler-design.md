# tianxuan 定时任务系统 — 设计文档

> 日期: 2026-07-08 | 状态: 设计完成 | 版本: V1.0

---

## 一、需求来源与市场定位

2025–2026 年，"定时 AI Agent 自动执行代码"已成为头部产品标配。Claude Code（Routines + Desktop + /loop 三层）、GitHub Copilot（Automations + CLI /every）、Cursor（Automations）均提供定时调度能力。

tianxuan 作为本地 AI 编程助手，采用进程中调度器方案，对标 Claude Code Desktop Scheduled Tasks。功能边界：**定时触发 AI agent（执行者模型）自动执行编码任务**。

---

## 二、核心决策

| 决策项 | 选择 |
|--------|------|
| 执行内容 | 触发 AI agent（跳过规划者，直接用执行者模型） |
| 时间定义 | 预设频率：hourly / daily / weekly + 时间点 |
| 执行上下文 | 每个任务绑定工作目录 + 可选环境变量 |
| 结果查看 | 独立的侧边栏管理面板，设置按钮上方 |
| 错过处理 | 不补跑，跳过 |
| 存储范围 | 全局 + 工作区双层 |
| 自动权限 | 无额外限制，信任 agent |
| 架构 | 进程内调度器（方案 A），goroutine + time.Ticker |
| 系统托盘 | 扩展菜单：任务状态 + 暂停/恢复/立即执行 |
| 桌面通知 | 开始执行 / 完成 / 失败 三级通知 |

---

## 三、数据模型

### 3.1 Schedule — 定时任务定义

```go
type Schedule struct {
    ID        string            `json:"id"`        // UUID
    Name      string            `json:"name"`      // 显示名称
    Prompt    string            `json:"prompt"`    // 发给 AI 执行者的任务描述
    Frequency string            `json:"frequency"` // "hourly" | "daily" | "weekly"
    Time      string            `json:"time"`      // "08:00"，hourly 时为空
    DayOfWeek int               `json:"dayOfWeek"` // 0=Sun,1=Mon... weekly 专用，非 weekly 时 -1
    WorkDir   string            `json:"workDir"`   // 工作目录（绝对路径）
    Env       map[string]string `json:"env,omitempty"`
    Enabled   bool              `json:"enabled"`
    CreatedAt int64             `json:"createdAt"` // Unix timestamp
    LastRunAt int64             `json:"lastRunAt"` // 0=从未执行
    Scope     string            `json:"scope"`     // "global" | "workspace"
}
```

### 3.2 ScheduleResult — 执行结果记录

```go
type ScheduleResult struct {
    ID          string `json:"id"`          // UUID
    ScheduleID  string `json:"scheduleId"`  // 关联 Schedule
    ExecutedAt  int64  `json:"executedAt"`  // Unix timestamp
    Success     bool   `json:"success"`     // 是否成功完成
    Summary     string `json:"summary"`     // AI 生成的执行摘要（一行）
    SessionFile string `json:"sessionFile"` // 归档 JSONL 文件路径
    Duration    int64  `json:"duration"`    // 执行耗时（毫秒）
}
```

每个 Schedule 最多保留 20 条 Result，超出删除最旧的。

---

## 四、调度器架构

### 4.1 进程内调度器

在 tianxuan 桌面进程内运行 goroutine 调度器，生命周期 = App 进程。系统托盘常驻保证进程不退出。

```
tianxuan 桌面进程
├── App (Wails bindings)
│   ├── GetSchedules / CreateSchedule / ...  ← 前端调用
│   └── GetResults(scheduleID)
│
├── Scheduler (internal/schedule/)
│   ├── Store: schedules.json + results.json 加载/保存
│   ├── Loop:  time.Ticker(1s) → checkDue() → fire()
│   └── fire(): 创建独立 Session → executor.Run(ctx, prompt) → 记录 + 通知
│
├── 系统托盘（扩展）
│   └── "定时任务 (n/m)" → 暂停全部 / 恢复全部 / 立即执行全部
│
└── executor (AgentRunner) ← Scheduler 复用执行者 provider + registry
```

### 4.2 fire() 执行流程

```
fire(schedule):
  1. os.Chdir(schedule.WorkDir)          → 切换工作目录
  2. 设置 schedule.Env 环境变量           → os.Setenv (fire 结束后恢复)
  3. 创建新 agent.Session               → 空对话，仅含 system prompt + L2 runtime
  4. 创建执行者 AgentRunner              → 复用 execProv + reg
                                        → Options{PlannerMode: true} 跳过规划者逻辑
  5. runner.Run(ctx, schedule.Prompt)    → 阻塞直到完成或超时
  6. TurnResult.Success → 记录 ScheduleResult
  7. emit 桌面通知: success/failure       → 复用 internal/notify
  8. 恢复原始工作目录 + 环境变量
```

### 4.3 并发隔离

- 每次 fire() 创建独立的 Session + AgentRunner，不污染用户当前活跃会话
- 定时任务执行与用户手动对话可并行
- ticker 检查粒度 1 秒，无竞态

### 4.4 跳过规划者

定时任务是预定好的任务，直接发给执行者（Hephaestus），跳过 Hermes 规划流程。实现方式：

- 创建 AgentRunner 时设置 `Options{PlannerMode: true}`，跳过规划者专属逻辑
- Scheduler 不经过 Hermes.Run()，直接调用 `executor.Run(ctx, prompt)`

### 4.5 超时控制

每个定时任务有 30 分钟超时（可配置），超时后强制取消 context 并记录为失败。

---

## 五、存储与持久化

### 5.1 文件布局

```
~/.config/tianxuan/                   ← 全局（MemoryUserDir）
├── schedules.json                    ← 全局定时任务列表
├── schedule-results.json             ← 全局任务的执行记录

<workspace>/.tianxuan/                ← 工作区级
├── schedules.json                    ← 工作区定时任务列表
├── schedule-results.json             ← 工作区任务的执行记录

<workspace>/.tianxuan/archive/        ← 已有，定时任务会话归档于此
└── schedule-<scheduleID>-<timestamp>.jsonl
```

### 5.2 文件格式

`schedules.json` — Schedule 数组
`schedule-results.json` — `map[scheduleID][]ScheduleResult`，每个 schedule 最多 20 条

### 5.3 原子写入

沿用现有 `saveAtomically` 模式（temp file + rename），避免写一半崩溃。

### 5.4 启动加载

1. `App.startup()` 中加载全局 schedules + results
2. 若有当前 workspace，加载 workspace 级 schedules + results
3. 合并 → 传给 Scheduler 启动 ticker
4. 工作区切换时重新加载工作区级任务（全局任务不变）

---

## 六、系统托盘扩展

```
┌──────────────────┐
│ 显示 tianxuan    │
├──────────────────┤
│ 定时任务 (1/3)   │  ← "已启用数/总数"
│ ├─ 立即执行全部  │
│ ├─ 暂停全部      │
│ └─ 恢复全部      │
├──────────────────┤
│ 退出             │
└──────────────────┘
```

桌面通知（复用 `internal/notify`）：
- 开始: `🔵 定时任务: {name} 正在执行...`
- 成功: `✅ {name} 完成 — {summary}`
- 失败: `❌ {name} 失败 — {error}`

---

## 七、前端管理面板

### 7.1 入口

侧边栏新增 Tab：**🗓️ 定时任务**（位于设置按钮上方），点击打开 SchedulePanel。

### 7.2 布局

```
┌─────────────────────────────────┐
│  🗓️ 定时任务           [+ 新建] │
├─────────────────────────────────┤
│  ● 全局任务 (1)                  │
│  ┌─────────────────────────┐    │
│  │ 🟢 每日代码审查         ⚙️   │  ← 状态灯 + 名称 + 操作
│  │ daily · 08:00            │    │  ← 频率 + 时间
│  │ 上次: 07/08 08:00 ✅     │    │  ← 上次执行
│  │ 共 15 次 · 2 失败        │    │  ← 统计
│  └─────────────────────────┘    │
│  ● 当前工作区 (/home/...)       │
│  ┌─────────────────────────┐    │
│  │ ⏸ CI 状态检查           ⚙️  │
│  │ weekly · 周一 09:00       │    │
│  │ 已暂停                   │    │
│  └─────────────────────────┘    │
└─────────────────────────────────┘
```

### 7.3 状态灯

- 🟢 已启用 + 上次成功
- 🟡 已启用 + 上次失败
- 🔵 已启用 + 从未执行
- ⏸ 已暂停

### 7.4 交互

- 点击任务卡片 → 展开执行历史（时间/成功/失败/摘要/查看会话）
- 新建 → 表单：名称、Prompt、频率、时间、工作区、环境变量
- ⚙️ → 编辑/删除/立即执行
- 状态灯点击 → 切换启用/禁用

### 7.5 Wails Bindings

```
App.GetSchedules()          → []ScheduleView
App.CreateSchedule(s)       → ScheduleView
App.UpdateSchedule(s)       → ScheduleView
App.DeleteSchedule(id)      → ok
App.ToggleSchedule(id, on)  → ok
App.RunScheduleNow(id)      → ScheduleResult
App.GetResults(scheduleID)  → []ScheduleResult
```

---

## 八、实现模块清单

| 模块 | 新增/修改 | 位置 | 职责 |
|------|----------|------|------|
| `internal/schedule/` | **新增** | schedule.go, store.go, scheduler.go, runner.go | 数据模型 + 持久化 + 调度循环 + 执行桥接 |
| `internal/boot/boot.go` | 修改 | 暴露 executor/provider/registry | 供 Scheduler 复用 |
| `desktop/app.go` | 修改 | 新增 Wails bindings + Scheduler 生命周期 | 前端接口 |
| `desktop/tray.go` | 修改 | 扩展托盘菜单 | 快捷控制 |
| `desktop/frontend/SchedulePanel.tsx` | **新增** | 任务管理面板组件 | 前端 UI |
| `desktop/frontend/Sidebar.tsx` | 修改 | 新增 Tab | 入口 |

---

## 九、测试策略

- `internal/schedule/scheduler_test.go` — 调度逻辑单元测试（checkDue、ticker 精度）
- `internal/schedule/store_test.go` — 读写 + 原子性 + 上限截断测试
- `desktop/app_schedule_test.go` — Wails binding 集成测试
- E2E：创建任务 → 等待执行 → 验证通知 + 会话归档 → 面板显示结果

---

## 十、风险与限制

- 电脑休眠/关机错过执行：不补跑（用户已确认）
- 定时任务执行期间用户切换工作区：不影响，每次 fire() 独立切换目录
- 大量任务同时到期：目前串行执行（后续可优化为并发 + 队列）
- 长期运行可能内存增长：每个 fire() 创建的 Session 独立，执行完即释放
