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
      className="jumpbar"
      onMouseMove={onMove}
      onMouseLeave={() => { setHovered(null); setShowPreview(false); }}
    >
      {turns.map((item) => {
        const isActive = active === item.turn;
        return (
          <button
            key={item.turn}
            type="button"
            className={`jumpbar__dot ${isActive ? "jumpbar__dot--active" : ""}`}
            data-turn={item.turn}
            onClick={(e) => { e.preventDefault(); scrollTo(item.turn); }}
            title={item.text.slice(0, 60)}
          />
        );
      })}

      {showPreview && hoverText && (
        <div
          className="jumpbar__preview"
          style={{
            top: Math.max(0, Math.min(previewY - 16, (barRef.current?.clientHeight ?? 200) - 40)),
          }}
        >
          {hoverText}
        </div>
      )}
    </div>
  );
}
