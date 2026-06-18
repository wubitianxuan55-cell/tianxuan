# Project memory

## Notes

- 思考输出说中文
-记得使用skill
- 2026-06-13: V7.2.0 发布 — DSR + 三闸门 + 归档 + 6 Bug 修复

## 🔴 核心约束：禁止损害缓存命中率

DeepSeek 前缀缓存是项目成本命脉。**任何修改如果会导致以下情况，绝对不允许：**

1. **L1 Identity 字节变化** — 系统提示词任何字符不可改。由 `verifyPrefix` SHA-256 守卫（漂移 → panic）
2. **tools 列表 session 中途变化** — 不能按输入动态增删工具。V8.0.2 `filteredSchemas` 致命事故：命中率从 99% 降到 0%
3. **L2 Runtime 首轮后变化** — 运行时上下文锁定后不可变
4. **动态系统提示词注入** — 不能在 user 消息前插入可变文本
5. **工具描述热更新** — session 中途不能改 CompactDescriptor

原理：DeepSeek prefix cache 按"前缀连续匹配"计费。1 字节差异 = 整轮 cache miss = 2.5 倍费用。

修改涉及以下任何文件/机制前，必须先确认不会破坏前缀不变性：
- `internal/cache/` 四域管理
- `internal/context/` TCCA 内核
- `internal/agent/agent.go` 中的 `filteredSchemas`/`activeSchemas`/`verifyPrefix`
- `internal/boot/boot.go` 中的系统提示词构建
- 工具注册表 (`internal/tool/`) 的热加载路径
