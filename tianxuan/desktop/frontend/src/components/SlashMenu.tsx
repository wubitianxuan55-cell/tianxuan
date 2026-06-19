import { useEffect, useRef } from "react";
import { useT } from "../lib/i18n";
import type { CommandInfo } from "../lib/types";

// SlashMenu is the "/" autocomplete dropdown above the composer. Presentational:
// the Composer owns filtering, the active index, and key handling; this renders
// the list and reports hover/pick. Uses mousedown (not click) so picking an item
// doesn't blur the textarea first.
export function SlashMenu({
  items,
  activeIndex,
  onPick,
  onHover,
}: {
  items: CommandInfo[];
  activeIndex: number;
  onPick: (c: CommandInfo) => void;
  onHover: (i: number) => void;
}) {
  const t = useT();
  // Keep the keyboard-selected item scrolled into view: the list is capped at
  // 280px and overflows, so ArrowDown past the visible window would otherwise
  // hide the active row. block:"nearest" only scrolls when it's actually off-screen.
  const activeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    activeRef.current?.scrollIntoView({ block: "nearest" });
  }, [activeIndex]);
  // builtin commands get no tag; custom (project) and mcp commands are labelled.
  const kindTag = (kind: CommandInfo["kind"]) =>
    kind === "custom"
      ? t("slash.project")
      : kind === "mcp"
        ? t("slash.mcp")
        : kind === "skill"
          ? t("slash.skill")
          : "";
  return (
    <div className="absolute bottom-[calc(100%+6px)] left-0 right-0 max-h-[280px] overflow-y-auto bg-bg-elev border border-border rounded-[10px] p-[5px] shadow-[0_12px_32px_rgba(0,0,0,0.4)] z-20 animate-[menu-in_0.12s_ease]" role="listbox">
      {items.map((c, i) => (
        <button
          key={c.kind + ":" + c.name}
          ref={i === activeIndex ? activeRef : undefined}
          role="option"
          aria-selected={i === activeIndex}
          className={`flex items-baseline gap-2 w-full px-2 py-1.5 bg-transparent border-0 rounded-md text-inherit text-left cursor-pointer ${
            i === activeIndex ? "bg-accent-soft" : ""
          }`}
          onMouseDown={(e) => {
            e.preventDefault();
            onPick(c);
          }}
          onMouseMove={() => onHover(i)}
        >
          <span className="font-mono text-[13px] text-accent shrink-0">/{c.name}</span>
          {c.hint && <span className="font-mono text-[11.5px] text-fg-faint shrink-0">{c.hint}</span>}
          <span className="text-[12.5px] text-fg-dim truncate">{c.description}</span>
          {kindTag(c.kind) && <span className="ml-auto text-[10px] uppercase tracking-[0.4px] text-fg-faint shrink-0">{kindTag(c.kind)}</span>}
        </button>
      ))}
    </div>
  );
}
