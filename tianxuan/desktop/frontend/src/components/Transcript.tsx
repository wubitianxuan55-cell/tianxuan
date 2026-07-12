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

// ── Scroll helpers ──────────────────────────────────────────────────────
const BOTTOM_THRESHOLD_PX = 80;
const NOOP_SCROLL = () => {};
function isNearBottom(el: HTMLElement) { return el.scrollHeight - el.scrollTop - el.clientHeight < BOTTOM_THRESHOLD_PX; }
type ToolItem = Extract<Item, { kind: "tool" }>;

function scrollVersion(items: Item[]): string {
  const n = items.length; if (n === 0) return "0";
  const last = items[n - 1];
  switch (last.kind) {
    case "assistant": return `${n}:${last.id}:${last.text.length}:${last.streaming ? 1 : 0}`;
    case "tool": return `${n}:${last.id}:${last.status}`;
    default: return `${n}:${last.id}`;
  }
}

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
    if (prev && prev.kind === "assistant" && it.kind === "assistant" && !prev.text && !it.text && !prev.streaming && !it.streaming) {
      out[prevIdx] = { ...prev, reasoning: prev.reasoning + "\n\n" + it.reasoning };
    } else { out.push(it); }
  }
  return out;
}

// ── Transcript ──────────────────────────────────────────────────────────

