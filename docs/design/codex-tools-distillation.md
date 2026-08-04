# Codex CLI 工具设计蒸馏报告

> 目标：降低 tianxuan 工具调用出错率。
> 参照源：OpenAI Codex CLI 开源版（`main` 分支，commit `6d4d944`，2026-08-04 克隆至 `D:\AI\refs\codex`）。
> 本文只做分析与方案设计；具体代码改动需另行批准后实施。

## 1. 结论速览

tianxuan 的执行防护层（precheck / stale-anchor guard / failure-guard / tool-feedback / 错误模式学习）已经相当完善，与 codex 相比**差距不在"事后防御"，而在"事前信息"与"事中校验"**：

1. **参数语义描述丢失**：CompactDescriptor 无条件生效，模型看到的 schema 全部是"无参数描述"的裸 JSON，只能猜参数含义 —— 这是参数类错误的首要根因。
2. **参数解析不一致**：30+ 个内置工具用宽松 `json.Unmarshal`（未知字段静默忽略、类型不严格），只有 `bash` 用 `DisallowUnknownFields` 严格模式。模型给了错误字段/类型时，工具拿到零值后才报"path is required"这类二次错误，而非直接的 schema 错误。
3. **无 schema 层统一校验**：参数校验逻辑散落在每个工具的 `Execute` 里，重复且标准不一；codex 由 27KB 的 `json_schema.rs` 在工具执行前统一校验。
4. **无工具级错误统计**：`auditFunc` 只做日志回调，没有落盘的结构化错误率数据，无法量化"哪个工具、哪种错误最频繁"，也就无法针对性优化。
5. **编辑工具模型不同**：codex 的 `apply_patch` 是 FREEFORM 工具 + Lark 语法（模型直接输出补丁文本，不经 JSON 包裹），从根本上消灭了 JSON 转义/结构错误；tianxuan 的 `edit_file`/`multi_edit` 走 JSON 参数，`old_string` 唯一性/存在性错误仍是最高频失败模式。

## 2. Codex 工具设计核心机制（防错视角）

### 2.1 工具定义：Freeform + 严格 JSON Schema 双轨

codex 的 `ToolSpec`（`codex-rs/tools/src/tool_spec.rs`）支持五种形态：

| 形态 | 用途 | 防错价值 |
|------|------|----------|
| `Function` | 常规 JSON 参数工具（`exec_command`、`write_stdin` 等） | 参数由 JSON Schema 约束 |
| `Freeform` | 文本协议工具（`apply_patch`） | 模型直接输出补丁文本，无 JSON 包裹/转义 |
| `Namespace` | 工具命名空间（MCP） | 名字冲突隔离 |
| `ToolSearch` | 延迟加载的工具发现 | 模型按需发现工具，减少首屏认知负担 |
| `WebSearch` | 平台原生搜索 | 不消耗 tool-call 配额 |

`apply_patch` 用 Lark 语法定义补丁格式（`apply_patch.lark`）：

```
start: begin_patch hunk+ end_patch
hunk: add_hunk | delete_hunk | update_hunk
update_hunk: "*** Update File: " filename LF change_move? change?
change: (change_context | change_line)+ eof_line?
change_line: ("+" | "-" | " ") /(.*)/ LF
```

解析器（`StreamingPatchParser`）在模型生成参数时**流式解析**（`ToolArgumentDiffConsumer`），边生成边校验，非法格式立即报错。这比"生成完整 JSON 后一次性反序列化"更早暴露错误。

### 2.2 参数 Schema：完整描述 + 严格约束

codex 的 `JsonSchema`（`json_schema.rs`，27KB + 64KB 测试）支持 `type/required/enum/anyOf/oneOf/items/additionalProperties/$defs`，且**每个参数都带语义描述**，例如 `exec_command` 的 `workdir`：

> "Working directory for the command. Defaults to the turn cwd."

`yield_time_ms` 还带有效范围：

> "Wait before yielding output. Defaults to 10000 ms; effective range is 250-30000 ms."

Windows 平台还有专门的安全指导注入工具描述（`windows_shell_guidance`）。模型在调用前就能看到：参数语义、默认值、有效范围、平台差异 —— 出错空间被大幅压缩。

