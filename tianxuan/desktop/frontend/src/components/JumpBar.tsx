import { useMemo, useRef, useState, useEffect } from "react";
import type React from "react";
import type { Item } from "../lib/store";

interface JumpBarProps {
  items: Item[];
  threadEl?: HTMLElement | null;
}

/**
 * JumpBar — a thin right-edge navigation strip showing each user turn as a dot.
 * Hover shows a preview of the user's message; click scrolls to that turn.
 */
export function JumpBar({ items, threadEl }: JumpBarProps) {
  const [hovered, setHovered] = useState<number | null>(null);
  const [active, setActive] = useState<number | null>(null);
  const barRef = useRef<HTMLDivElement>(null);
  const [showPreview, setShowPreview] = useState(false);
  const [previewY, setPreviewY] = useState(0);

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

  // Scroll active dot into view
  useEffect(() => {
    if (active === null || !barRef.current) return;
    const el = barRef.current.querySelector(`[data-turn="${active}"]`);
    el?.scrollIntoView({ block: "nearest" });
  }, [active]);

  if (turns.length <= 1) return null;

  const onMove = (e: React.MouseEvent) => {
    const rect = barRef.current?.getBoundingClientRect();
    if (!rect) return;
    const relY = e.clientY - rect.top;
    setPreviewY(relY);
    const turnIdx = Math.round((relY / rect.height) * (turns.length - 1));
    const clamped = Math.max(0, Math.min(turns.length - 1, turnIdx));
    const turn = turns[clamped]?.turn ?? null;
    setHovered(turn);
    setShowPreview(true);
  };

  const scrollTo = (turn: number) => {
    setActive(turn);
    if (threadEl) {
      const el = threadEl.querySelector(`[data-turn="${turn}"]`);
      el?.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  };

  const hoverText = hovered !== null
    ? turns.find((v) => v.turn === hovered)?.text ?? null
    : null;

  return (
    <div
      ref={barRef}
      className="absolute right-1.5 top-1/2 -translate-y-1/2 flex flex-col items-center gap-0.5 z-10 py-2"
      onMouseMove={onMove}
      onMouseLeave={() => { setHovered(null); setShowPreview(false); }}
    >
      {turns.map((item) => {
        const isActive = active === item.turn;
        const isHovered = hovered === item.turn;
        return (
          <button
            key={item.turn}
            type="button"
            className={`relative w-[6px] h-[6px] rounded-full border-0 cursor-pointer p-0 shrink-0 transition-all duration-150 ${
              isActive
                ? "bg-accent scale-125 shadow-[0_0_4px_var(--accent)]"
                : isHovered
                  ? "bg-fg-dim scale-150"
                  : "bg-border hover:bg-fg-dim hover:scale-[1.6]"
            }`}
            data-turn={item.turn}
            onClick={(e) => { e.preventDefault(); scrollTo(item.turn); }}
            title={item.text.slice(0, 60)}
          >
            {/* Active ring pulse */}
            {isActive && (
              <span className="absolute inset-[-3px] rounded-full border border-accent/30 animate-pulse pointer-events-none" />
            )}
          </button>
        );
      })}

      {showPreview && hoverText && (
        <div
          className="absolute right-[calc(100%+10px)] max-w-64 px-2.5 py-1.5 bg-bg-elev-2 border border-border rounded-lg text-[11px] text-fg-dim leading-snug whitespace-pre-wrap break-words shadow-[0_4px_16px_rgba(0,0,0,0.25)] pointer-events-none z-20 animate-[menu-in_0.1s_ease]"
          style={{
            top: Math.max(0, Math.min(previewY - 18, (barRef.current?.clientHeight ?? 200) - 48)),
          }}
        >
          <span className="text-[10px] font-semibold text-fg-faint uppercase tracking-wider mr-1">#{hovered}</span>
          {hoverText}
        </div>
      )}
    </div>
  );
}
