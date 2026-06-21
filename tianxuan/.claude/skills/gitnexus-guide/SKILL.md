---
name: gitnexus-guide
description: "当用户使用GitNexus代码知识图谱时使用——13个MCP工具完整参考"
---

# GitNexus 指南

GitNexus 是一个代码知识图谱 MCP 服务器，提供 13 个工具用于代码理解、影响分析和安全重构。

## 何时使用 GitNexus 工具

| 你的需求 | 直接用 MCP 工具 | 搭配的内置技能 |
|---------|----------------|---------------|
| 理解代码架构 / "X 怎么工作" | `mcp__gitnexus__query` + `mcp__gitnexus__context` | `explore` (子代理深度探索) |
| 改代码前查影响范围 | `mcp__gitnexus__impact` | — |
| 调试 / "为什么 X 失败" | `mcp__gitnexus__query` + `mcp__gitnexus__detect_changes` | `debug` (内置4步调试法) |
| 重命名/重构 | `mcp__gitnexus__rename` | — |
| 了解可用工具和 schema | 本文档 | — |

## 工具速查表

| 工具 | 用途 |
|-----|------|
| `mcp__gitnexus__query` | 概念搜索 → 返回关联的执行流程 |
| `mcp__gitnexus__context` | 360° 符号视图：调用者/被调用者/所属流程 |
| `mcp__gitnexus__impact` | 符号变更影响范围 (d=1 直接破坏 / d=2 可能影响 / d=3 需测试) |
| `mcp__gitnexus__detect_changes` | Git diff → 受影响流程 (pre-commit 检查) |
| `mcp__gitnexus__rename` | 多文件协调重命名 (图关系 + 文本搜索双重确认) |
| `mcp__gitnexus__cypher` | 原始图查询 (先读 `gitnexus://repo/{name}/schema`) |
| `mcp__gitnexus__list_repos` | 列出已索引仓库 |
| `mcp__gitnexus__api_impact` | API 路由变更影响：消费者/响应字段/中间件 |
| `mcp__gitnexus__route_map` | API 路由映射：处理器 ↔ 消费者 |
| `mcp__gitnexus__shape_check` | API 响应形状 vs 消费者字段访问比对 |
| `mcp__gitnexus__tool_map` | MCP/RPC 工具定义与处理器 |
| `mcp__gitnexus__group_list` | 列出仓库组 |
| `mcp__gitnexus__group_sync` | 同步仓库组的 Contract Registry |

## 资源 URI 参考

| 资源 | 内容 |
|-----|------|
| `gitnexus://repo/{name}/context` | 索引统计、新鲜度检查 |
| `gitnexus://repo/{name}/clusters` | 所有功能区域 + 内聚分数 |
| `gitnexus://repo/{name}/cluster/{name}` | 区域成员列表 |
| `gitnexus://repo/{name}/processes` | 所有执行流程 |
| `gitnexus://repo/{name}/process/{name}` | 逐步执行轨迹 |
| `gitnexus://repo/{name}/schema` | Cypher 图 schema |

## 图 Schema

**节点:** File, Function, Class, Interface, Method, Community, Process
**边 (CodeRelation.type):** CALLS, IMPORTS, EXTENDS, IMPLEMENTS, DEFINES, MEMBER_OF, STEP_IN_PROCESS, HANDLES_ROUTE, HAS_METHOD, HAS_PROPERTY, ACCESSES

```cypher
-- 查找函数的所有调用者
MATCH (caller)-[:CodeRelation {type: 'CALLS'}]->(f:Function {name: "myFunc"})
RETURN caller.name, caller.filePath
```

## 快速入门

```
# 第一步：了解代码库状态
mcp__gitnexus__context (读 gitnexus://repo/{name}/context)

# 第二步：搜索相关代码
mcp__gitnexus__query(query="你的问题", goal="你想做什么")

# 第三步：深入具体符号
mcp__gitnexus__context(name="符号名", include_content=true)
```
