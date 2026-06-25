import { useMemo, useRef, useState, useEffect, useCallback } from "react";
import type React from "react";
import type { Item } from "../lib/store";

interface JumpBarProps {
  items: Item[];
  threadEl?: HTMLElement | null;
  scrollToTurn?: (turn: number) => void;
}

/**
 * JumpBar — 右侧轮次导航横条。每个用户消息对应一条横条，
 * 显示轮次编号。↑↓ 键盘导航，点击滚动到对应轮次。
 * 轮次超过 15 时容器可滚动，始终显示活跃轮次。
 */
export function JumpBar({ items, threadEl, scrollToTurn }: JumpBarProps) {
  const [active, setActive] = useState<number | null>(null);
  const barRef = useRef<HTMLDivElement>(null);
  const activeRef = useRef<HTMLButtonElement>(null);

  // Extract user messages with their turn number
  const turns = useMemo(() => {
    const result: { turn: number; text: string; id: string }[] = [];
    let n = 0;
    for (const it of items) {
      if (it.kind === "user") {
        result.push({ turn: n, text: it.text.slice(0, 80), id: it.id });
        n++;
      }
    }
    return result;
  }, [items]);

  // Set active to the last turn
  useEffect(() => {
    if (turns.length > 0) setActive(turns[turns.length - 1].turn);
  }, [turns]);

  // Scroll active into view inside the bar
  useEffect(() => {
    activeRef.current?.scrollIntoView({ block: "nearest" });
  }, [active]);

  const scrollTo = useCallback((turn: number) => {
    setActive(turn);
    if (scrollToTurn) {
      scrollToTurn(turn);
    } else if (threadEl) {
      const el = threadEl.querySelector(`[data-turn="${turn}"]`);
      el?.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [threadEl, scrollToTurn]);

  // Keyboard navigation
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (turns.length <= 1) return;
    const currentIdx = active !== null ? turns.findIndex(t => t.turn === active) : -1;
    let nextIdx: number | null = null;
    if (e.key === "ArrowDown")     nextIdx = Math.min(turns.length - 1, currentIdx + 1);
    else if (e.key === "ArrowUp")  nextIdx = Math.max(0, currentIdx - 1);
    else if (e.key === "Home")     nextIdx = 0;
    else if (e.key === "End")      nextIdx = turns.length - 1;
    else return;
    e.preventDefault();
    if (nextIdx !== null) scrollTo(turns[nextIdx].turn);
  }, [active, turns, scrollTo]);

  if (turns.length <= 1) return null;

  return (
    <div
      className="absolute right-2 top-1/2 -translate-y-1/2 flex flex-col items-stretch z-10 w-[22px]"
      onKeyDown={handleKeyDown}
      tabIndex={0}
    >
      {/* 轮次计数 */}
      <div className="text-center text-[9px] text-fg-faint/50 font-mono leading-none mb-0.5 select-none">
        {turns.length}
      </div>

      {/* 横条列表——超过 15 轮可滚动 */}
      <div
        ref={barRef}
        className={`flex flex-col gap-[3px] items-stretch ${turns.length > 15 ? "overflow-y-auto" : ""}`}
        style={turns.length > 15 ? { maxHeight: "calc(100vh - 280px)", scrollbarWidth: "none" as any } : undefined}
      >
        {turns.map((item) => {
          const isActive = active === item.turn;
          return (
            <button
              key={item.turn}
              ref={isActive ? activeRef : undefined}
              type="button"
              data-turn={item.turn}
              tabIndex={-1}
              onClick={(e) => { e.preventDefault(); scrollTo(item.turn); }}
              title={`#${item.turn}: ${item.text}`}
              className={`relative h-[11px] min-h-[11px] rounded-sm border-0 cursor-pointer transition-all duration-150 ${
                isActive
                  ? "bg-accent shadow-[0_0_6px_var(--accent)]"
                  : "bg-border hover:bg-fg-dim hover:shadow-[0_0_3px_var(--border)]"
              }`}
            >
              {/* 轮次编号 */}
              <span className={`absolute inset-0 flex items-center justify-center text-[7.5px] font-bold select-none leading-none pointer-events-none ${
                isActive ? "text-accent-fg" : "text-fg-faint/60"
              }`}>
                {item.turn}
              </span>
              {/* 活跃光晕 */}
              {isActive && (
                <span className="absolute inset-[-2px] rounded-sm border border-accent/40 animate-pulse pointer-events-none" />
              )}
            </button>
          );
        })}
      </div>

      {/* 底部提示 */}
      <div className="text-center text-[8px] text-fg-faint/30 font-mono leading-none mt-0.5 select-none">
        ↑↓
      </div>
    </div>
  );
}
