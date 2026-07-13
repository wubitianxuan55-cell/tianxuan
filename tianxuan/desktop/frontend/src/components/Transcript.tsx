import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowDown, ChevronRight } from "lucide-react";
import type { Item } from "../lib/store";
import { useItems, useTurnStartAt } from "../lib/store";
import { AssistantMessage, UserMessage } from "./Message";
import { StreamingIndicator } from "./StreamingIndicator";
import { ToolCard } from "./ToolCard";
import { ToolGroup, scanGroups } from "./ToolGroup";
import { ErrorCard } from "./ErrorCard";
import { Welcome } from "./Welcome";
import { useEntranceAnimation } from "../lib/useEntranceAnimation";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";
import { ReadOnlyBatch } from "./ReadOnlyBatch";
import { ProcessBrainIcon, ProcessPhaseIcon } from "./ProcessCard";
import { displayReasoningText } from "../lib/reasoningDisplay";
import { buildTurnGroups, createWarmLayerState, warmPagination, warmUserPreview, warmLayerWithExpandedTurn, warmLayerWithNextColdPage, compactQuestionText, questionAnchorId, type WarmLayerState, type QuestionAnchor } from "../lib/transcriptGrouping";

// ── Scroll helpers ──────────────────────────────────────────────────────
const BOTTOM_THRESHOLD_PX = 80;
const NOOP_SCROLL = () => {};
function isNearBottom(el: HTMLElement) { return el.scrollHeight - el.scrollTop - el.clientHeight < BOTTOM_THRESHOLD_PX; }
type ToolItem = Extract<Item, { kind: "tool" }>;
type AssistantItem = Extract<Item, { kind: "assistant" }>;

const HOT_TURNS = 30;
const WARM_PAGE_SIZE = 20;
const QUESTION_NAV_MIN_COUNT = 2;

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

// ── TurnCollapse: fold completed process steps ──────────────────────────
function collapseDisplayItems(items: Item[]): Item[] {
  return items.filter((it) => {
    if (it.kind === "assistant" && !it.reasoning) return false;
    if (it.kind === "tool") {
      if ("parentId" in it && it.parentId) return false;
      if (it.name === "todo_write") return false;
    }
    return true;
  });
}

function TurnCollapse({ items, toolCount, thoughtCount, running = false }: { items: Item[]; toolCount: number; thoughtCount: number; running?: boolean }) {
  const [open, setOpen] = useState(running);
  const userOverridden = useRef(false);
  const prevRunningRef = useRef(running);
  const bodyRef = useRef<HTMLDivElement>(null);
  const display = useMemo(() => collapseDisplayItems(items), [items]);
  const turnStartAt = useTurnStartAt();
  const finalElapsedRef = useRef(0);

  useGSAPCollapse(bodyRef, open);

  // Auto-open during running, auto-close when done (unless user toggled)
  useEffect(() => {
    const wasRunning = prevRunningRef.current;
    prevRunningRef.current = running;
    if (running) {
      if (!wasRunning) userOverridden.current = false;
      if (!userOverridden.current) setOpen(true);
    } else if (wasRunning && !userOverridden.current) {
      setOpen(false);
      // Freeze elapsed at completion moment
      finalElapsedRef.current = turnStartAt > 0 ? Math.max(0, Date.now() - turnStartAt) : 0;
    }
  }, [running, turnStartAt]);

  if (display.length === 0) return null;

  const labelParts: string[] = [];
  if (toolCount > 0) labelParts.push(`${toolCount} 个工具`);
  if (thoughtCount > 0) labelParts.push(`${thoughtCount} 次思考`);

  // Elapsed: live while running, frozen after completion
  const elapsed = running
    ? (turnStartAt > 0 ? Math.max(0, Date.now() - turnStartAt) : 0)
    : finalElapsedRef.current;
  const elapsedStr = elapsed > 0 ? (elapsed < 60000 ? `${Math.round(elapsed / 1000)}s` : `${Math.floor(elapsed / 60000)}m${Math.round((elapsed % 60000) / 1000)}s`) : "";
  const label = labelParts.length > 0
    ? (elapsedStr ? `${labelParts.join(" · ")} · ${elapsedStr}` : labelParts.join(" · "))
    : (running ? "处理中…" : elapsedStr);

  // Pre-compute body: batch consecutive completed read-only tools into ReadOnlyBatch
  const body = useMemo(() => {
    const nodes: React.ReactNode[] = [];
    const roBatch: ToolItem[] = [];
    const flushRO = () => {
      if (roBatch.length === 0) return;
      nodes.push(<ReadOnlyBatch key={`rob-${roBatch[0].id}`} items={[...roBatch]} />);
      roBatch.length = 0;
    };
    for (const it of display) {
      if (it.kind === "tool" && it.status === "done" && it.readOnly) {
        roBatch.push(it);
        continue;
      }
      flushRO();
      switch (it.kind) {
        case "assistant":
          nodes.push(<InlineReasoning key={it.id} item={it as AssistantItem} />);
          break;
        case "tool":
          nodes.push(<ToolCard key={it.id} item={it} />);
          break;
        case "phase":
          nodes.push(<div key={it.id} className="phase"><ProcessPhaseIcon size={12} /><span>{it.text}</span></div>);
          break;
      }
    }
    flushRO();
    return nodes;
  }, [display]);

  const toggle = () => { userOverridden.current = true; setOpen((v) => !v); };

  return (
    <div className={`turn-collapse${open ? " turn-collapse--open" : ""}`}>
      <button
        className="reasoning__head"
        data-running={running ? "" : undefined}
        onClick={toggle}
        aria-expanded={open}
      >
        <ChevronRight size={13} className={`reasoning__chevron${open ? " reasoning__chevron--open" : ""}`} />
        <span className="turn-collapse__label">{label}</span>
      </button>
      <div ref={bodyRef} className="turn-collapse__body">{body}</div>
    </div>
  );
}

