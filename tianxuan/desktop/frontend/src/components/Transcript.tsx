import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowDown } from "lucide-react";
import gsap from "gsap";
import { ScrollToPlugin } from "gsap/ScrollToPlugin";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { Item } from "../lib/store";
import { AssistantMessage, UserMessage } from "./Message";
import { StreamingIndicator } from "./StreamingIndicator";
import { ToolCard } from "./ToolCard";
import { ToolGroup, scanGroups } from "./ToolGroup";
import { ErrorCard } from "./ErrorCard";
import { Welcome } from "./Welcome";

gsap.registerPlugin(ScrollToPlugin);

// ── 滚动参数 ──────────────────────────────────────────────────────────
const BOTTOM_THRESHOLD_PX = 80; // 距底部 80px 以内视为"在底部"
const SCROLL_DURATION = 0.12;   // GSAP 滚动动画时长(s)

function isNearBottom(el: HTMLElement): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight < BOTTOM_THRESHOLD_PX;
}

type ToolItem = Extract<Item, { kind: "tool" }>;

// scrollVersion: 轻量级内容变化信号，只用最后一项的标识 + 流式状态。
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

// mergeConsecutiveReasoning: 合并连续纯推理消息，减少渲染碎片。
function mergeConsecutiveReasoning(items: Item[]): Item[] {
  const out: Item[] = [];
  for (const it of items) {
    let prevIdx = out.length - 1;
    while (prevIdx >= 0) {
      const pi = out[prevIdx];
      if (pi.kind === "phase" || pi.kind === "notice") { prevIdx--; continue; }
      if (pi.kind === "tool" && (pi.name === "todo_write" || pi.name === "exit_plan_mode")) { prevIdx--; continue; }
      break;
    }
    const prev = prevIdx >= 0 ? out[prevIdx] : null;
    if (
      prev && prev.kind === "assistant" && it.kind === "assistant" &&
      !prev.text && !it.text && !prev.streaming && !it.streaming
    ) {
      out[prevIdx] = { ...prev, reasoning: prev.reasoning + "\n\n" + it.reasoning };
    } else {
      out.push(it);
    }
  }
  return out;
}

