# 对话交互优化 — 设计规格

> 日期：2026-06-06
> 方案：混合方案（B+）— 保留 React 布局体系，重写对话核心三个组件
> 状态：设计阶段

---

## 一、目标

在不推翻现有 React 18 + Zustand + Wails 架构的前提下，重写对话渲染核心，解决三个痛点：

1. **长对话卡顿** — 100+ 条消息后 DOM 节点堆积，滚动掉帧
2. **流式渲染闪烁** — Markdown 全文 re-parse，代码高亮重复计算
3. **信息密度失控** — 思考过程占满屏幕，工具调用列表冗长

---

## 二、改动清单

| 文件 | 动作 | 预计行数 |
|------|------|----------|
| `Transcript.tsx` | 重写：接入 `useVirtualizer` 虚拟滚动 | ~150 |
| `Message.tsx` | 新增 `MemoMarkdown` + `ReasoningCard` 折叠 | ~100 |
| `ui/Markdown.tsx` | 新增 AST 缓存层 | ~40 |
| `ToolCard.tsx` | 信息密度重构 | ~80 |
| `Composer.tsx` | 加输入历史回溯 | ~30 |
| `styles.css` | 虚拟滚动 + 卡片过渡动画 | ~60 |
| `ReasoningCard.tsx` | 新增组件（从 Message 中拆出） | ~50 |
| `MemoMarkdown.tsx` | 新增组件（带缓存的 Markdown） | ~60 |

**总计：8 文件，~570 行增量**

---

## 三、各模块设计

### 3.1 Transcript — 虚拟滚动

**引入：** `@tanstack/react-virtual`

```tsx
// 核心逻辑伪代码
const virtualizer = useVirtualizer({
  count: items.length,
  getScrollElement: () => scrollRef.current,
  estimateSize: () => 120,          // 默认估计高度
  measureElement: (el) => el.getBoundingClientRect().height,
  overscan: 5,                       // 上下各预渲染 5 条
});

// stick-to-bottom 复用现有逻辑
const stick = useRef(true);
virtualizer.options.onChange = () => {
  if (stick.current) scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight });
};
```

**关键约束：**
- 虚拟列表的每个 item 是一条"完整消息"（user/assistant/tool/phase/notice），内部嵌套（子工具调用）用普通 DOM，不参与虚拟化
- 流式更新时，当前 assistant 消息高度在变化，`measureElement` 的 `ResizeObserver` 自动触发重新测量
- `mergeConsecutiveReasoning` 和 `scanGroups` 逻辑保持不变，在传入 virtualizer 之前预处理

**不动的部分：**
- `scrollVersion` 信号保留（O(1) 检测新内容）
- `stick` ref 逻辑保留
- `userTurn` / `openTurn` / `subcallsByParent` 逻辑全部保留

### 3.2 Message — MemoMarkdown + ReasoningCard

#### MemoMarkdown（新增组件）

```
核心思路：同一条消息在流式更新时，Markdown AST 的主体（前 90%）不变，
只有最后一段在变化。用内容 hash 缓存 react-markdown 的解析结果。
```

```tsx
function MemoMarkdown({ text, streaming }: { text: string; streaming: boolean }) {
  // 流式时用前 N 字符做缓存 key，避免每次 re-parse
  const cacheKey = streaming ? text.slice(0, Math.max(0, text.length - 200)) : text;
  const cached = useMemo(() => <ReactMarkdown>{cacheKey}</ReactMarkdown>, [cacheKey]);
  
  if (!streaming) return cached;
  
  // 流式时：渲染缓存部分 + 尾部增量（纯文本，避免频繁 re-parse）
  const tail = text.slice(cacheKey.length);
  return (
    <>
      {cached}
      {tail ? <span>{tail}</span> : null}
      <span className="cursor" />
    </>
  );
}
```

#### ReasoningCard（从 Message 拆出）

