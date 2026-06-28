# V10.10.0 项目基准

**版本**: V10.10.0
**发布日期**: 2026-06-29
**构建产物**: `release/v10.10.0/tianxuan-desktop.exe` (16MB)
**SHA256**: `ebdd496cb4d87d9e7ab73847bae46b5e87abdf6fe4889053d59eb78bc7198c47`
**上一版本**: V10.9.0

---

## 构建命令

```bash
# 桌面端（必须用 wails build，禁止 go build）
cd tianxuan/desktop
wails build -o release/v10.10.0/tianxuan-desktop.exe

# 前端开发
cd tianxuan/desktop/frontend
pnpm dev

# 测试
cd tianxuan/desktop && go test ./...
cd tianxuan/internal/agent && go test -count=1 -run "TestCompact|TestKeep" ./...
```

## 架构变化

### V10.10.0 新增

| 模块 | 文件 | 说明 |
|------|------|------|
| UpdateFact | `controller_memory.go` + `app_meta.go` | 记忆编辑后端链路 |
| KeepPolicy | `agent.go` | 新增 `KeepProtected` 标志位 |
| protectedTools | `compact.go` | 压缩时保护 read_skill/memory_search/remember 输出 |
| ToolContext | `tool/tool.go` | ToolContext 结构体 + ContextualTool 接口 |
| wrapTaskResult | `agent/task.go` | 子代理 `<task-result>` XML 包装 |
| saveAtomically | `sessions.go` | 原子写入共享函数 |

### V10.10.0 删除

| 模块 | 说明 |
|------|------|
| `WorkspaceTab.saveCond/Mu/saving/saveAgain` | 死代码——全项目零调用 |
| `resolveSessionDisplay` | 死函数——零非测试调用方 |
| `MemoryView.suggestions` | 死字段——从未填充/读取 |

## V10.9.0 保留的架构

- TCCA 四层缓存架构 (Identity → Runtime → Context → Flow)
- 3 种 Agent 模式 (explore/develop/orchestrate)
- 多标签页骨架 (WorkspaceTab + tabEventSink)
- 记忆建议引擎 (memory_suggestions.go)
- 技能系统 (skill.Store)
- MCP 集成 (mcp__ 前缀工具)