### 2.3 统一错误与输出契约

- `FunctionCallError` 只有两态：`RespondToModel(String)`（可恢复，反馈给模型修复）与 `Fatal(String)`（不可恢复）。所有工具错误**必须**反馈模型，禁止静默吞掉。
- `ToolOutput` trait 统一输出：`success_for_logging()` 显式标记成败；`log_preview()` 安全截断（2KiB/64 行）供遥测。
- `format_exec_output_for_model` 统一命令输出格式：`Exit code` / `Wall time` / `Total output lines` / `Output`（带截断策略），模型容易解析。

### 2.4 全链路工具追踪

`ToolDispatchTrace`（`tool_dispatch_trace.rs`）为每个工具调用记录：线程、turn、call_id、工具名、请求方（模型/Code Mode）、完整参数、结果 payload、成败状态。配合遥测可以量化每个工具的错误率 —— 这是"降低出错率"的前提：**先能度量，才能优化**。

## 3. tianxuan 现状对比与差距（带证据）

### 3.1 已具备（无需重复建设）

| 机制 | 位置 | 说明 |
|------|------|------|
| 统一结果信封 | `internal/tool/envelope.go` | `ToolEnvelope{ok, success, code, error, message, data}`，等价 codex `ToolOutput` |
| 执行前预检 | `internal/agent/tool_precheck.go` | 确定性预检（old_string/anchor 存在性） |
| stale-anchor 守卫 | `internal/agent/execute_one.go` | 同一轮已写文件必须先 read 再 edit |
| Auto Failure Guard | `internal/agent/failure_guard.go` | 失败升级与阻断 |
| 工具失败反馈 | `internal/agent/tool_feedback.go` | 连续全败注入 STOP 引导 |
| 跨会话错误学习 | `internal/learning/patterns.go` | 常见错误模式分类与注入 |
| 错误信息质量 | `internal/tool/builtin/encoding_helpers.go` | old_string not found 带最近行/匹配行号提示 |
| 工具 Kind 分类 | `internal/tool/tool.go` | Read/Edit/Write/Delete/... 策略门控 |

### 3.2 关键差距（出错率根因）

#### 差距 1：模型看不到参数语义（P0）

证据链：

1. `internal/tool/builtin/compact.go` —— `compactSchema` 全部是**无 `description` 的裸 schema**；
2. `internal/tool/tool.go:FilteredSchemas` —— 只要工具实现 `CompactDescriptor` 就**无条件**用 compact 版本（`compact_wiring_test.go` 锁定所有内置工具都实现了）；
3. `internal/provider/schema_canonicalize.go:compressSchema` —— 连完整 schema 的 `description` 也会被剥离；
4. `internal/tool/builtin/desc_language_test.go` —— compact 描述被限定为 15-25 字中文单行。

结果：模型看到的是 `{"type":"string"}` 这类参数，只能靠猜。对比 codex 的 `"Defaults to the turn cwd."`，差距一目了然。

#### 差距 2：参数解析不一致（P0）

`internal/tool/builtin/bash.go` 的 `decodeStrictArgs`（`DisallowUnknownFields`）只有 bash 使用；其余 30+ 个工具（edit_file、write_file、grep、glob、read_file、git_*、todo、complete_step、web_* 等，见 `grep -rn "json.Unmarshal(args"`）全部是宽松模式。模型传了 schema 外的字段（如给 `edit_file` 传 `timeout`）会被静默丢弃，直到后续业务校验才暴露。

#### 差距 3：无 schema 层统一校验（P1）

codex 在 `parse_arguments` 处统一按 schema 校验；tianxuan 的每个工具各自 `json.Unmarshal` + 手写校验，标准不一（有的查空字符串、有的查类型、有的不查），漏检场景多。

#### 差距 4：无工具错误统计（P1）

`execute_one.go` 的 `auditFunc` 只做内存回调，未落盘；`learning/patterns.go` 的 TOML 只存"模式"不存"比率"。无法回答"上周 edit_file 失败率是多少、主要是什么错误"。

#### 差距 5：编辑工具模型差异（P2，架构级）

