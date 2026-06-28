import { useMemo, useRef, useState, useEffect, useCallback } from "react";
import type React from "react";
import { MessageSquare } from "lucide-react";
import type { Item } from "../lib/store";

interface Props {
  items: Item[];
  threadEl?: HTMLElement | null;
  scrollToTurn?: (turn: number) => void;
}

interface TurnEntry { turn: number; text: string; id: string; }

/**
 * MessageNavigator — 右侧面板「消息」标签页。
 * 列出所有用户消息，显示轮次编号 + 首行文字预览，
 * 点击/键盘导航后跳转到对话中对应轮次。
 */
export function MessageNavigator({ items, threadEl, scrollToTurn }: Props) {
  const [active, setActive] = useState<number | null>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const activeRef = useRef<HTMLDivElement>(null);

  // ── 提取所有用户消息，分配轮次编号 ──
  const turns = useMemo<TurnEntry[]>(() => {
    const result: TurnEntry[] = [];
    let n = 0;
    for (const it of items) {
      if (it.kind === "user") {
        result.push({ turn: n, text: it.text, id: it.id });
        n++;
      }
    }
    return result;
  }, [items]);

  // ── 默认选中最后一条消息 ──
  useEffect(() => {
    if (turns.length > 0) setActive(turns[turns.length - 1].turn);
    else setActive(null);
  }, [turns]);

  // ── 活跃项始终滚动到可视区 ──
  useEffect(() => {
    activeRef.current?.scrollIntoView({ block: "nearest" });
  }, [active]);

  // ── 跳转到指定轮次 ──
  const scrollTo = useCallback((turn: number) => {
    setActive(turn);
    if (scrollToTurn) {
      scrollToTurn(turn);
    } else if (threadEl) {
      // Fallback: DOM querySelector. With virtual scrolling, only visible
      // + overscan items exist in the DOM. For elements outside the viewport,
      // estimate scroll position by turn number (120px ~ average item height).
      const el = threadEl.querySelector('[data-turn="' + turn + '"]');
      if (el) {
        el.scrollIntoView({ behavior: "smooth", block: "start" });
      } else {
        threadEl.scrollTo({ top: turn * 120, behavior: "smooth" });
      }
    }
  }, [threadEl, scrollToTurn]);

  // ── 键盘导航 ──
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (turns.length <= 1) return;
    const idx = active !== null ? turns.findIndex(t => t.turn === active) : -1;
    let next: number | null = null;
    if (e.key === "ArrowDown")      next = Math.min(turns.length - 1, idx + 1);
    else if (e.key === "ArrowUp")   next = Math.max(0, idx - 1);
    else if (e.key === "Home")      next = 0;
    else if (e.key === "End")       next = turns.length - 1;
    else if (e.key === "Enter")     { if (active !== null) scrollTo(active); return; }
    else return;
    e.preventDefault();
    if (next !== null) scrollTo(turns[next].turn);
  }, [active, turns, scrollTo]);

  // ── 截取首行文字作为预览 ──
  const preview = (text: string, maxLen = 72): string => {
    const firstLine = text.split("\n")[0] ?? text;
    return firstLine.length > maxLen ? firstLine.slice(0, maxLen) + "\u2026" : firstLine;
  };

  if (turns.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 h-full text-fg-faint">
        <MessageSquare size={28} className="opacity-30" />
        <span className="text-[12px]">暂无消息</span>
        <span className="text-[10px] opacity-60">发起对话后自动出现</span>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full" onKeyDown={handleKeyDown} tabIndex={0}>
      {/* 标题栏 */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-border-soft shrink-0">
        <div className="flex items-center gap-1.5 text-[11px] font-semibold text-fg-faint uppercase tracking-wider">
          <MessageSquare size={12} /> 消息 ({turns.length})
        </div>
        <div className="text-[9px] text-fg-faint/50 font-mono">↑↓ Enter</div>
      </div>

      {/* 消息列表 */}
      <div ref={listRef} className="flex-1 overflow-y-auto py-1">
        {turns.map((item) => {
          const isActive = active === item.turn;
          return (
            <div
              key={item.turn}
              ref={isActive ? activeRef : undefined}
              role="button"
              tabIndex={-1}
              onClick={() => scrollTo(item.turn)}
              onKeyDown={(e) => { if (e.key === "Enter") scrollTo(item.turn); }}
              className={"flex items-start gap-2.5 px-3 py-1.5 cursor-pointer transition-[background,border-color] border-l-[3px] " +
                (isActive ? "border-l-accent bg-accent-soft/60" : "border-l-transparent hover:bg-bg-soft")}
              title={item.text.slice(0, 200)}
            >
              {/* 轮次编号 */}
              <span className={"shrink-0 w-7 text-right font-mono text-[10px] tabular-nums leading-[1.5] " +
                (isActive ? "text-accent font-semibold" : "text-fg-faint")}>
                #{item.turn}
              </span>
              {/* 消息预览 */}
              <span className={"flex-1 min-w-0 text-[12px] leading-[1.5] whitespace-pre-wrap break-words " +
                (isActive ? "text-fg" : "text-fg-dim")}>
                {preview(item.text)}
              </span>
            </div>
          );
        })}
      </div>

      {/* 底栏 */}
      <div className="shrink-0 px-3 py-1.5 border-t border-border-soft text-[10px] text-fg-faint/60 font-mono text-center">
        {turns.length} 条消息
      </div>
    </div>
  );
}
