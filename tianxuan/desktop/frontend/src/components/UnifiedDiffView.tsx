// UnifiedDiffView — Codex review pane 的 diff 渲染：把内核返回的 unified
// diff 解析为结构化行，删除行红色、新增行绿色、hunk 头深色分隔。行号列
// 按新旧行追踪。（与 DiffView 不同：DiffView 接收 original/modified 两段
// 文本由客户端比对；本组件直接渲染现成的 unified diff 字符串。）
import { useMemo } from "react";
import { parseUnifiedDiff } from "../lib/diffParser";

export function UnifiedDiffView({ diff }: { diff: string }) {
  const lines = useMemo(() => parseUnifiedDiff(diff), [diff]);
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
            return (
              <tr key={i} className={cls}>
                <td className="w-10 px-2 text-right select-none text-fg-faint/70 tabular-nums">
                  {l.kind === "add" ? "" : num(l.oldLine)}
                </td>
                <td className="w-10 px-2 text-right select-none text-fg-faint/70 tabular-nums">
                  {l.kind === "del" ? "" : num(l.newLine)}
                </td>
                <td className="px-3 py-0 whitespace-pre-wrap break-all">
                  {l.kind === "add" ? "+ " : l.kind === "del" ? "- " : "  "}
                  {l.text}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
