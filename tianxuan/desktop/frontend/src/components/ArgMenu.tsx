import { MenuContainer, useMenuScroll } from "./MenuContainer";
import type { SlashArgItem } from "../lib/types";

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
  const activeRef = useMenuScroll(activeIndex);
  return (
    <MenuContainer count={items.length}>
      {items.map((it, i) => (
        <button
          key={it.label}
          ref={i === activeIndex ? activeRef : undefined}
          role="option"
          aria-selected={i === activeIndex}
          className={`flex items-baseline gap-2 w-full px-2 py-1.5 bg-transparent border-0 rounded-md text-inherit text-left cursor-pointer transition-colors duration-100 ${
            i === activeIndex ? "bg-accent-soft border-l-[2px] border-l-accent pl-[6px]" : "border-l-[2px] border-l-transparent pl-[6px]"
          }`}
          onMouseDown={(e) => { e.preventDefault(); onPick(it); }}
          onMouseMove={() => onHover(i)}
        >
          <span className="font-mono text-[13px] text-accent shrink-0">{it.label}</span>
          {it.hint && (
            <span className="font-mono text-[11.5px] text-fg-faint shrink-0">{it.hint}</span>
          )}
        </button>
      ))}
    </MenuContainer>
  );
}
