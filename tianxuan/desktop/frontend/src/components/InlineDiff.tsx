import { useMemo, useState } from "react";
import { Check, Copy, ChevronDown, ChevronRight } from "lucide-react";
import { diffLines, type DiffRow } from "../lib/diff";

// InlineDiff 是一个紧凑、可展开的 unified diff 视图。展示 edit/MultiEdit 等写
// 入工具产生的 before/after 对比，默认折叠到 12 行，点击展开完整内容。
// (Design adopted from DeepSeek-Reasonix-V1.12)
export function InlineDiff({
  before,
  after,
  filename,
  maxRows = 12,
}: {
  before: string;
  after: string;
  filename?: string;
  maxRows?: number;
}) {
  const rows = useMemo(() => diffLines(before, after), [before, after]);
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);

  const visible = open ? rows : rows.slice(0, maxRows);
  const hidden = rows.length - visible.length;

  const addCount = rows.filter((r) => r.type === "add").length;
  const delCount = rows.filter((r) => r.type === "del").length;

  const copy = async () => {
    const text = rows.map((r) => (r.type === "add" ? "+ " : r.type === "del" ? "- " : "  ") + r.text).join("\n");
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1200);
    } catch {
      /* clipboard unavailable */
    }
  };

  return (
    <div className="inline-diff">
      <header className="inline-diff__head">
        <span className="inline-diff__file" title={filename}>
          {filename ?? "diff"}
        </span>
        <span className="inline-diff__stats">
          <span className="inline-diff__add">+{addCount}</span>
          <span className="inline-diff__del">−{delCount}</span>
        </span>
        <span className="inline-diff__spacer" />
        <button type="button" className="inline-diff__btn" onClick={copy} title="Copy diff">
          {copied ? <Check size={11} /> : <Copy size={11} />}
          <span>{copied ? "Copied" : "Copy"}</span>
        </button>
      </header>
      <pre className="inline-diff__body">
        <span className="inline-diff__table">
          {visible.map((r, i) => (
            <span key={i} className={`inline-diff__row inline-diff__row--${r.type}`}>
              <span className="inline-diff__gutter">
                <span className="inline-diff__sign">{r.type === "add" ? "+" : r.type === "del" ? "−" : " "}</span>
              </span>
              <span className="inline-diff__text">{r.text || " "}</span>
            </span>
          ))}
        </span>
      </pre>
      {hidden > 0 && (
        <button
          type="button"
          className="inline-diff__more"
          onClick={() => setOpen(true)}
          aria-expanded={open}
        >
          <ChevronRight size={11} />
          <span>Show {hidden} more lines</span>
        </button>
      )}
      {open && rows.length > maxRows && (
        <button
          type="button"
          className="inline-diff__more"
          onClick={() => setOpen(false)}
          aria-expanded={open}
        >
          <ChevronDown size={11} />
          <span>Collapse</span>
        </button>
      )}
    </div>
  );
}

// Re-export for callers that want to drive the row coloring themselves.
export type { DiffRow };
