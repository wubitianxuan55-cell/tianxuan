import { memo } from "react";
import { Markdown } from "./Markdown";

interface MemoMarkdownProps {
  text: string;
  streaming: boolean;
}

/**
 * MemoMarkdown — 流式友好的 Markdown 渲染器。
 *
 * 全量渲染 + React.memo 缓存。contain: layout style 隔离布局，
 * 防止流式文本增长时触发全页重排。
 */
export const MemoMarkdown = memo(function MemoMarkdown({ text, streaming }: MemoMarkdownProps) {
  return (
    <div
      className="bg-bg-elev rounded-md max-w-full px-3.5 py-2 break-words overflow-wrap-break-word [&>p:first-child]:mt-0 [&>p:last-child]:mb-0"
      style={streaming ? { color: "var(--fg)", WebkitTextFillColor: "var(--fg)", contain: "layout style" } : undefined}
    >
      <Markdown text={text || ""} />
    </div>
  );
});