export function Transcript({
  items, onPrompt, onRewind, running, onThreadEl, onScrollToTurnReady,
  cwd, cwdName, sessions, onResumeSession, meta,
}: {
  items: Item[];
  onPrompt: (text: string) => void;
  onRewind?: (turn: number, scope: string) => void;
  running: boolean;
  onThreadEl?: (el: HTMLElement | null) => void;
  onScrollToTurnReady?: (fn: (turn: number) => void) => void;
  cwd?: string;
  cwdName?: string;
  sessions?: import("../lib/types").SessionMeta[];
  onResumeSession?: (path: string) => Promise<void>;
  meta?: import("../lib/types").Meta;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const resizeFrame = useRef<number | null>(null);
  const layoutScrollFrames = useRef<number[]>([]);

  useEffect(() => {
    onThreadEl?.(scrollRef.current);
    return () => onThreadEl?.(null);
  }, [onThreadEl]);

  // 清理动画和 rAF
  useEffect(() => {
    return () => {
      if (resizeFrame.current !== null) cancelAnimationFrame(resizeFrame.current);
      for (const frame of layoutScrollFrames.current) cancelAnimationFrame(frame);
      layoutScrollFrames.current = [];
    };
  }, []);

  const [showScrollDown, setShowScrollDown] = useState(false);

  const onScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const atBottom = isNearBottom(el);
    stick.current = atBottom;
    setShowScrollDown(!atBottom && el.scrollHeight > el.clientHeight + 200);
  }, []);

  // ── 智能滚动：GSAP 驱动，无节流 ──────────────────────────────────
  const scrollToBottom = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    if (!stick.current) return;
    if (resizeFrame.current !== null) cancelAnimationFrame(resizeFrame.current);
    resizeFrame.current = requestAnimationFrame(() => {
      resizeFrame.current = null;
      if (!stick.current) return;
      gsap.to(el, {
        scrollTo: { y: "max" },
        duration: SCROLL_DURATION,
        ease: "none",
        overwrite: "auto",
      });
    });
  }, []);

  // 新问题提交时强制到底
  const onNewQuestion = useCallback(() => {
    stick.current = true;
    setShowScrollDown(false);
    scrollToBottom();
  }, [scrollToBottom]);

  // 布局后多帧确认滚动（工具结果展开等场景）
  const scrollToBottomAfterLayout = useCallback((frames = 4) => {
    for (const frame of layoutScrollFrames.current) cancelAnimationFrame(frame);
    layoutScrollFrames.current = [];
    const el = scrollRef.current;
    if (!el) return;
    // 先立即设置
    stick.current = true;
    el.scrollTop = el.scrollHeight;
    let remaining = frames;
    const tick = () => {
      if (remaining <= 0) return;
      const frame = requestAnimationFrame(() => {
        layoutScrollFrames.current = layoutScrollFrames.current.filter((id) => id !== frame);
        if (scrollRef.current) scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
        remaining -= 1;
        tick();
      });
      layoutScrollFrames.current.push(frame);
    };
    tick();
  }, []);

  // ── 内容变化时自动跟随 ──────────────────────────────────────────
  const contentVersion = scrollVersion(items);
  const prevItemsLen = useRef(items.length);
  // 仅在新用户消息提交时强制到底（工具结果/流式输出不强制）
  useEffect(() => {
    if (items.length > prevItemsLen.current) {
      const last = items[items.length - 1];
      if (last && last.kind === "user") {
        onNewQuestion();
      }
    }
    prevItemsLen.current = items.length;
  }, [items.length, onNewQuestion]);

  useEffect(() => {
    scrollToBottom();
  }, [contentVersion, scrollToBottom]);

  // ── ResizeObserver：工具结果展开/折叠时保持底部 ──────────────────
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => {
      if (!stick.current) return;
      scrollToBottom();
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [scrollToBottom]);

  // ── 预处理 ──────────────────────────────────────────────────────
  const grouped = useMemo(() => scanGroups(mergeConsecutiveReasoning(items)), [items]);

  const turnToIndex = useMemo(() => {
    const map = new Map<number, number>();
    let turn = 0;
    for (let i = 0; i < grouped.length; i++) {
      const g = grouped[i];
      const it = g.kind === "group" ? null : g.item;
      if (it && it.kind === "user") { map.set(turn, i); turn++; }
    }
    return map;
  }, [grouped]);

  const virtualizer = useVirtualizer({
    count: grouped.length,
    getScrollElement: useCallback(() => scrollRef.current, []),
    estimateSize: useCallback(() => 120, []),
    overscan: 12,
  });

  const turnToIndexRef = useRef(turnToIndex);
  turnToIndexRef.current = turnToIndex;
  const virtualizerRef = useRef(virtualizer);
  virtualizerRef.current = virtualizer;

  const scrollToTurnRef = useRef((turn: number) => {
    const idx = turnToIndexRef.current.get(turn);
    if (idx != null) {
      stick.current = true;
      virtualizerRef.current.scrollToIndex(idx, { align: "start" });
    }
  });
  useEffect(() => {
    onScrollToTurnReady?.(scrollToTurnRef.current);
  }, [onScrollToTurnReady]);

  // ── 折叠/展开保持滚动 ──────────────────────────────────────────
  const scheduleMeasure = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const savedTop = el.scrollTop;
    const timer = setTimeout(() => {
      if (scrollRef.current) scrollRef.current.scrollTop = savedTop;
    }, 250);
    return () => clearTimeout(timer);
  }, []);

  // ── 子调用收集 ──────────────────────────────────────────────────
  const subcallsByParent = new Map<string, ToolItem[]>();
  for (const it of items) {
    if (it.kind === "tool" && it.parentId) {
      const arr = subcallsByParent.get(it.parentId) ?? [];
      arr.push(it);
      subcallsByParent.set(it.parentId, arr);
    }
  }

  const [dismissedErrors, setDismissedErrors] = useState(new Set<string>());
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

  const renderItem = (g: (typeof grouped)[number]) => {
    if (g.kind === "group") {
      return <ToolGroup key={g.id} tools={g.tools} onCollapse={scheduleMeasure} />;
    }
    const it = g.item;
    switch (it.kind) {
      case "user": {
        const tn = userTurn.get(it.id);
        return (
          <div key={it.id} data-turn={tn != null ? tn : undefined}>
            <UserMessage
              key={it.id} text={it.text} turn={tn}
              open={tn != null && openTurn === tn}
              onToggle={() => setOpenTurn((cur) => (cur === tn ? null : (tn ?? null)))}
              onRewind={(turn, scope) => { onRewind?.(turn, scope); setOpenTurn(null); }}
            />
          </div>
        );
      }
      case "assistant":
        return <AssistantMessage key={it.id} item={it} onCollapse={scheduleMeasure} />;
      case "tool":
        if (it.parentId) return null;
        if (it.name === "todo_write" || it.name === "exit_plan_mode") return null;
        return <ToolCard key={it.id} item={it} subcalls={subcallsByParent.get(it.id)} />;
      case "phase":
        return <div key={it.id} className="phase">{it.text}</div>;
      case "notice":
        if (it.level === "warn") {
          if (dismissedErrors.has(it.id)) return null;
          return <ErrorCard key={it.id} item={it as any} onDismiss={(id) => setDismissedErrors((p) => new Set(p).add(id))} />;
        }
        if (it.text.startsWith("diagnostics:")) {
          const clean = it.text.includes("— clean");
          return (
            <div key={it.id} className={`flex items-center gap-1.5 px-4 py-1 text-[11px] ${clean ? "text-ok" : "text-warning"}`}>
              <span className="shrink-0">{clean ? "✔" : "⚠"}</span>
              <span>{it.text}</span>
            </div>
          );
        }
        return <div key={it.id} className="notice">{it.text}</div>;
      case "compaction":
        return <CompactionCard key={it.id} item={it} />;
    }
  };

  const scrollDown = useCallback(() => {
    stick.current = true;
    setShowScrollDown(false);
    scrollToBottomAfterLayout();
  }, [scrollToBottomAfterLayout]);

  return (
    <div className="transcript" ref={scrollRef} onScroll={onScroll}>
      <div className="max-w-[--maxw] mx-auto px-8">
        {items.length === 0 && (
          <Welcome onPrompt={onPrompt} cwd={cwd} cwdName={cwdName} sessions={sessions} onResumeSession={onResumeSession} meta={meta} />
        )}
        <StreamingIndicator running={running} items={items} />
        <div style={{ height: `${virtualizer.getTotalSize()}px`, width: "100%", position: "relative" }}>
          {virtualizer.getVirtualItems().map((virtualItem) => (
            <div
              key={virtualItem.key}
              data-index={virtualItem.index}
              ref={virtualizer.measureElement}
              style={{
                position: "absolute", top: 0, left: 0, width: "100%",
                display: "flex", flexDirection: "column",
                transform: `translateY(${virtualItem.start}px)`,
              }}
            >
              {renderItem(grouped[virtualItem.index])}
            </div>
          ))}
        </div>
      </div>
      {/* 回到底部按钮 —— 柔和设计，不抢眼 */}
      {showScrollDown && (
        <button
          className="absolute bottom-5 right-8 z-10 flex items-center gap-1.5 rounded-full bg-accent text-accent-fg border-0 cursor-pointer hover:brightness-110 active:scale-95 transition-all px-3 py-1.5 opacity-80 hover:opacity-100"
          style={{ boxShadow: "var(--ds-shadow-accent-btn)" }}
          onClick={scrollDown}
          aria-label="回到底部"
        >
          <ArrowDown size={14} />
          <span className="text-[11px] font-medium">到底</span>
        </button>
      )}
    </div>
  );
}

