import type { DiffProps } from "../DiffView";
import { diffLines } from "../../lib/diff";
import { highlightToHtml } from "../../lib/highlight";

// HljsDiff is the syntax-highlighted default behind the diff seam: an LCS line
// diff with a +/- gutter, each line highlighted in the target language. A real
// editor (Monaco DiffEditor / CodeMirror merge) would replace this via
// DiffView.tsx's lazy import.
const SIGN: Record<"ctx" | "add" | "del", string> = { ctx: " ", add: "+", del: "-" };

export default function HljsDiff({ original, modified, language, maxHeight }: DiffProps) {
  const rows = diffLines(original, modified);
  return (
    <div className="my-1 bg-bg-soft border border-border-soft rounded-lg font-mono text-[12.5px] leading-[1.55] overflow-auto hljs" style={maxHeight ? { maxHeight } : undefined}>
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
