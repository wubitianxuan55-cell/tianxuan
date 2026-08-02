// UnifiedDiffView — Codex review pane 的 diff 渲染：把内核返回的 unified
// diff 解析为结构化行，删除行红色、新增行绿色、hunk 头深色分隔。行号列
// 按新旧行追踪。（与 DiffView 不同：DiffView 接收 original/modified 两段
// 文本由客户端比对；本组件直接渲染现成的 unified diff 字符串。）
import { useMemo, useState } from "react";
import { MessageSquare, Plus } from "lucide-react";
import { parseUnifiedDiff } from "../lib/diffParser";
import type { ReviewComment } from "../lib/reviewComments";

export function UnifiedDiffView({
  diff,
  comments = [],
  onAddComment,
}: {
  diff: string;
  comments?: ReviewComment[];
  onAddComment?: (line: number, text: string) => void;
}) {
  const lines = useMemo(() => parseUnifiedDiff(diff), [diff]);
  const commentsByLine = useMemo(() => {
    const m = new Map<number, ReviewComment[]>();
    for (const c of comments) {
      const arr = m.get(c.line) ?? [];
      arr.push(c);
      m.set(c.line, arr);
    }
    return m;
  }, [comments]);
  const [draftLine, setDraftLine] = useState<number | null>(null);
  const [draftText, setDraftText] = useState("");

  const startDraft = (line: number) => {
    setDraftLine(line);
    setDraftText("");
  };
  const saveDraft = () => {
    if (draftLine !== null && draftText.trim() && onAddComment) {
      onAddComment(draftLine, draftText);
    }
    setDraftLine(null);
    setDraftText("");
  };

  return (
    <div className="h-full overflow-auto bg-bg font-mono text-[11.5px] leading-[1.6]">
      <table className="w-full border-collapse">
        <tbody>
          {lines.map((l, i) => {
            const num = (v?: number) => (v === undefined ? "" : String(v));
            if (l.kind === "header") {
              return (
                <tr key={i} className="bg-bg-soft/50 text-fg-faint">
                  <td className="w-10 px-2 text-right select-none" />
                  <td className="w-10 px-2 text-right select-none" />
                  <td className="px-3 py-0 whitespace-pre-wrap break-all">{l.text}</td>
                </tr>
              );
            }
            if (l.kind === "hunk") {
              return (
                <tr key={i} className="bg-accent/8 text-accent">
                  <td className="w-10 px-2 text-right select-none" />
                  <td className="w-10 px-2 text-right select-none" />
                  <td className="px-3 py-0 whitespace-pre-wrap break-all">@@ {l.text} @@</td>
                </tr>
              );
            }
            const cls =
              l.kind === "add"
                ? "bg-add-bg text-fg"
                : l.kind === "del"
                  ? "bg-del-bg text-fg"
                  : "text-fg-dim";
            const line = l.newLine ?? l.oldLine;
            const lineComments = line === undefined ? [] : (commentsByLine.get(line) ?? []);
            return (
              <tr key={i} className={`${cls} group`}>
                <td className="w-10 px-2 text-right select-none text-fg-faint/70 tabular-nums">
                  {l.kind === "add" ? "" : num(l.oldLine)}
                </td>
                <td className="w-10 px-2 text-right select-none text-fg-faint/70 tabular-nums">
                  {l.kind === "del" ? "" : num(l.newLine)}
                </td>
                <td className="px-3 py-0 whitespace-pre-wrap break-all">
                  {draftLine === line && line !== undefined && onAddComment ? (
                    <input
                      autoFocus
                      className="w-full border border-accent/50 rounded bg-bg-elev text-fg text-[11.5px] px-1.5 py-0.5 outline-none font-mono"
                      placeholder={`${l.kind === "add" ? "+" : l.kind === "del" ? "-" : " "} ${l.text}`}
                      value={draftText}
                      onChange={(e) => setDraftText(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") { e.preventDefault(); saveDraft(); }
                        if (e.key === "Escape") { setDraftLine(null); setDraftText(""); }
                      }}
                      onBlur={saveDraft}
                    />
                  ) : (
                    <>
                      <span className="inline-flex items-center gap-1.5">
                        <span>{l.kind === "add" ? "+ " : l.kind === "del" ? "- " : "  "}{l.text}</span>
                        {lineComments.length > 0 && (
                          <span
                            className="inline-flex items-center gap-0.5 text-[10px] text-warn shrink-0 cursor-help"
                            title={lineComments.map((c) => `${c.line}: ${c.text}`).join("\n")}
                          >
                            <MessageSquare size={11} fill="currentColor" />
                            {lineComments.length}
                          </span>
                        )}
                      </span>
                      {line !== undefined && onAddComment && (
                        <button
                          type="button"
                          className="ml-1.5 inline-flex items-center justify-center w-4 h-4 align-middle border-0 rounded bg-transparent text-fg-faint opacity-0 group-hover:opacity-100 hover:text-accent hover:bg-bg-soft cursor-pointer transition-opacity"
                          title="行内评论"
                          onClick={() => startDraft(line)}
                        >
                          <Plus size={11} />
                        </button>
                      )}
                    </>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