// ── CompactionCard ──────────────────────────────────────────────────
type CompactionItem = Extract<Item, { kind: "compaction" }>;
function CompactionCard({ item }: { item: CompactionItem }) {
  const [open, setOpen] = useState(false);
  if (item.pending) {
    return (
      <div className="flex items-center gap-2 my-1 mx-2 px-3 py-2 border border-border-soft rounded-lg bg-bg-soft text-fg-faint text-xs animate-pulse">
        <span className="text-accent font-bold">⋯</span> Compacting conversation…
      </div>
    );
  }
  return (
    <div className="my-1 mx-2 border border-border-soft rounded-lg bg-bg-soft overflow-hidden">
      <button className="flex items-center gap-2 w-full px-3 py-2 bg-transparent border-0 text-fg-dim text-[12.5px] cursor-pointer hover:bg-bg-elev" onClick={() => setOpen((v) => !v)}>
        <span className="text-accent text-xs shrink-0">◆</span>
        <span className="font-medium text-fg">Context compacted</span>
        <span className="text-fg-faint text-[11px] ml-auto">{item.messages} messages · {item.trigger}</span>
        <span className="text-fg-faint text-[10.5px] underline shrink-0">{open ? "hide summary" : "show summary"}</span>
      </button>
      {open && <pre className="m-0 p-3 bg-bg text-fg-dim text-[11.5px] leading-relaxed whitespace-pre-wrap border-t border-border-soft">{item.summary}</pre>}
    </div>
  );
}
