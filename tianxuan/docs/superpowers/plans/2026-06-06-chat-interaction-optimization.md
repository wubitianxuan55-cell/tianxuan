# 对话交互优化 实现计划

> **给 agentic worker：** 使用 superpowers:subagent-driven-development 或 superpowers:executing-plans 来实现此计划。步骤使用 checkbox（`- [ ]`）语法跟踪。

**目标：** 重写对话渲染核心 — Transcript 虚拟滚动 + Markdown 流式缓存 + 思考卡默认折叠 + ToolCard 信息密度 + 输入历史回溯

**架构：** 混合方案 — 保留 React 18 + Zustand + Wails 布局体系，重写 Transcript / Message / ToolCard / Composer 四个组件，新增 MemoMarkdown 缓存层

**技术栈：** React 18 + @tanstack/react-virtual + react-markdown + highlight.js

**规格：** `docs/superpowers/specs/2026-06-06-chat-interaction-optimization.md`

---

## 阶段 0：依赖安装

- [ ] **step-0.1** 安装 @tanstack/react-virtual
  ```bash
  cd D:\AI\tianxuanX\tianxuan\desktop\frontend
  npm install @tanstack/react-virtual
  ```
  **验证：** `npm ls @tanstack/react-virtual` 显示版本号

---

## 阶段 1：MemoMarkdown — 流式缓存层

> 文件：`desktop/frontend/src/components/MemoMarkdown.tsx`（新建）

- [ ] **step-1.1** 创建 MemoMarkdown 组件
  ```tsx
  // desktop/frontend/src/components/MemoMarkdown.tsx
  import { useMemo } from "react";
  import { Markdown } from "./Markdown";

  interface MemoMarkdownProps {
    text: string;
    streaming: boolean;
  }

  /**
   * MemoMarkdown — 流式友好的 Markdown 渲染器。
   *
   * 核心思路：流式输出时，文本主体（前 N 字符）不变，只有尾部在增长。
   * 对主体做 useMemo 缓存 react-markdown 的解析结果，尾部用纯文本追加，
   * 避免每次 token 都触发完整的 Markdown re-parse + KaTeX re-render。
   *
   * 非流式时直接全量渲染。
   */
  export function MemoMarkdown({ text, streaming }: MemoMarkdownProps) {
    // 流式时：保留最后 200 字符作为"尾部增量"，其余缓存
    const cacheKey = streaming ? text.slice(0, Math.max(0, text.length - 200)) : text;
    const tail = streaming ? text.slice(cacheKey.length) : "";

    // 缓存主体部分的 Markdown 渲染
    const cached = useMemo(
      () => (cacheKey ? <Markdown text={cacheKey} /> : null),
      [cacheKey]
    );

    // 非流式：完整渲染
    if (!streaming) {
      return (
        <div className="msg__body">
          <Markdown text={text} />
        </div>
      );
    }

    // 流式：缓存主体 + 尾部纯文本（避免频繁 re-parse）+ 光标
    return (
      <div className="msg__body">
        {cached}
        {tail && <span>{tail}</span>}
        <span className="cursor" />
      </div>
    );
  }
  ```
  **验证：** `npx tsc --noEmit` 零错误

---

## 阶段 2：Message — 思考卡折叠 + 接入 MemoMarkdown

> 文件：`desktop/frontend/src/components/Message.tsx`（修改）