export function Transcript({
  onPrompt, onRewind, running, onThreadEl, onScrollToTurnReady,
  cwd, cwdName, sessions, onResumeSession, meta,
}: {
  onPrompt: (text: string) => void;
  onRewind?: (turn: number, scope: string) => void;
  running: boolean;
  onThreadEl?: (el: HTMLElement | null) => void;
  onScrollToTurnReady?: (fn: (turn: number) => void) => void;
  cwd?: string; cwdName?: string;
  sessions?: import("../lib/types").SessionMeta[];
  onResumeSession?: (path: string) => Promise<void>;
  meta?: import("../lib/types").Meta;
}) {
  const items = useItems();
  const scrollRef = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const rAF = useRef<number | null>(null);

  useEffect(() => { onThreadEl?.(scrollRef.current); return () => onThreadEl?.(null); }, [onThreadEl]);
  useEffect(() => { return () => { if (rAF.current !== null) cancelAnimationFrame(rAF.current); }; }, []);

  const [showScrollDown, setShowScrollDown] = useState(false);
  const onScroll = useCallback(() => {
    const el = scrollRef.current; if (!el) return;
    const atBottom = isNearBottom(el);
    stick.current = atBottom;
    setShowScrollDown(!atBottom && el.scrollHeight > el.clientHeight);
  }, []);

  const scrollToBottom = useCallback(() => {
    const el = scrollRef.current; if (!el || !stick.current) return;
    if (rAF.current !== null) cancelAnimationFrame(rAF.current);
    rAF.current = requestAnimationFrame(() => {
      rAF.current = null; if (!stick.current) return;
      el.scrollTop = el.scrollHeight;
    });
  }, []);

  const onNewQuestion = useCallback(() => { stick.current = true; setShowScrollDown(false); scrollToBottom(); }, [scrollToBottom]);

  const contentVersion = useMemo(() => scrollVersion(items), [items]);
  const prevItemsLen = useRef(items.length);
  useEffect(() => {
    if (items.length > prevItemsLen.current) {
      const last = items[items.length - 1];
      if (last && last.kind === "user") onNewQuestion();
    }
    prevItemsLen.current = items.length;
  }, [items.length, onNewQuestion]);
  useEffect(() => { scrollToBottom(); }, [contentVersion, scrollToBottom]);

  useEffect(() => {
    const el = scrollRef.current; if (!el) return;
    let prevScrollHeight = 0;
    let prevClientHeight = 0;
    const ro = new ResizeObserver(() => {
      const sh = el.scrollHeight;
      const ch = el.clientHeight;
      if (prevScrollHeight > 0 && sh === prevScrollHeight && ch !== prevClientHeight) {
        // Layout-only resize (Composer/footer height change, not new content).
        // Maintain the distance from the bottom so typing doesn't cause jumps.
        const prevDist = prevScrollHeight - el.scrollTop - prevClientHeight;
        el.scrollTop = sh - ch - Math.min(prevDist, sh - ch);
      } else {
        // Content changed — use normal stick logic.
        prevScrollHeight = sh;
        if (!stick.current) { prevClientHeight = ch; return; }
        scrollToBottom();
      }
      prevScrollHeight = sh;
      prevClientHeight = ch;
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [scrollToBottom]);

  useEffect(() => { if (items.length === 0) turnEls.current.clear(); }, [items.length]);
  const merged = useMemo(() => mergeConsecutiveReasoning(items), [items]);
  const grouped = useMemo(() => scanGroups(merged), [merged]);

  const turnEls = useRef(new Map<number, HTMLElement>());
  const scrollToTurnRef = useRef((turn: number) => {
    const el = turnEls.current.get(turn); if (el) el.scrollIntoView({ behavior: "smooth", block: "start" });
  });
  useEffect(() => { onScrollToTurnReady?.(scrollToTurnRef.current); return () => onScrollToTurnReady?.(NOOP_SCROLL); }, [onScrollToTurnReady]);

  const scheduleMeasure = useCallback(() => {
    const el = scrollRef.current; if (!el) return;
    const savedTop = el.scrollTop;
    setTimeout(() => { if (scrollRef.current) scrollRef.current.scrollTop = savedTop; }, 250);
  }, []);

  const entranceRef = useEntranceAnimation<HTMLDivElement>(
    items.length > 0 ? `${items[0].id}|${items[items.length - 1].id}` : undefined, items.length,
  );

  const subcallsByParent = useMemo(() => {
    const map = new Map<string, ToolItem[]>();
    for (const it of items) { if (it.kind === "tool" && it.parentId) { const a = map.get(it.parentId) ?? []; a.push(it); map.set(it.parentId, a); } }
    return map;
  }, [items]);

  const [dismissedErrors, setDismissedErrors] = useState(new Set<string>());
  const [openTurn, setOpenTurn] = useState<number | null>(null);
  useEffect(() => {
    if (openTurn === null) return;
    const onDown = (e: MouseEvent) => { if (!(e.target as Element)?.closest(".rewind")) setOpenTurn(null); };
    document.addEventListener("mousedown", onDown); return () => document.removeEventListener("mousedown", onDown);
  }, [openTurn]);

  const userTurn = useMemo(() => {
    const map = new Map<string, number>(); let nt = 0;
    for (const it of items) { if (it.kind === "user") map.set(it.id, nt++); }
    return map;
  }, [items]);

  const scrollDown = useCallback(() => { stick.current = true; setShowScrollDown(false); scrollToBottom(); }, [scrollToBottom]);

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
                <div key={it.id} data-turn={tn != null ? tn : undefined} data-entrance={it.id}
                  ref={(el) => { if (el && tn != null) turnEls.current.set(tn, el); else if (tn != null) turnEls.current.delete(tn); }}>
                  <UserMessage text={it.text} turn={tn}
                    open={tn != null && openTurn === tn}
                    onToggle={() => setOpenTurn((cur) => (cur === tn ? null : (tn ?? null)))}
                    onRewind={(turn, scope) => { onRewind?.(turn, scope); setOpenTurn(null); }} />
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
            case "phase": return <div key={it.id} className="phase">{it.text}</div>;
            case "notice":
              if (it.level === "warn") {
                if (dismissedErrors.has(it.id)) return null;
                return <ErrorCard key={it.id} item={it as Extract<Item, { kind: "notice" }>} onDismiss={(id) => setDismissedErrors((p) => new Set(p).add(id))} />;
              }
              if (it.text.startsWith("diagnostics:")) {
                const clean = it.text.includes("— clean");
                return <div key={it.id} className={`flex items-center gap-1.5 px-4 py-1 text-[11px] ${clean ? "text-ok" : "text-warning"}`}>
                  <span className="shrink-0">{clean ? "✔" : "⚠"}</span><span>{it.text}</span></div>;
              }
              return <div key={it.id} className="notice">{it.text}</div>;
            case "compaction": return <CompactionCard key={it.id} item={it} />;
            default: return null;
          }
        })}
      </div>
      </div>
      {showScrollDown && (
        <button className="absolute left-1/2 bottom-8 z-20 flex items-center justify-center w-9 h-9 rounded-full border border-accent/20 bg-bg-elev text-fg-dim cursor-pointer hover:text-accent hover:border-accent/40 hover:bg-bg-elev-2 active:scale-95 transition-all shadow-lg"
          style={{ transform: "translateX(-50%)" }} onClick={scrollDown} aria-label="回到底部">
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
    return <div className="flex items-center gap-2 my-1 mx-2 px-3 py-2 border border-border-soft rounded-lg bg-bg-soft text-fg-faint text-xs animate-pulse">
      <span className="text-accent font-bold">⋯</span> Compacting conversation…</div>;
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
