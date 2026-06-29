# V10.12.0 发布记录

**发布日期**: 2026-06-29
**构建产物**: `release/v10.12.0/tianxuan-desktop.exe`
**基于**: V10.11.0

**SHA256**: `832585cb1fb5c7a0981abacf34412d7c97a1515c177ba88d9471e6f43ec8aa48`

## 核心变更

### 1. Bug修复
- **session_route_features: FilesModified 永久为0** → Pro模型自动升级失效
  从 ToolCall.Arguments 提取路径，修复了从执行结果字符串提取的bug
- **回到底部按钮不可见** → 虚拟列表 scrollHeight 阈值过高 + 滚动不到位

### 2. 流式输出流畅度
- text/reasoning 事件 setTimeout(0) 绕过 React 18 批处理
- stream_batcher 换行感知 flush
- useItems() 分离订阅，Transcript 直连

### 3. auto_router 增强
- HasWrittenFiles / HasUsedSubAgent 检测
- TurnCount 阈值 10→5
- Options.RouterKeywords 外部化

### 4. grep 增强
- context_lines 上下文行 + >>>highlight<<< 匹配高亮

### 5. Bash 输出智能截断
- JSON 模式 stdout/stderr 独立截断(~24KB) + truncated 标志
- bash_output tail_lines 参数(1..500)

### 6. 配色系统重设计
- :root 暗色重设计(深蓝灰基底 + 粘土橙accent)
- 4个可变主题: light/warm/ice/forest
- 删除无效主题 midnight/neon/mono

### 7. UI 紧凑化
- 思考卡: 缩小间距/字号/边框
- ToolCard: 缩小间距/字号/图标/边距

### 8. MemoryPanel 重构
- 5个子组件提取为独立文件
- React.memo + useCallback 性能优化

### 9. 测试 (新增 58 用例)
- session_route_features 20 + tool_coherence 10 + tool_precheck 12 + grep 8 + bash
- agent 包总计 242 测试
