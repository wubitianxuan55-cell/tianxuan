import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowDown } from "lucide-react";
import type { Item } from "../lib/store";
import { useItems } from "../lib/store";
import { AssistantMessage, UserMessage } from "./Message";
import { StreamingIndicator } from "./StreamingIndicator";
import { ToolCard } from "./ToolCard";
import { ToolGroup, scanGroups } from "./ToolGroup";
import { ErrorCard } from "./ErrorCard";
import { Welcome } from "./Welcome";
import { useEntranceAnimation } from "../lib/useEntranceAnimation";


// ── 滚动参数 ──────────────────────────────────────────────────────────
const BOTTOM_THRESHOLD_PX = 80;
const NOOP_SCROLL = () => {};

function isNearBottom(el: HTMLElement): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight < BOTTOM_THRESHOLD_PX;
}

type ToolItem = Extract<Item, { kind: "tool" }>;

// scrollVersion: 轻量级内容变化信号
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

// mergeConsecutiveReasoning: 合并连续纯推理消息
function mergeConsecutiveReasoning(items: Item[]): Item[] {
  const out: Item[] = [];
  for (const it of items) {
    let prevIdx = out.length - 1;
    while (prevIdx >= 0) {
      const pi = out[prevIdx];
      if (pi.kind === "phase" || pi.kind === "notice") { prevIdx--; continue; }
      if (pi.kind === "tool" && pi.name === "todo_write") { prevIdx--; continue; }
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
  onPrompt, onRewind, running, onThreadEl, onScrollToTurnReady,
  cwd, cwdName, sessions, onResumeSession, meta,
}: {
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
  const items = useItems();
  const scrollRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const rAF = useRef<number | null>(null);

  useEffect(() => {
    onThreadEl?.(scrollRef.current);
    return () => onThreadEl?.(null);
  }, [onThreadEl]);

  useEffect(() => {
    return () => { if (rAF.current !== null) cancelAnimationFrame(rAF.current); };
  }, []);

  const [showScrollDown, setShowScrollDown] = useState(false);

  const onScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const atBottom = isNearBottom(el);
    stick.current = atBottom;
    setShowScrollDown(!atBottom && el.scrollHeight > el.clientHeight);
  }, []);

  // ── 智能滚动 ──────────────────────────────────────────────────────
  const scrollToBottom = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    if (!stick.current) return;

    if (rAF.current !== null) cancelAnimationFrame(rAF.current);
    rAF.current = requestAnimationFrame(() => {
      rAF.current = null;
      if (!stick.current) return;
      el.scrollTop = el.scrollHeight;
    });
  }, []);

  const onNewQuestion = useCallback(() => {
    stick.current = true;
    setShowScrollDown(false);
    scrollToBottom();
  }, [scrollToBottom]);

  // ── 内容变化时自动跟随 ──────────────────────────────────────────
  const contentVersion = useMemo(() => scrollVersion(items), [items]);
  const prevItemsLen = useRef(items.length);
  useEffect(() => {
    if (items.length > prevItemsLen.current) {
      const last = items[items.length - 1];
      if (last && last.kind === "user") onNewQuestion();
    }
    prevItemsLen.current = items.length;
  }, [items.length, onNewQuestion]);

  useEffect(() => {
    scrollToBottom();
  }, [contentVersion, scrollToBottom]);

  // ── ResizeObserver ─────────────────────────────────────────────────
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
  // items 重置（新会话/切换会话）时清空 turnEls，防止残留旧 DOM 引用。
  useEffect(() => {
    if (items.length === 0) turnEls.current.clear();
  }, [items.length]);
  const grouped = useMemo(() => scanGroups(mergeConsecutiveReasoning(items)), [items]);

  // turn→DOM 元素映射（用于跳转）
  const turnEls = useRef(new Map<number, HTMLElement>());
  const scrollToTurnRef = useRef((turn: number) => {
    const el = turnEls.current.get(turn);
    if (el) el.scrollIntoView({ behavior: "smooth", block: "start" });
  });
  // V10.17.1: Transcript 卸载时清除 App 中的 scrollToTurn，避免
  // 重新挂载后 MessageNavigator/JumpBar 仍持有旧实例的 turnEls 引用导致跳转失效。
  useEffect(() => {
    onScrollToTurnReady?.(scrollToTurnRef.current);
    return () => onScrollToTurnReady?.(NOOP_SCROLL);
  }, [onScrollToTurnReady]);
  // ── 折叠/展开保持滚动 ──────────────────────────────────────────
  // 250ms 与 GSAP collapse 动画时长耦合（useGSAPCollapse 默认 duration）。
  // 若动画时长变更，此处需同步调整。
  const scheduleMeasure = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const savedTop = el.scrollTop;
    setTimeout(() => {
      if (scrollRef.current) scrollRef.current.scrollTop = savedTop;
    }, 250);
  }, []);

  // ── 入场动画 ──────────────────────────────────────────────────────
  const entranceRef = useEntranceAnimation<HTMLDivElement>(
    items.length > 0 ? `${items[0].id}|${items[items.length - 1].id}` : undefined,
    items.length,
  );

  // ── 子调用收集 ──────────────────────────────────────────────────
  const subcallsByParent = useMemo(() => {
    const map = new Map<string, ToolItem[]>();
    for (const it of items) {
      if (it.kind === "tool" && it.parentId) {
        const arr = map.get(it.parentId) ?? [];
        arr.push(it);
        map.set(it.parentId, arr);
      }
    }
    return map;
  }, [items]);

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

  const userTurn = useMemo(() => {
    const map = new Map<string, number>();
    let nt = 0;
    for (const it of items) {
      if (it.kind === "user") map.set(it.id, nt++);
    }
    return map;
  }, [items]);

  const scrollDown = useCallback(() => {
    stick.current = true;
    setShowScrollDown(false);
    scrollToBottom();
  }, [scrollToBottom]);

  return (
    <div className="relative flex-1 min-h-0">
    <div className="transcript h-full" ref={scrollRef} onScroll={onScroll}>
      <div className="max-w-[--maxw] mx-auto px-8" ref={entranceRef}>
        {items.length === 0 && (
          <Welcome onPrompt={onPrompt} cwd={cwd} cwdName={cwdName} sessions={sessions} onResumeSession={onResumeSession} meta={meta} />
        )}
        <StreamingIndicator running={running} items={items} />
        {grouped.map((g) => {
          if (g.kind === "group") {
            return <ToolGroup key={g.id} tools={g.tools} onCollapse={scheduleMeasure} />;
          }
          const it = g.item;
          switch (it.kind) {
            case "user": {
              const tn = userTurn.get(it.id);
              return (
                <div
                  key={it.id}
                  data-turn={tn != null ? tn : undefined}
                  data-entrance={it.id}
                  ref={(el) => {
                    if (el && tn != null) {
                      turnEls.current.set(tn, el);
                    } else if (tn != null) {
                      turnEls.current.delete(tn);
                    }
                  }}
                >
                  <UserMessage
                    text={it.text} turn={tn}
                    open={tn != null && openTurn === tn}
                    onToggle={() => setOpenTurn((cur) => (cur === tn ? null : (tn ?? null)))}
                    onRewind={(turn, scope) => { onRewind?.(turn, scope); setOpenTurn(null); }}
                  />
                </div>
              );
            }
            case "assistant":
              return (
                <div key={it.id} data-entrance={it.id}>
                  <AssistantMessage item={it} onCollapse={scheduleMeasure} />
                </div>
              );
            case "tool":
              if (it.parentId) return null;
              if (it.name === "todo_write") return null;
              return (
                <div key={it.id} data-entrance={it.id}>
                  <ToolCard item={it} subcalls={subcallsByParent.get(it.id)} />
                </div>
              );
            case "phase":
              return <div key={it.id} className="phase">{it.text}</div>;
            case "notice":
              if (it.level === "warn") {
                if (dismissedErrors.has(it.id)) return null;
                return <ErrorCard key={it.id} item={it as Extract<Item, { kind: "notice" }>} onDismiss={(id) => setDismissedErrors((p) => new Set(p).add(id))} />;
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
            default:
              return null;
          }
        })}
      </div>
      </div>
      {/* 回到底部按钮 —— 居中圆形，accent 色调 */}
      {showScrollDown && (
        <button
          className="absolute left-1/2 bottom-8 z-20 flex items-center justify-center w-9 h-9 rounded-full border border-accent/20 bg-bg-elev text-fg-dim cursor-pointer hover:text-accent hover:border-accent/40 hover:bg-bg-elev-2 active:scale-95 transition-all shadow-lg"
          style={{ transform: "translateX(-50%)" }}
          onClick={scrollDown}
          aria-label="回到底部"
        >
          <ArrowDown size={15} />
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
