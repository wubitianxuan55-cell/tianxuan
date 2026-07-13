import { memo, useRef, useState } from "react";
import { ChevronRight } from "lucide-react";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";
import type { Item } from "../lib/store";
import { ToolCard } from "./ToolCard";

type ToolItem = Extract<Item, { kind: "tool" }>;

/**
 * ReadOnlyBatch — folds consecutive completed read-only tool calls
 * (read_file, ls, grep, glob, web_fetch) into a single summary line.
 * Pattern ported from DeepSeek-Reasonix.
 */
export const ReadOnlyBatch = memo(function ReadOnlyBatch({
  items,
  subcalls,
}: {
  items: ToolItem[];
  subcalls?: Map<string, ToolItem[]>;
}) {
  const [open, setOpen] = useState(false);
  const bodyRef = useRef<HTMLDivElement>(null);
  useGSAPCollapse(bodyRef, open);

  const readCount = items.filter((it) => it.name === "read_file" || it.name === "ls").length;
  const searchCount = items.filter((it) => it.name === "grep" || it.name === "glob" || it.name === "web_fetch").length;

  const parts: string[] = [];
  if (readCount > 0) parts.push(`${readCount} 次读取`);
  if (searchCount > 0) parts.push(`${searchCount} 次搜索`);
  const otherCount = items.length - readCount - searchCount;
  if (otherCount > 0) parts.push(`${otherCount} 其他`);
  const label = parts.join(" · ");

  if (!label || items.length === 0) return null;

  return (
    <div className={`readonly-batch${open ? " readonly-batch--open" : ""}`} data-entrance={items[0]?.id}>
      <button type="button" className="reasoning__head" onClick={() => setOpen((v) => !v)} aria-expanded={open}>
        <ChevronRight className={`reasoning__chevron${open ? " reasoning__chevron--open" : ""}`} size={12} />
        <span className="readonly-batch__label">{label}</span>
      </button>
      <div ref={bodyRef} className="readonly-batch__body">
        {items.map((it) => (
          <ToolCard key={it.id} item={it} subcalls={subcalls?.get(it.id)} />
        ))}
      </div>
    </div>
  );
});