- [ ] **step-2.1** 修改 AssistantMessage — 思考卡默认折叠 + 改用 MemoMarkdown

  读取 `Message.tsx`，找到 `AssistantMessage` 组件。当前代码已有 `reasoning` 折叠按钮（`open` 默认 `false`），但逻辑需要微调：流式时自动展开思考卡。

  定位到以下区域并修改：

  **区域 1：移除旧的 throttle 逻辑（第 68-82 行附近）**

  找到：
  ```tsx
  // Throttled markdown during streaming: update every 200ms instead of every
  // token so tables/code/cards render progressively without excessive reflows.
  const [throttled, setThrottled] = useState(item.text);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastRef = useRef(item.text);
  useEffect(() => {
    if (!item.streaming) { setThrottled(item.text); return; }
    if (timerRef.current === null) {
      timerRef.current = setTimeout(() => { timerRef.current = null; setThrottled(lastRef.current); }, 200);
    }
    lastRef.current = item.text;
    return () => { if (timerRef.current !== null) { clearTimeout(timerRef.current); timerRef.current = null; } };
  }, [item.text, item.streaming]);
  ```

  替换为：
  ```tsx
  ```

  **区域 2：修改思考卡部分（`reasoning` 区块），流式时自动展开**

  找到：
  ```tsx
      {item.reasoning && (
        <div className="reasoning">
          <button className="reasoning__toggle" onClick={() => setOpen((v) => !v)}>
            <ChevronRight
              className={`reasoning__chevron ${open ? "reasoning__chevron--open" : ""}`}
              size={12}
            />
            {t("msg.thinking")}
            {thinkOnly && <span className="reasoning__timer">{elapsed}s</span>}
          </button>
          {open && <div className="reasoning__body">{item.reasoning}</div>}
        </div>
      )}
  ```

  替换为：
  ```tsx
      {item.reasoning && (
        <div className="reasoning">
          <button className="reasoning__toggle" onClick={() => setOpen((v) => !v)}>
            <ChevronRight
              className={`reasoning__chevron ${effectiveOpen ? "reasoning__chevron--open" : ""}`}
              size={12}
            />
            {item.streaming
              ? `💭 ${t("msg.thinking")}…`
              : `💭 ${t("msg.thinking")} (${item.reasoning.split("\n").filter(l => l.trim()).length} 段)`}
            {thinkOnly && <span className="reasoning__timer">{elapsed}s</span>}
          </button>
          {effectiveOpen && <div className="reasoning__body">{item.reasoning}</div>}
        </div>
      )}
  ```

  **区域 3：在 `open` state 附近添加 `effectiveOpen` 计算**

  找到：
  ```tsx
  const [open, setOpen] = useState(false);
  ```

  在其后添加：
  ```tsx
  // 流式时自动展开思考卡；完成后保持用户选择的状态
  const [userToggled, setUserToggled] = useState(false);
  const effectiveOpen = item.streaming ? true : userToggled ? open : false;
  ```

  并将 `onClick` 改为：
  ```tsx
  onClick={() => { setOpen((v) => !v); setUserToggled(true); }}
  ```

  **区域 4：替换 Markdown 渲染为 MemoMarkdown**

  找到：
  ```tsx
      <div className="msg__body">
        <Markdown text={throttled || item.text} />
        {item.streaming && <span className="cursor" />}
      </div>
  ```

  替换为：
  ```tsx
      <MemoMarkdown text={item.text} streaming={item.streaming} />
  ```

  **区域 5：更新 imports**

  移除不再使用的 import：
  - 删除 `import { Markdown } from "./Markdown";`
  - 删除 `import { useEffect, useRef, useState }` 中的 `useEffect` 和 `useRef`（如果不再使用——检查是否 `startRef` / `timerRef` / `lastRef` 仍被引用）
  - 添加 `import { MemoMarkdown } from "./MemoMarkdown";`

  **验证：** `npx tsc --noEmit` 零错误

---

## 阶段 3：ToolCard — 信息密度重构

> 文件：`desktop/frontend/src/components/ToolCard.tsx`（修改）

- [ ] **step-3.1** 只读工具默认折叠 + 嵌套缩进增强

  读取 `ToolCard.tsx`，找到以下区域并修改：

  **区域 1：非只读工具默认折叠（第 ~80 行，`open` state）**

  找到：
  ```tsx
  const [open, setOpen] = useState(false);
  ```

  替换为：
  ```tsx
  // 运行中的工具自动展开；已完成/出错的需要手动展开
  const [open, setOpen] = useState(item.status === "running");
  ```

  **区域 2：quiet 模式增强（第 ~90 行，`quiet` 变量）**

  找到：
  ```tsx
  const quiet =
    item.readOnly && !hasNested && item.status !== "error" && item.status !== "stopped";
  ```

  替换为：
  ```tsx
  // 只读工具（read_file/grep/ls/glob）默认一行显示，不出卡片
  const quiet =
    item.readOnly && !hasNested && item.status !== "error" && item.status !== "stopped";
  // quiet 工具不折叠——它们已经够紧凑了
  const effectiveOpen = quiet ? false : open;
  ```

  然后将模板中的 `open` 全部替换为 `effectiveOpen`。

  **区域 3：嵌套缩进增强（找到 `.tool__nested` 的渲染处）**

  找到：
  ```tsx
          {hasNested && (
            <div className="tool__nested">
  ```

  确保嵌套子调用在 `open` 条件下也渲染（已存在），保持不变。

  **验证：** `npx tsc --noEmit` 零错误