// ── InlineReasoning — collapsible thought block inside TurnCollapse ───
function InlineReasoning({ item }: { item: AssistantItem }) {
  const [open, setOpen] = useState(true);
  const bodyRef = useRef<HTMLDivElement>(null);
  useGSAPCollapse(bodyRef, open);
  const running = item.streaming && !item.text;
  const reasoning = displayReasoningText(item.reasoning ?? "", {
    streaming: item.streaming ?? false,
    truncateStreaming: true,
  });
  if (!reasoning) return null;
  return (
    <div className={`turn-collapse__reasoning-phase${open ? " turn-collapse__reasoning-phase--open" : ""}`}>
      <button
        type="button"
        className="turn-collapse__reasoning-head"
        data-running={running ? "" : undefined}
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        <ProcessBrainIcon size={12} />
        <span>思考</span>
        <ChevronRight className={`reasoning__chevron${open ? " reasoning__chevron--open" : ""}`} size={12} />
      </button>
      <div ref={bodyRef} className="turn-collapse__inline-reasoning">{reasoning}</div>
    </div>
  );
}

// ── QuestionJumpBar ────────────────────────────────────────────────────
function QuestionJumpBar({ questions, onJump }: { questions: QuestionAnchor[]; onJump: (q: QuestionAnchor) => void }) {
  const [hoverIdx, setHoverIdx] = useState<number | null>(null);
  if (questions.length < QUESTION_NAV_MIN_COUNT) return null;
  return (
    <nav className="jump-bar" aria-label="Questions">
      <div className="jump-scroll">
        {questions.map((q, i) => (
          <button
            key={q.id}
            className="jump-item"
            onClick={() => onJump(q)}
            onMouseEnter={() => setHoverIdx(i)}
            onMouseLeave={() => setHoverIdx(null)}
          >
            <span className="jump-dot" data-d={hoverIdx != null ? Math.min(Math.abs(i - hoverIdx), 2) : undefined} />
            {hoverIdx === i && <span className="jump-preview"><span className="jump-text">{compactQuestionText(q.text)}</span></span>}
          </button>
        ))}
      </div>
    </nav>
  );
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
  const rawItems = useItems();
  const items = useMemo(() => mergeConsecutiveReasoning(rawItems), [rawItems]);
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
  useEffect(() => { scrollToBottom(); }, [contentVersion, scrollToBottom]);

  const prevItemsLen = useRef(items.length);
  useEffect(() => {
    if (items.length > prevItemsLen.current) {
      const last = items[items.length - 1];
      if (last && last.kind === "user") onNewQuestion();
    }
    prevItemsLen.current = items.length;
  }, [items.length, onNewQuestion]);

  useEffect(() => {
    const el = scrollRef.current; if (!el) return;
    let prevScrollHeight = 0, prevClientHeight = 0;
    const ro = new ResizeObserver(() => {
      const sh = el.scrollHeight, ch = el.clientHeight;
      if (prevScrollHeight > 0 && sh === prevScrollHeight && ch !== prevClientHeight) {
        const prevDist = prevScrollHeight - el.scrollTop - prevClientHeight;
        el.scrollTop = sh - ch - Math.min(prevDist, sh - ch);
      } else {
        if (stick.current) scrollToBottom();
      }
      prevScrollHeight = sh; prevClientHeight = ch;
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [scrollToBottom]);

  useEffect(() => { if (items.length === 0) turnEls.current.clear(); }, [items.length]);
  const turnGroups = useMemo(() => buildTurnGroups(items), [items]);

  const [warmState, setWarmState] = useState<WarmLayerState>(() => createWarmLayerState(""));
  const turnCount = turnGroups.length;
  const { warmStartTurn, warmEndTurn, coldTurnCount } = useMemo(
    () => warmPagination({ turnCount, hotTurns: HOT_TURNS, pageSize: WARM_PAGE_SIZE, coldPage: warmState.coldPage }),
    [turnCount, warmState.coldPage],
  );

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

  // Question navigation
  const questions = useMemo<QuestionAnchor[]>(() =>
    turnGroups.map((tg, i) => ({
      id: questionAnchorId(tg.userItem.id),
      text: (tg.userItem as Extract<Item, { kind: "user" }>).text,
      turn: i,
    })),
  [turnGroups]);
  const [showQuestionNav, setShowQuestionNav] = useState(false);
  useEffect(() => {
    if (questions.length >= QUESTION_NAV_MIN_COUNT) setShowQuestionNav(true);
  }, [questions.length]);

  const handleJumpToQuestion = useCallback((q: QuestionAnchor) => {
    const el = document.getElementById(questionAnchorId(q.id || ""));
    if (el) el.scrollIntoView({ behavior: "smooth", block: "start" });
  }, []);

  const scrollDown = useCallback(() => { stick.current = true; setShowScrollDown(false); scrollToBottom(); }, [scrollToBottom]);

  // Partition items into (process, outside) segments.
  // Each assistant answer text acts as a segment boundary: process items
  // before the text fold into one TurnCollapse, text renders outside,
  // then subsequent process items start a new TurnCollapse.
  // Pattern ported from DeepSeek-Reasonix partitionTurnItems.
  const renderItems = useMemo(() => {
    const segments: { processItems: Item[]; outsideItems: Item[] }[] = [];
    let curProcess: Item[] = [];
    let curOutside: Item[] = [];

    const flush = () => {
      if (curProcess.length > 0 || curOutside.length > 0) {
        segments.push({ processItems: curProcess, outsideItems: curOutside });
        curProcess = []; curOutside = [];
      }
    };

    for (const it of items) {
      // User messages always flush and start a new segment
      if (it.kind === "user") {
        flush();
        segments.push({ processItems: [], outsideItems: [it] });
        continue;
      }

      // Assistant with text: reasoning → TurnCollapse, text → outside.
      // Strip reasoning from outside copy so text area stays clean.
      if (it.kind === "assistant") {
        if (it.text) {
          if (curOutside.length > 0) flush();
          if (it.reasoning) curProcess.push({ ...it, text: "" } as Item);
          curOutside.push({ ...it, reasoning: "" } as Item);
        } else {
          // reasoning-only: new segment if text already rendered
          if (curOutside.length > 0) flush();
          curProcess.push(it);
        }
        continue;
      }

      // process items (tools, compactions, info notices → inside fold;
      // phases → outside as section headers)
      if (curOutside.length > 0) flush();
      if (it.kind === "tool" || it.kind === "compaction" || it.kind === "notice") {
        curProcess.push(it);
      } else if (it.kind === "phase") {
        curOutside.push(it);
      } else {
        curOutside.push(it);
      }
    }
    flush();
    return segments;
  }, [items]);

  const renderSegmentGroups = useCallback((segItems: Item[]) => {
    const g = scanGroups(segItems);
    return g.map((gi) => {
      if (gi.kind === "group") return <ToolGroup key={gi.id} tools={gi.tools} onCollapse={scheduleMeasure} />;
      const it = gi.item;
      switch (it.kind) {
        case "user": {
          const tn = userTurn.get(it.id);
          return (
            <div key={it.id} id={questionAnchorId(it.id)} data-turn={tn != null ? tn : undefined} data-entrance={it.id}
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
              <AssistantMessage item={it as AssistantItem} onCollapse={scheduleMeasure} />
            </div>
          );
        case "tool":
          if (it.parentId) return null;
          if (it.name === "todo_write") return null;
          return (
            <div key={it.id} data-entrance={it.id}>
              <ToolCard item={it as ToolItem} subcalls={subcallsByParent.get(it.id)} />
            </div>
          );
        case "phase": return <div key={it.id} className="phase"><ProcessPhaseIcon size={12} /><span>{it.text}</span></div>;
        case "notice":
          if (it.level === "warn") {
            if (dismissedErrors.has(it.id)) return null;
            return <ErrorCard key={it.id} item={it as Extract<Item, { kind: "notice" }>} onDismiss={(id) => setDismissedErrors((p) => new Set(p).add(id))} />;
          }
          if (it.text.startsWith("diagnostics:")) {
            const clean = it.text.includes("— clean");
            return <div key={it.id} className={`diag-line${clean ? " diag-line--ok" : " diag-line--warn"}`}>
              <span className="shrink-0">{clean ? "✔" : "⚠"}</span><span>{it.text}</span></div>;
          }
          return <div key={it.id} className="notice">{it.text}</div>;
        case "compaction": return <CompactionCard key={it.id} item={it} />;
        default: return null;
      }
    });
  }, [userTurn, openTurn, onRewind, scheduleMeasure, subcallsByParent, dismissedErrors]);

  return (
    <div className="transcript-shell relative flex-1 min-h-0">
    <div className="transcript h-full" ref={scrollRef} onScroll={onScroll}>
      {/* Warm zone — older history pagination */}
      {coldTurnCount > 0 && (
        <button className="warm-collapse" onClick={() => setWarmState((s) => warmLayerWithNextColdPage(s, ""))}>
          显示更早的 {Math.min(coldTurnCount, WARM_PAGE_SIZE)} 轮对话
        </button>
      )}

      {/* Warm turns (folded) */}
      {warmStartTurn < warmEndTurn && turnGroups.slice(warmStartTurn, warmEndTurn).map((tg, i) => {
        const turnNum = warmStartTurn + i;
        const isExpanded = warmState.expandedWarmTurns.has(turnNum);
        return (
          <div key={tg.userItem.id} className={`warm-turn${isExpanded ? " warm-turn--open" : ""}`}>
            <button className="warm-turn__head" onClick={() => setWarmState((s) => warmLayerWithExpandedTurn(s, "", turnNum, !isExpanded))}>
              <ChevronRight size={13} className={`warm-turn__chevron${isExpanded ? " warm-turn__chevron--open" : ""}`} />
              <span className="warm-turn__preview">{warmUserPreview((tg.userItem as Extract<Item, { kind: "user" }>).text)}</span>
              <span className="warm-turn__meta">{tg.toolCount > 0 ? `${tg.toolCount} 个工具` : ""}{tg.assistantPreview ? ` · ${tg.assistantPreview.slice(0, 40)}` : ""}</span>
            </button>
            {isExpanded && (
              <div style={{ padding: "8px 12px" }}>
                {renderSegmentGroups(items.slice(tg.startIdx, tg.endIdx))}
              </div>
            )}
          </div>
        );
      })}

      <div className="max-w-[--maxw] mx-auto px-8" ref={entranceRef}>
        {items.length === 0 && (
          <Welcome onPrompt={onPrompt} cwd={cwd} cwdName={cwdName} sessions={sessions} onResumeSession={onResumeSession} meta={meta} />
        )}
        <StreamingIndicator running={running} items={rawItems} />

        {/* Hot zone — always rendered with TurnCollapse wrapping process */}
        {renderItems.slice(warmEndTurn).filter(seg => seg.processItems.length > 0 || seg.outsideItems.length > 0).map((seg, segIdx, arr) => {
          const toolCount = seg.processItems.filter((it) => it.kind === "tool").length;
          const thoughtCount = seg.processItems.filter((it) => it.kind === "assistant" && it.reasoning).length;
          const hasProcess = seg.processItems.length > 0;
          const isLast = segIdx === arr.length - 1;
          // Stable key: use first process item's id to avoid React remounting
          // when segments shift. Falls back to segIdx for empty/edge cases.
          const segKey = seg.processItems[0]?.id ?? seg.outsideItems[0]?.id ?? `seg${segIdx}`;

          return (
            <div key={segKey}>
              {/* 过程在上 — TurnCollapse always wraps, auto-expands when running */}
              {hasProcess && (
                <TurnCollapse items={seg.processItems} toolCount={toolCount} thoughtCount={thoughtCount} running={running && isLast} />
              )}
              {/* 文本在下 */}
              {seg.outsideItems.length > 0 && renderSegmentGroups(seg.outsideItems)}
            </div>
          );
        })}
      </div>
    </div>

    {/* Jump bar */}
    {items.length > 0 && showQuestionNav && (
      <QuestionJumpBar questions={questions} onJump={handleJumpToQuestion} />
    )}

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
    return <div className="compaction compaction--pending"><span className="text-accent font-bold">⋯</span> 正在压缩对话…</div>;
  }
  return (
    <div className="compaction" style={{display: "block"}}>
      <button className="compaction__head" onClick={() => setOpen((v) => !v)}>
        <span className="text-accent text-xs shrink-0">◆</span>
        <span className="font-medium">上下文已压缩</span>
        <span className="compaction__meta">{item.messages} 条消息 · {item.trigger}</span>
        <span className="text-fg-faint text-[10.5px] underline shrink-0">{open ? "隐藏摘要" : "显示摘要"}</span>
      </button>
      {open && <pre className="compaction__body">{item.summary}</pre>}
    </div>
  );
}