codex 用 freeform patch 消灭 JSON 编辑错误；tianxuan 的 `edit_file`/`multi_edit` 仍依赖模型精确重现 `old_string`（大小写、空白、CRLF、制表符），这是编辑类错误的主要来源。tianxuan 已有 `edit_lines`（按行号 + 锚点）作为部分缓解，但模型仍频繁先选 `edit_file`。

## 4. 蒸馏实施建议

### P0（低风险高收益，可直接实施）

| # | 改动 | 涉及文件 | 状态 |
|---|------|----------|------|
| 1 | 执行前 schema 统一校验（必填/类型/枚举大声失败 + 别名兼容，对齐 codex `json_schema.rs`） | `internal/tool/validate.go`（新增）、`internal/agent/execute_one.go` | ✅ 已实施（V10.154） |
| 2 | CompactSchema 保留关键参数描述（默认值/枚举语义/单位；`CanonicalizeSchemaVerbose` 保留描述） | `internal/tool/builtin/compact.go`、`internal/provider/schema_canonicalize.go`、`internal/tool/tool.go` | ✅ 已实施（V10.154） |
| 3 | 工具错误统计落盘（tool/error_kind/count/last_seen）+ `tianxuan tools stats` CLI | `internal/tool/stats.go`（新增）、`internal/agent/batch_executor.go`、`internal/boot/boot.go`、`internal/cli/tools.go` | ✅ 已实施（V10.154） |

### P1（中等改动）

| # | 改动 | 说明 |
|---|------|------|
| 4 | 建立统一 `parse_arguments` 辅助：schema 校验 + 严格解码 + 错误信息附带 schema（对齐 codex） | ✅ `validate.go` 已实现执行前校验（V10.154） |
| 5 | 错误反馈统一：所有工具错误走 `WrapError`，禁止裸 error 字符串进入模型上下文 | ✅ executeOne 统一包装；审计通过 |
| 6 | 编辑工具错误反馈对齐 codex：`old_string not found` 附带恢复 hint | ✅ edit_file 已有；delete_range/delete_symbol 已补齐（V10.154） |
| 7 | 修复 learning 链路：`Extract` 返回的 pattern 从未 merge，计数永不增长 → `Observe`（提取+合并+持久化） | ✅ 已修复（V10.154） |
| 8 | validation_error 通配分类：校验器新错误类型可被学习并注入系统提示 | ✅ 已实现（V10.154） |

### P2（架构级，需评估）

| # | 改动 | 说明 |
|---|------|------|
| 7 | 评估 freeform 编辑工具（Lark 语法）替代 JSON old_string/new_string | 需要权衡：与现有多工具 schema 兼容性、解析器开发成本、CRLF/模糊匹配能力 |
| 8 | 引入 ToolDispatchTrace 等价物：每个工具调用的参数/结果/成败结构化落盘 | 为后续错误率仪表和针对性优化提供数据底座 |

## 5. 建议实施顺序

1. 先做 P0-1（统一严格解析）：改动面小、测试覆盖已有、能直接消灭"静默吞参数"类错误。
2. 再做 P0-3（错误统计）：用真实数据验证根因排序，决定后续投入方向。
3. 然后 P0-2（描述保留）：根据统计数据判断参数语义缺失的实际影响面。
4. P1/P2 视数据再定优先级。

## 6. 附：参照源信息

- 仓库：`git@github.com:openai/codex.git`（国内环境经 `ghfast.top` 镜像克隆）
- 本地路径：`D:\AI\refs\codex`
- 版本：`main` @ `6d4d944` "Support leaf models in multi-agent v2 (#36892)"
- 关键文件：
  - `codex-rs/tools/src/tool_spec.rs`（工具形态）
  - `codex-rs/tools/src/json_schema.rs`（schema 校验）
  - `codex-rs/core/src/tools/registry.rs`（注册与分发）
  - `codex-rs/core/src/tools/tool_dispatch_trace.rs`（调用追踪）
  - `codex-rs/core/src/tools/handlers/apply_patch*.rs` + `apply_patch.lark`（freeform 编辑）
  - `codex-rs/core/src/tools/handlers/shell_spec.rs`（工具描述范例）
