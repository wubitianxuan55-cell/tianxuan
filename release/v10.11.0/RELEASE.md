# V10.11.0 发布记录

**发布日期：** 2026-06-29  
**构建产物：** `release/v10.11.0/tianxuan-desktop.exe` (16MB)  
**SHA256：** `ed353644fdc7956568c706798525a367e0e1845e27a8a4111477cc4d4789a5c6`

---

## 变更摘要

| 指标 | 数值 |
|------|------|
| 修改文件 | ~15 个 |
| 新增翻译键 | 14 个（zh/en/zh-TW） |
| 新增测试 | 修复后全通过 |

---

## 一、流式输出流畅度（P0）

### 1.1 streamBatcher 参数微调
- `maxBytes`: 64 → 8（每批 2-3 token，消除 20+ token 爆发）
- `maxDelay`: 16ms → 4ms（低于人眼闪烁融合阈值）
- 效果：文字以连续流的方式出现，无可见块状爆发

### 1.2 滚动抖动修复
- 流式期间：`scrollTop = scrollHeight` 直接跟随（无动画）
- 非流式期间：GSAP `power2.out` 平滑过渡
- 修复：每 token 重启 GSAP tween 导致的"永远到不了底"的抖动

### 1.3 闪烁光标 GPU 优化
- `background-clip:text` 渐变 → `border-left` 脉冲动画
- GPU 合成层保留（`translateZ(0)`），但不再做文本裁剪重绘

---

## 二、终端输出降噪（TextSink）

### 2.1 推理→进度指示器
```
改前：▎ thinking（2000+ 字 dim 文本刷屏）
改后：▎ thinking ··· 347 chars（每 500ms \r 覆盖更新）
```

### 2.2 工具批量摘要
```
改前：-> read_file {"path":"a.go"}   ← 5 行同时出现
      -> read_file {"path":"b.go"}
      -> ...

改后：▸ 5 tools running...            ← 一行摘要
```

### 2.3 错误聚合
```
改前：⊘ edit_file old_string not found
      ⊘ bash exit code 1
改后：⊘ 2 tools failed: edit_file(...), bash(...)
```

---

## 三、记忆面板重设计

- 全中文 i18n（消除 12 处硬编码）
- 快速添加区：卡片式容器 + scope 路径 mono 字体
- 建议卡片：`SuggestionCard` 独立组件，胶囊 badge，证据引用线
- 搜索框仅在有事实时显示，空结果 + 清空筛选

---

## 四、CMD 窗口闪现修复

- `hideBashWindow`: 添加 `CREATE_NO_WINDOW` (0x08000000)
- 覆盖 git、markitdown、hook、notify、plugin 等所有子进程
- `proc.HideWindow` 统一导出供跨包使用

---

## 五、其他 UI 优化

| 项目 | 修改 |
|------|------|
| ToolGroup 动画 | CSS Grid → GSAP（修复 Chrome 闪烁） |
| StreamingIndicator | `return null` → `invisible` 占位（防布局跳动） |
| ThemeSwitcher | 5→9 主题（+forest/midnight/neon/mono） |
| 回到底部按钮 | `absolute` → `fixed` + `backdrop-blur` 毛玻璃 |
| 推理→正文过渡 | `msg-fade-in` 0.25s opacity+translateY |
