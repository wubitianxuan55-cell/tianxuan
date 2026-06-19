import { useEffect, useRef } from "react";
import { Folder, FileText } from "lucide-react";
import type { DirEntry } from "../lib/types";

// FileMenu is the "@" file-reference dropdown above the composer. Like SlashMenu,
// it's presentational — the Composer owns navigation and the one-level-at-a-time
// descend logic. Reuses the .slashmenu container styling.
export function FileMenu({
  items,
  activeIndex,
  onPick,
  onHover,
}: {
  items: DirEntry[];
  activeIndex: number;
  onPick: (e: DirEntry) => void;
  onHover: (i: number) => void;
}) {
  // Keep the keyboard-selected item in view (the list overflows at 280px).
  const activeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    activeRef.current?.scrollIntoView({ block: "nearest" });
  }, [activeIndex]);
  return (
    <div className="absolute bottom-[calc(100%+6px)] left-0 right-0 max-h-[280px] overflow-y-auto bg-bg-elev border border-border rounded-[10px] p-[5px] shadow-[0_12px_32px_rgba(0,0,0,0.4)] z-20 animate-[menu-in_0.12s_ease]" role="listbox">
      {items.map((e, i) => (
        <button
          key={(e.isDir ? "d:" : "f:") + e.name}
          ref={i === activeIndex ? activeRef : undefined}
          role="option"
          aria-selected={i === activeIndex}
          className={`flex items-baseline gap-2 w-full px-2 py-1.5 bg-transparent border-0 rounded-md text-inherit text-left cursor-pointer ${
            i === activeIndex ? "bg-accent-soft" : ""
          }`}
          onMouseDown={(ev) => {
            ev.preventDefault();
            onPick(e);
          }}
          onMouseMove={() => onHover(i)}
        >
          {e.isDir ? (
            <Folder size={13} className="text-accent shrink-0" />
          ) : (
            <FileText size={13} className="text-fg-faint shrink-0" />
          )}
          <span className="font-mono text-[13px] text-fg font-normal shrink-0">
            {e.name}
            {e.isDir ? "/" : ""}
          </span>
        </button>
      ))}
    </div>
  );
}
