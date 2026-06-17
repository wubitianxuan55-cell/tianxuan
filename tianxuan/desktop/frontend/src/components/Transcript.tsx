import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { Item } from "../lib/store";
import { AssistantMessage, UserMessage } from "./Message";
import { StreamingIndicator } from "./StreamingIndicator";
import { ToolCard } from "./ToolCard";
import { ToolGroup, scanGroups } from "./ToolGroup";
import { JumpBar } from "./JumpBar";
import type { JumpEntry } from "./JumpBar";
import { Welcome } from "./Welcome";

type ToolItem = Extract<Item, { kind: "tool" }>;

// scrollVersion returns a lightweight signal that changes whenever the
// transcript grows (new items or streaming updates). Instead of iterating all
// items (O(n)), it uses just the count and the last item's identity — enough
// to trigger scroll-to-bottom on new content without heavy per-frame work.
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

// mergeConsecutiveReasoning collapses adjacent assistant items that have ONLY
// reasoning (no text) into one, so the model thinking in several bursts without
// tools in between renders as a single card instead of many.
function mergeConsecutiveReasoning(items: Item[]): Item[] {
  const out: Item[] = [];
  for (const it of items) {
    // 跳过不可见项（phase/notice/隐藏tool），找上一个可合并的思考卡
    let prevIdx = out.length - 1;
    while (prevIdx >= 0) {
      const pi = out[prevIdx];
      if (pi.kind === "phase" || pi.kind === "notice") { prevIdx--; continue; }
      if (pi.kind === "tool" && (pi.name === "todo_write" || pi.name === "exit_plan_mode")) { prevIdx--; continue; }
      break;
    }
    const prev = prevIdx >= 0 ? out[prevIdx] : null;
    if (
      prev &&
      prev.kind === "assistant" &&
      it.kind === "assistant" &&
      !prev.text &&
      !it.text &&
      !prev.streaming &&
      !it.streaming
    ) {
      out[prevIdx] = { ...prev, reasoning: prev.reasoning + "\n\n" + it.reasoning };
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
  running,
}: {
  items: Item[];
  onPrompt: (text: string) => void;
  onRewind?: (turn: number, scope: string) => void;
  running: boolean;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  // stick tracks whether the view is pinned to the bottom; once the user scrolls
  // up to read, we stop yanking them back down.
  const stick = useRef(true);

  // 预处理：合并连续推理 + 扫描工具组
  const grouped = useMemo(() => scanGroups(mergeConsecutiveReasoning(items)), [items]);

  // 虚拟滚动 — 动态高度测量 + 5 条预渲染
  const virtualizer = useVirtualizer({
    count: grouped.length,
    getScrollElement: useCallback(() => scrollRef.current, []),
    estimateSize: useCallback(() => 120, []),
    overscan: 12,  // V5.30: 增到12减少快速滚动空白
  });

  const onScroll = useCallback(() => {
    const el = scrollRef.current;
    if (el) stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  }, []);

  // Follow new content by setting scrollTop directly inside rAF so layout has
  // settled first — together with plain-text streaming this keeps the view from
  // jittering.
  const contentVersion = scrollVersion(items);
  useEffect(() => {
    if (!stick.current) return;
    const el = scrollRef.current;
    if (!el) return;
    // 虚拟滚动模式下，虚拟列表总高度变化时也要滚动到底部
    virtualizer.scrollToIndex(grouped.length - 1, { align: "end" });
    const id = requestAnimationFrame(() => {
      el.scrollTop = el.scrollHeight;
    });
    return () => cancelAnimationFrame(id);
  }, [contentVersion]);

  // Sub-agent calls carry a parentId; collect them under their parent `task`
  // call so the parent card can render them nested, and skip them at top level.
  const subcallsByParent = new Map<string, ToolItem[]>();
  for (const it of items) {
    if (it.kind === "tool" && it.parentId) {
      const arr = subcallsByParent.get(it.parentId) ?? [];
      arr.push(it);
      subcallsByParent.set(it.parentId, arr);
    }
  }

  // The rewind menu's open state is lifted here so at most one is open at a time;
  // a mousedown outside any .rewind closes it.
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

  // Each user message's turn = its ordinal among user messages, so a rewind
  // targets the matching checkpoint.
  const userTurn = new Map<string, number>();
  let nt = 0;
  for (const it of items) {
    if (it.kind === "user") userTurn.set(it.id, nt++);
  }

  // 构建跳转条目：grouped 中的每条用户消息对应一条跳转标记
  const jumpEntries: JumpEntry[] = useMemo(() => {
    const entries: JumpEntry[] = [];
    let turn = 0;
    for (let vi = 0; vi < grouped.length; vi++) {
      const g = grouped[vi];
      if (g.kind === "item" && g.item.kind === "user") {
        entries.push({ id: g.item.id, text: g.item.text, virtualIndex: vi, turn });
        turn++;
      }
    }
    return entries;
  }, [grouped]);

  // 折叠/展开时保持滚动位置稳定。
  // ResizeObserver（measureElement ref）会自动追踪内容尺寸变化并更新虚拟列表，
  // 手动调用 measure() 反而会在 CSS 过渡期间与 ResizeObserver 冲突导致布局混乱。
  // 这里只负责在过渡完成后恢复滚动位置，防止视口上方卡片展开时界面跳转。
  const scheduleMeasure = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const savedTop = el.scrollTop;
    // 等待 CSS max-height 过渡完成 (200ms) + 余量，然后恢复滚动位置
    const timer = setTimeout(() => {
      if (scrollRef.current) {
        scrollRef.current.scrollTop = savedTop;
      }
    }, 250);
    // 如果组件卸载（不太可能但防御性），清理定时器
    return () => clearTimeout(timer);
  }, []);

  // 渲染单条条目（user / assistant / tool / phase / notice / compaction）
  const renderItem = (g: (typeof grouped)[number]) => {
    if (g.kind === "group") {
      return <ToolGroup key={g.id} tools={g.tools} onCollapse={scheduleMeasure} />;
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
        return <AssistantMessage key={it.id} item={it} onCollapse={scheduleMeasure} />;
      case "tool":
        if (it.parentId) return null; // rendered nested under its parent
        if (it.name === "todo_write") return null; // shown live in the pinned TodoPanel
        if (it.name === "exit_plan_mode") return null; // the plan was shown in the approval card
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
        return <CompactionCard key={it.id} item={it} />;
    }
  };

  return (
    <div className="transcript" ref={scrollRef} onScroll={onScroll}>
      <div
        style={{
          position: "sticky",
          top: 0,
          zIndex: 10,
          float: "right",
          width: 0,
          height: 0,
        }}
      >
        <JumpBar entries={jumpEntries} totalItems={grouped.length} virtualizer={virtualizer} />
      </div>

      {items.length === 0 && <Welcome onPrompt={onPrompt} />}
      <StreamingIndicator running={running} items={items} />

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
              display: "flex",
              flexDirection: "column",
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

// CompactionCard marks a context-compaction boundary in the transcript. While
// the pass runs it shows a "compacting…" placeholder; once done it shows the
// message count and trigger with the summary collapsed behind a toggle (the
// summary is the new context base, so it's available but doesn't flood the view).
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