---

## 阶段 4：Transcript — 虚拟滚动

> 文件：`desktop/frontend/src/components/Transcript.tsx`（修改）

- [ ] **step-4.1** 重写 Transcript 接入 useVirtualizer

  完整替换 Transcript.tsx：

  ```tsx
  import { useCallback, useEffect, useMemo, useRef, useState } from "react";
  import { useVirtualizer } from "@tanstack/react-virtual";
  import type { Item } from "../lib/store";
  import { AssistantMessage, UserMessage } from "./Message";
  import { ToolCard } from "./ToolCard";
  import { ToolGroup, scanGroups } from "./ToolGroup";
  import { Welcome } from "./Welcome";

  type ToolItem = Extract<Item, { kind: "tool" }>;

  // scrollVersion 返回一个轻量信号，在 transcript 增长（新条目或流式更新）时改变。
  function scrollVersion(items: Item[]): string {
    const n = items.length;
    if (n === 0) return "0";
    const last = items[n - 1];
    switch (last.kind) {
      case "assistant":
        return `${n}:${last.id}:${last.text.length}:${last.streaming ? 1 : 0}`;
      case "tool":
        return `${n}:${last.id}:${last.status}`;
      default:
        return `${n}:${last.id}`;
    }
  }

  // mergeConsecutiveReasoning 合并相邻的纯思考 assistant 条目
  function mergeConsecutiveReasoning(items: Item[]): Item[] {
    const out: Item[] = [];
    for (const it of items) {
      const prev = out[out.length - 1];
      if (
        prev &&
        prev.kind === "assistant" &&
        it.kind === "assistant" &&
        !prev.text &&
        !it.text &&
        !prev.streaming &&
        !it.streaming
      ) {
        out[out.length - 1] = { ...prev, reasoning: prev.reasoning + "\n\n" + it.reasoning };
      } else {
        out.push(it);
      }
    }
    return out;
  }

  export function Transcript({
    items,
    onPrompt,
    onRewind,
  }: {
    items: Item[];
    onPrompt: (text: string) => void;
    onRewind?: (turn: number, scope: string) => void;
  }) {
    const scrollRef = useRef<HTMLDivElement>(null);
    const stick = useRef(true);

    // 预处理：合并连续推理 + 扫描工具组
    const grouped = useMemo(() => scanGroups(mergeConsecutiveReasoning(items)), [items]);

    // 虚拟滚动
    const virtualizer = useVirtualizer({
      count: grouped.length,
      getScrollElement: useCallback(() => scrollRef.current, []),
      estimateSize: useCallback(() => 120, []),
      overscan: 5,
    });

    const onScroll = useCallback(() => {
      const el = scrollRef.current;
      if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
    }, []);

    // 内容版本变化时自动滚到底部（如果 stick 为 true）
    const contentVersion = scrollVersion(items);
    useEffect(() => {
      if (!stick.current) return;
      const el = scrollRef.current;
      if (!el) return;
      const id = requestAnimationFrame(() => {
        el.scrollTop = el.scrollHeight;
      });
      return () => cancelAnimationFrame(id);
    }, [contentVersion]);

    // 子调用收集（虚拟列表的 item 是顶层条目，嵌套子调用在 ToolCard 内部渲染）
    const subcallsByParent = new Map<string, ToolItem[]>();
    for (const it of items) {
      if (it.kind === "tool" && it.parentId) {
        const arr = subcallsByParent.get(it.parentId) ?? [];
        arr.push(it);
        subcallsByParent.set(it.parentId, arr);
      }
    }

    // Rewind 菜单
    const [openTurn, setOpenTurn] = useState<number | null>(null);
    useEffect(() => {
      if (openTurn === null) return;
      const onDown = (e: MouseEvent) => {
        const el = e.target as Element | null;
        if (!el || !el.closest(".rewind")) setOpenTurn(null);
      };
      document.addEventListener("mousedown", onDown);
      return () => document.removeEventListener("mousedown", onDown);
    }, [openTurn]);

    const userTurn = new Map<string, number>();
    let nt = 0;
    for (const it of items) {
      if (it.kind === "user") userTurn.set(it.id, nt++);
    }

    // 渲染单个条目
    const renderItem = (g: (typeof grouped)[number]) => {
      if (g.kind === "group") {
        return <ToolGroup key={g.id} tools={g.tools} />;
      }
      const it = g.item;
      switch (it.kind) {
        case "user": {
          const tn = userTurn.get(it.id);
          return (
            <UserMessage
              key={it.id}
              text={it.text}
              turn={tn}
              open={tn != null && openTurn === tn}
              onToggle={() => setOpenTurn((cur) => (cur === tn ? null : (tn ?? null)))}
              onRewind={(turn, scope) => {
                onRewind?.(turn, scope);
                setOpenTurn(null);
              }}
            />
          );
        }
        case "assistant":
          return <AssistantMessage key={it.id} item={it} />;
        case "tool":
          if (it.parentId) return null;
          if (it.name === "todo_write") return null;
          if (it.name === "exit_plan_mode") return null;
          return <ToolCard key={it.id} item={it} subcalls={subcallsByParent.get(it.id)} />;
        case "phase":
          return (
            <div key={it.id} className="phase">
              {it.text}
            </div>
          );
        case "notice":
          return (
            <div key={it.id} className={`notice notice--${it.level}`}>
              {it.text}
            </div>
          );
        case "compaction":
          return <CompactionCard key={it.id} item={it as CompactionItem} />;
      }
    };

    return (
      <div className="transcript" ref={scrollRef} onScroll={onScroll}>
        {items.length === 0 && <Welcome onPrompt={onPrompt} />}

        <div
          style={{
            height: `${virtualizer.getTotalSize()}px`,
            width: "100%",
            position: "relative",
          }}
        >
          {virtualizer.getVirtualItems().map((virtualItem) => (
            <div
              key={virtualItem.key}
              data-index={virtualItem.index}
              ref={virtualizer.measureElement}
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                width: "100%",
                transform: `translateY(${virtualItem.start}px)`,
              }}
            >
              {renderItem(grouped[virtualItem.index])}
            </div>
          ))}
        </div>
      </div>
    );
  }

  type CompactionItem = Extract<Item, { kind: "compaction" }>;

  function CompactionCard({ item }: { item: CompactionItem }) {
    const [open, setOpen] = useState(false);
    if (item.pending) {
      return (
        <div className="compaction compaction--pending">
          <span className="compaction__spinner">⋯</span> Compacting conversation…
        </div>
      );
    }
    return (
      <div className="compaction">
        <button className="compaction__head" onClick={() => setOpen((v) => !v)}>
          <span className="compaction__icon">◆</span>
          <span className="compaction__title">Context compacted</span>
          <span className="compaction__meta">
            {item.messages} messages · {item.trigger}
          </span>
          <span className="compaction__toggle">{open ? "hide summary" : "show summary"}</span>
        </button>
        {open && <pre className="compaction__summary">{item.summary}</pre>}
      </div>
    );
  }
  ```
  **验证：** `npx tsc --noEmit` 零错误

