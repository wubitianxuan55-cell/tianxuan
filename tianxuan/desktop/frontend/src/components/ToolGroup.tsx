import { useState } from "react";
import { ChevronRight, FolderOpen } from "lucide-react";
import { ToolCard } from "./ToolCard";
import { subjectOf } from "../lib/tools";
import type { Item } from "../lib/store";

type ToolItem = Extract<Item, { kind: "tool" }>;

// ToolGroup collapses consecutive same-name tool calls into a single row.
// V5.30: all tools are grouped (not just read-only). The collapsed row shows
// the tool name × count + subjects summary; clicking expands individual cards.
export function ToolGroup({ tools, onCollapse }: { tools: ToolItem[]; onCollapse?: () => void }) {
  const [open, setOpen] = useState(false);
  if (tools.length === 0) return null;
  const t = tools[0];

  // 提取所有工具的操作目标（最多5个）
  const subjects = tools
    .map(t => subjectOf(t.name, t.args))
    .filter(Boolean)
    .slice(0, 5);
  const moreCount = tools.length - subjects.length;

  return (
    <div className="my-1 border border-border-soft rounded-lg overflow-hidden">
      <div
        className="flex items-center gap-2 px-2.5 py-1.5 cursor-pointer hover:bg-bg-soft text-fg-dim"
        onClick={() => { setOpen((v) => !v); onCollapse?.(); }}
      >
        <ChevronRight
          className={`shrink-0 transition-transform duration-150 ${open ? "rotate-90" : ""}`}
          size={13}
        />
        <FolderOpen className="shrink-0 text-fg-faint" size={14} />
        <span className="font-mono text-xs text-fg font-medium">{t.name}</span>
        <span className="text-[11px] text-fg-faint font-mono">× {tools.length}</span>
        {subjects.length > 0 && (
          <span className="text-[11px] text-fg-faint truncate ml-1">
            {subjects.join(", ")}
            {moreCount > 0 ? ` +${moreCount}` : ""}
          </span>
        )}
      </div>
      {open && (
        <div className="border-t border-border-soft pt-0.5">
          {tools.map((t) => (
            <ToolCard key={t.id} item={t} />
          ))}
        </div>
      )}
    </div>
  );
}

// scanGroups walks items and replaces runs of ≥2 consecutive same-name tools
// with a single synthetic "group" marker. V5.30: all tools (not just read-only)
// are grouped. The marker has kind "group" and carries the grouped items in `tools`.
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
    // V5.30: 所有工具都分组（包括读写工具），减少刷屏
    // Collect consecutive same-name tools.
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
