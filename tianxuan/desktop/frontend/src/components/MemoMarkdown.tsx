import { memo } from "react";
import { Markdown } from "./Markdown";

interface MemoMarkdownProps {
  text: string;
  streaming: boolean;
}

/**
 * MemoMarkdown — 流式友好的 Markdown 渲染器。
 *
 * 流式期间显示纯文本（<pre>），避免每个 text chunk 触发全量
 * ReactMarkdown + KaTeX + highlight.js 重解析（这是流式闪烁的根源）。
 * 流式结束后切换为完整 Markdown 渲染。
 */
export const MemoMarkdown = memo(function MemoMarkdown({ text, streaming }: MemoMarkdownProps) {
  return (
    <div
      className="bg-bg-elev rounded-md max-w-full px-3.5 py-2 break-words overflow-wrap-break-word [&>p:first-child]:mt-0 [&>p:last-child]:mb-0"
      style={streaming ? { color: "var(--fg)", WebkitTextFillColor: "var(--fg)", contain: "layout style" } : undefined}
    >
      {streaming ? (
        <pre className="!font-sans whitespace-pre-wrap !bg-transparent !p-0 !m-0 !text-[inherit]">{text || ""}</pre>
      ) : (
        <Markdown text={text || ""} />
      )}
    </div>
  );
});