---

## 阶段 5：Composer — 输入历史

> 文件：`desktop/frontend/src/components/Composer.tsx`（修改）

- [ ] **step-5.1** 添加输入历史回溯

  在 Composer.tsx 中：

  **区域 1：在组件顶部添加常量（第 18 行附近，`COMPOSER_MAX_VIEWPORT_RATIO` 之后）**

  ```tsx
  const INPUT_HISTORY_KEY = "reasonix.inputHistory";
  const MAX_INPUT_HISTORY = 50;
  ```

  **区域 2：在 `submit` 函数中添加保存逻辑（第 ~170 行，`onSend(displayText, submitText)` 之后）**

  找到：
  ```tsx
    setText("");
    setAttachments([]);
  ```

  替换为：
  ```tsx
    // 保存到输入历史
    if (displayText.trim()) {
      try {
        const history = JSON.parse(sessionStorage.getItem(INPUT_HISTORY_KEY) || "[]") as string[];
        history.unshift(displayText);
        sessionStorage.setItem(INPUT_HISTORY_KEY, JSON.stringify(history.slice(0, MAX_INPUT_HISTORY)));
      } catch { /* ignore */ }
    }
    setText("");
    setAttachments([]);
  ```

  **区域 3：添加历史回溯状态和逻辑（在 `submit` 函数上方添加）**

  ```tsx
  const [historyIndex, setHistoryIndex] = useState(-1);
  const historyDraft = useRef(""); // 用户在按 ↑ 之前的草稿

  const navigateHistory = (dir: 1 | -1) => {
    try {
      const history: string[] = JSON.parse(sessionStorage.getItem(INPUT_HISTORY_KEY) || "[]");
      if (history.length === 0) return;
      if (historyIndex === -1) historyDraft.current = text;
      const next = Math.max(-1, Math.min(history.length - 1, historyIndex + dir));
      setHistoryIndex(next);
      setText(next === -1 ? historyDraft.current : history[next] || "");
    } catch { /* ignore */ }
  };
  ```

  **区域 4：在 `onKeyDown` 中添加 ↑/↓ 处理（`pickActive()` 之后，`// Enter sends` 之前）**

  找到：
  ```tsx
    // Enter sends; Shift+Enter newline. isComposing guards IME (pinyin) confirms.
    if (e.key === "Enter" && !e.shiftKey && !composing) {
  ```

  在其上方插入：
  ```tsx
    // 空输入框 + 无菜单时：↑↓ 回溯已发送消息
    if (!menuMode && !composing) {
      if (e.key === "ArrowUp" && text === "") {
        e.preventDefault();
        navigateHistory(1);
        return;
      }
      if (e.key === "ArrowDown" && historyIndex >= 0) {
        e.preventDefault();
        navigateHistory(-1);
        return;
      }
      // 用户开始打字，退出历史模式
      if (e.key !== "ArrowUp" && e.key !== "ArrowDown" && historyIndex >= 0) {
        setHistoryIndex(-1);
      }
    }
  ```

  **区域 5：在 `submit` 函数中，发送后重置历史索引**

  在 `setText("")` 之前添加：
  ```tsx
    setHistoryIndex(-1);
  ```

  **验证：** `npx tsc --noEmit` 零错误