```tsx
function ReasoningCard({ reasoning, streaming }: { reasoning: string; streaming: boolean }) {
  const [open, setOpen] = useState(false);
  if (!reasoning) return null;
  
  // 流式时自动展开
  const effectiveOpen = streaming || open;
  
  return (
    <div className="reasoning">
      <button className="reasoning__toggle" onClick={() => setOpen(v => !v)}>
        {streaming ? "💭 思考中…" : `💭 思考 (${reasoning.split('\n').filter(Boolean).length} 段)`}
      </button>
      {effectiveOpen && <div className="reasoning__body">{reasoning}</div>}
    </div>
  );
}
```

**交互规则：**
- 流式时默认展开（能看到模型在思考）
- 完成后默认折叠（节省屏幕空间）
- 用户手动切换后保持该状态

### 3.3 ToolCard — 信息密度

```
当前问题：
- 只读工具（read_file/grep/ls/glob）虽然 quiet 化，但仍然占一整行
- 所有工具调用默认展开 args/output，视觉噪音大
- 嵌套工具卡缩进不够明显

优化：
- 只读工具合并到一行：图标 + 名称 + 摘要（如 "read_file main.go (230 行)"）
- 非只读工具保持卡片样式，但默认折叠
- 嵌套缩进用左边框代替 margin
```

**改动点：**
1. `tool--quiet` 样式更紧凑（`font-size: 0.85em; padding: 2px 8px`）
2. 非 quiet 工具默认折叠（`open` 默认 `false`，运行中自动展开）
3. 嵌套左边框 `.tool__nested { border-left: 2px solid var(--border); margin-left: 12px; }`

### 3.4 Composer — 输入历史

**新增：上下箭头回溯已发送消息**

```tsx
const HISTORY_KEY = "reasonix.inputHistory";
const MAX_HISTORY = 50;

// 发送时保存
const submit = () => {
  // ... 现有逻辑 ...
  const history = JSON.parse(sessionStorage.getItem(HISTORY_KEY) || "[]");
  history.unshift(displayText);
  sessionStorage.setItem(HISTORY_KEY, JSON.stringify(history.slice(0, MAX_HISTORY)));
  setText("");
};

// 空输入框时按上下箭头
const onKeyDown = (e) => {
  if (e.key === "ArrowUp" && text === "" && !menuMode) {
    e.preventDefault();
    // 从 history 回填
  }
  if (e.key === "ArrowDown" && /* 在历史中 */) {
    e.preventDefault();
    // 回下一个
  }
  // ... 现有逻辑 ...
};
```

### 3.5 CSS — 动画与过渡

```
新增变量：
--transition-card: 150ms ease;      // 卡片折叠/展开
--transition-reasoning: 200ms ease; // 思考卡展开

新增动画：
.reasoning__body { 
  max-height: 0; overflow: hidden; transition: max-height var(--transition-reasoning);
}
.reasoning__body--open { max-height: 2000px; }

.tool__body {
  max-height: 0; overflow: hidden; transition: max-height var(--transition-card);
}
.tool__body--open { max-height: 3000px; }
```

---

## 四、不动的内容

- App.tsx 布局（侧栏/面板/快捷键）
- Zustand store（sessionStore / monitorStore / layoutStore）
- 后端 Go 代码（app.go / controller / agent）
- Wails 绑定
- 国际化文件
- Composer 的斜杠命令/@文件补全（已经很好）
- 主题系统

---

## 五、验收标准

1. 100 条消息的对话，滚动帧率 ≥ 50fps（Chrome DevTools Performance）
2. 流式输出时，CPU 占用不高于当前水平的 70%
3. 思考卡默认折叠，手动展开后不自动关闭
4. 只读工具默认收起，一行显示
5. 空输入框按 ↑ 回填上一条已发送消息
6. 所有现有功能不受影响（快捷键、面板、设置）
7. `npm run build` 零错误
