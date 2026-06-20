import type { DiffProps } from "../DiffView";
import { diffLines } from "../../lib/diff";
import { highlightToHtml } from "../../lib/highlight";

const SIGN: Record<"ctx" | "add" | "del", string> = { ctx: " ", add: "+", del: "-" };

export default function HljsDiff({ original, modified, language, maxHeight }: DiffProps) {
  const rows = diffLines(original, modified);
  const added = rows.filter(r => r.type === "add").length;
  const deleted = rows.filter(r => r.type === "del").length;

  return (
    <div
      className="my-1 bg-bg-soft border border-border-soft rounded-lg font-mono text-[12.5px] leading-[1.55] overflow-auto hljs relative"
      style={maxHeight ? { maxHeight } : undefined}
    >
      {/* Stats badge */}
      {(added > 0 || deleted > 0) && (
        <div className="sticky top-0 z-[2] flex items-center gap-2 px-3 py-0.5 bg-bg-soft/90 border-b border-border-soft text-[10px] tabular-nums">
          {deleted > 0 && <span className="text-del-fg">-{deleted}</span>}
          {added > 0 && <span className="text-add-fg">+{added}</span>}
          <span className="text-fg-faint ml-auto">{rows.length} 行</span>
        </div>
      )}
      {rows.map((r, idx) => (
        <div key={idx} className={`flex gap-2 px-3 whitespace-pre ${r.type === "add" ? "bg-add-bg text-add-fg" : r.type === "del" ? "bg-del-bg text-del-fg" : ""}`}>
          <span className={`w-[0.7em] shrink-0 select-none ${r.type === "ctx" ? "text-fg-faint" : ""}`}>{SIGN[r.type]}</span>
          <code
            className="flex-1 font-mono"
            dangerouslySetInnerHTML={{ __html: highlightToHtml(r.text, language) }}
          />
        </div>
      ))}
    </div>
  );
}
