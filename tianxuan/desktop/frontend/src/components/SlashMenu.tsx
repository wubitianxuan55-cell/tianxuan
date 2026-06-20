import { MenuContainer, useMenuScroll } from "./MenuContainer";
import { useT } from "../lib/i18n";
import type { CommandInfo } from "../lib/types";

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
  const activeRef = useMenuScroll(activeIndex);

  const kindTag = (kind: CommandInfo["kind"]) =>
    kind === "custom"
      ? t("slash.project")
      : kind === "mcp"
        ? t("slash.mcp")
        : kind === "skill"
          ? t("slash.skill")
          : "";

  return (
    <MenuContainer count={items.length}>
      {items.map((c, i) => (
        <button
          key={c.kind + ":" + c.name}
          ref={i === activeIndex ? activeRef : undefined}
          role="option"
          aria-selected={i === activeIndex}
          className={`flex items-baseline gap-2 w-full px-2 py-1.5 bg-transparent border-0 rounded-md text-inherit text-left cursor-pointer transition-colors duration-100 ${
            i === activeIndex ? "bg-accent-soft border-l-[2px] border-l-accent pl-[6px]" : "border-l-[2px] border-l-transparent pl-[6px]"
          }`}
          onMouseDown={(e) => { e.preventDefault(); onPick(c); }}
          onMouseMove={() => onHover(i)}
        >
          <span className="font-mono text-[13px] text-accent shrink-0">/{c.name}</span>
          {c.hint && <span className="font-mono text-[11.5px] text-fg-faint shrink-0">{c.hint}</span>}
          <span className="text-[12.5px] text-fg-dim truncate">{c.description}</span>
          {kindTag(c.kind) && (
            <span className="ml-auto text-[10px] uppercase tracking-[0.4px] text-fg-faint shrink-0">
              {kindTag(c.kind)}
            </span>
          )}
        </button>
      ))}
    </MenuContainer>
  );
}
