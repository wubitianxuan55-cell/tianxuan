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
 * 流式末尾带闪烁光标。
 */
export const MemoMarkdown = memo(function MemoMarkdown({ text, streaming }: MemoMarkdownProps) {
  return (
    <div className="break-words overflow-wrap-break-word">
      {streaming ? (
        <pre className="!font-sans whitespace-pre-wrap !bg-transparent !p-0 !m-0 !text-[inherit] !border-0 leading-relaxed">
          {text || ""}
          <span className="inline-block w-[2px] h-[1em] bg-accent align-middle ml-px animate-pulse" />
        </pre>
      ) : (
        <Markdown text={text || ""} />
      )}
    </div>
  );
});
