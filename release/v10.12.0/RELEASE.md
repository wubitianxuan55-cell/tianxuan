# V10.12.0 发布记录

**发布日期**: 2026-06-29
**构建产物**: `release/v10.12.0/tianxuan-desktop.exe`
**基于**: V10.11.0

## 核心变更

### 1. Bug 修复
- **session_route_features: FilesModified 永久为 0** — 修复 Pro 模型自动升级失效

### 2. 流式输出流畅度（三层优化）
- 前端: text/reasoning 事件用 setTimeout(0) 绕过 React 18 批处理
- 后端: stream_batcher 换行感知 flush
- Store: useItems() 分离订阅，Transcript 直连，App 不全局重渲染
- CSS: 流式禁用 msg-fade-in，contain:layout 防抖动

### 3. auto_router 增强
- HasWrittenFiles / HasUsedSubAgent 信号检测
- TurnCount 阈值 10→5
- 关键词外部化 Options.RouterKeywords

### 4. grep 增强
- context_lines 上下文行 + highlight >>>匹配<<< 高亮

### 5. Bash 输出智能截断
- JSON 模式 stdout/stderr 独立截断 + truncated 标志
- bash_output tail_lines 参数

### 6. 代码整理
- 8 个未跟踪文件纳入 git + release/ 冗余清理

### 7. 测试（新增 58 用例）
- session_route_features 20 + tool_coherence 10 + tool_precheck 12 + grep 8 + bash truncate
- agent 包总计 242 测试用例
