import { Folder, FileText } from "lucide-react";
import { MenuContainer, useMenuScroll } from "./MenuContainer";
import type { DirEntry } from "../lib/types";

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
  const activeRef = useMenuScroll(activeIndex);
  return (
    <MenuContainer count={items.length} hint="Tab 进入子目录">
      {items.map((e, i) => (
        <button
          key={(e.isDir ? "d:" : "f:") + e.name}
          ref={i === activeIndex ? activeRef : undefined}
          role="option"
          aria-selected={i === activeIndex}
          className={`flex items-baseline gap-2 w-full px-2 py-1.5 bg-transparent border-0 rounded-md text-inherit text-left cursor-pointer transition-colors duration-100 ${
            i === activeIndex ? "bg-accent-soft border-l-[2px] border-l-accent pl-[6px]" : "border-l-[2px] border-l-transparent pl-[6px]"
          }`}
          onMouseDown={(ev) => { ev.preventDefault(); onPick(e); }}
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
    </MenuContainer>
  );
}
