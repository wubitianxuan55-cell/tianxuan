import { useEffect, useRef } from "react";
import type { SlashArgItem } from "../lib/types";

// ArgMenu is the autocomplete dropdown for a slash command's arguments (the part
// after the command word) — e.g. /skill → list/show/new/paths, /model → refs.
// Like SlashMenu but the entries are bare tokens (no leading "/"); the Composer
// owns filtering, the active index, and key handling. Reuses .slashmenu styling.
export function ArgMenu({
  items,
  activeIndex,
  onPick,
  onHover,
}: {
  items: SlashArgItem[];
  activeIndex: number;
  onPick: (it: SlashArgItem) => void;
  onHover: (i: number) => void;
}) {
  // Keep the keyboard-selected item in view (the list overflows at 280px).
  const activeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    activeRef.current?.scrollIntoView({ block: "nearest" });
  }, [activeIndex]);
  return (
    <div className="absolute bottom-[calc(100%+6px)] left-0 right-0 max-h-[280px] overflow-y-auto bg-bg-elev border border-border rounded-[10px] p-[5px] shadow-[0_12px_32px_rgba(0,0,0,0.4)] z-20 animate-[menu-in_0.12s_ease]" role="listbox">
      {items.map((it, i) => (
        <button
          key={it.label}
          ref={i === activeIndex ? activeRef : undefined}
          role="option"
          aria-selected={i === activeIndex}
          className={`flex items-baseline gap-2 w-full px-2 py-1.5 bg-transparent border-0 rounded-md text-inherit text-left cursor-pointer ${
            i === activeIndex ? "bg-accent-soft" : ""
          }`}
          onMouseDown={(e) => {
            e.preventDefault();
            onPick(it);
          }}
          onMouseMove={() => onHover(i)}
        >
          <span className="font-mono text-[13px] text-accent shrink-0">{it.label}</span>
          {it.hint && <span className="font-mono text-[11.5px] text-fg-faint shrink-0">{it.hint}</span>}
        </button>
      ))}
    </div>
  );
}
