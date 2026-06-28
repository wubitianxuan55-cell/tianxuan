import { useRef, useState } from "react";
import { ChevronRight, FolderOpen } from "lucide-react";
import { ToolCard } from "./ToolCard";
import { useCompact } from "../hooks/useCompact";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";
import { subjectOf } from "../lib/tools";
import type { Item } from "../lib/store";

type ToolItem = Extract<Item, { kind: "tool" }>;

// ToolGroup collapses consecutive same-name tool calls into a single row.
export function ToolGroup({ tools, onCollapse }: { tools: ToolItem[]; onCollapse?: () => void }) {
  const [open, setOpen] = useState(false);
  const contentRef = useRef<HTMLDivElement>(null);
  useGSAPCollapse(contentRef, open);
  const compact = useCompact();
  if (tools.length === 0) return null;
  const t = tools[0];

  // 提取所有工具的操作目标 — 最多 3 个，溢出显示 +N
  const allSubjects = tools
    .map(t => subjectOf(t.name, t.args))
    .filter((s): s is string => !!s);
  const uniqueSubjects = [...new Set(allSubjects)];
  const subjects = uniqueSubjects.slice(0, 3);
  const moreSubjects = uniqueSubjects.length - 3;

  const rowPy = compact ? "py-1" : "py-1.5";
  const rowPx = compact ? "px-2" : "px-2.5";
  const iconSize = compact ? 12 : 14;

  return (
    <div className="my-1 border border-border-soft rounded-lg overflow-hidden bg-bg-elev/30">
      <div
        className={`flex items-center gap-2 ${rowPx} ${rowPy} cursor-pointer hover:bg-bg-elev/60 text-fg-dim transition-colors duration-[var(--dur-fast)]`}
        onClick={() => { setOpen((v) => !v); onCollapse?.(); }}
      >
        <ChevronRight
          className={`shrink-0 transition-transform duration-180 ${open ? "rotate-90" : ""}`}
          size={iconSize}
        />
        <FolderOpen className="shrink-0 text-fg-faint" size={iconSize} />
        <span className={`font-mono text-fg font-medium ${compact ? "text-[11px]" : "text-xs"}`}>{t.name}</span>
        <span className={`text-fg-faint font-mono ${compact ? "text-[10px]" : "text-[11px]"}`}>× {tools.length}</span>
        {subjects.length > 0 && (
          <span className={`text-fg-faint truncate ml-1 ${compact ? "text-[10px]" : "text-[11px]"}`}>
            {subjects.join(", ")}
            {moreSubjects > 0 ? ` +${moreSubjects}` : ""}
          </span>
        )}
      </div>
      {/* GSAP 驱动的高度动画 — 替代有 Chrome bug 的 CSS grid-rows 方案 */}
      <div ref={contentRef} className="overflow-hidden">
        <div className="border-t border-border-soft pt-0.5">
          {tools.map((t) => (
            <ToolCard key={t.id} item={t} />
          ))}
        </div>
      </div>
    </div>
  );
}

// scanGroups walks items and replaces runs of ≥2 consecutive same-name tools
// with a single synthetic "group" marker.
export type GroupItem =
  | { kind: "item"; item: Item }
  | { kind: "group"; id: string; name: string; tools: ToolItem[] };

export function scanGroups(items: Item[]): GroupItem[] {
  const result: GroupItem[] = [];
  let i = 0;
  while (i < items.length) {
    const it = items[i];
    if (it.kind !== "tool" || it.parentId) {
      result.push({ kind: "item", item: it });
      i++;
      continue;
    }
    const t = it as ToolItem;
    let j = i + 1;
    while (
      j < items.length &&
      items[j].kind === "tool" &&
      (items[j] as ToolItem).name === t.name
    ) {
      j++;
    }
    const run = items.slice(i, j).filter((x): x is ToolItem => x.kind === "tool");
    if (run.length >= 2) {
      result.push({ kind: "group", id: `grp${i}`, name: t.name, tools: run });
    } else {
      for (const x of run) result.push({ kind: "item", item: x });
    }
    i = j;
  }
  return result;
}