---

## 阶段 6：CSS — 卡片过渡 + 虚拟滚动适配

> 文件：`desktop/frontend/src/styles.css`（修改）

- [ ] **step-6.1** 添加思考卡过渡动画 + 虚拟滚动适配

  在 `styles.css` 末尾追加：

  ```css
  /* ========================================
     对话交互优化 — 过渡与动画
     ======================================== */

  /* 思考卡折叠动画 */
  .reasoning__body {
    max-height: 0;
    overflow: hidden;
    transition: max-height 200ms ease;
  }

  .reasoning__body--open {
    max-height: 2000px;
  }

  /* 思考卡 toggle 增强 */
  .reasoning__toggle {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    background: none;
    border: none;
    color: var(--fg-faint);
    font-size: 0.82em;
    cursor: pointer;
    padding: 2px 0;
    user-select: none;
  }

  .reasoning__toggle:hover {
    color: var(--fg-dim);
  }

  .reasoning__timer {
    margin-left: 4px;
    opacity: 0.6;
  }

  /* 工具卡折叠动画 */
  .tool__body {
    max-height: 0;
    overflow: hidden;
    transition: max-height 200ms ease;
  }

  .tool--live .tool__body {
    max-height: 3000px; /* 运行中自动展开 */
  }

  /* quiet 工具卡更紧凑 */
  .tool--quiet .tool__row {
    padding: 3px 8px;
    font-size: 0.84em;
    opacity: 0.7;
  }

  .tool--quiet .tool__row:hover {
    opacity: 1;
  }

  /* 虚拟滚动容器 */
  .transcript {
    contain: strict; /* 隔离重排，提升虚拟滚动性能 */
  }

  /* 流式光标 */
  .cursor {
    display: inline-block;
    width: 8px;
    height: 1em;
    background: var(--accent);
    animation: cursor-blink 1s step-end infinite;
    vertical-align: text-bottom;
    margin-left: 1px;
  }

  @keyframes cursor-blink {
    0%, 100% { opacity: 1; }
    50% { opacity: 0; }
  }
  ```

  **验证：** `npx vite build` 成功

---

## 阶段 7：端到端验证

- [ ] **step-7.1** 构建验证
  ```bash
  cd D:\AI\tianxuanX\tianxuan\desktop\frontend
  npm run build
  ```
  **验收：** tsc 零错误 + vite build 成功

- [ ] **step-7.2** 桌面端构建
  ```bash
  cd D:\AI\tianxuanX\tianxuan\desktop
  wails build
  ```
  **验收：** 输出 `build/bin/tianxuan-desktop.exe`

- [ ] **step-7.3** Go 测试
  ```bash
  go test -C D:\AI\tianxuanX\tianxuan\desktop ./...
  ```
  **验收：** 所有包通过

---

## 执行选项

计划完成，已保存。两种执行方式：

1. **内联执行** — 在当前会话逐步实现，每步验证
2. **子Agent驱动** — 每任务派发独立子Agent
