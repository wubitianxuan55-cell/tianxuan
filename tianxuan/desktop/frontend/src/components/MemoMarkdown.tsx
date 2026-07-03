import { memo } from "react";
import { Markdown } from "./Markdown";

interface MemoMarkdownProps {
  text: string;
  streaming: boolean;
}

/**
 * Lightweight streaming preview that preserves basic Markdown visual structure
 * (headings, fenced code blocks, list indentation) without the full
 * ReactMarkdown + KaTeX + highlight.js cost. This reduces the visual jump when
 * streaming completes and the full Markdown renderer takes over.
 */
function streamingPreview(text: string): string {
  let out = "";
  let inFence = false;
  for (const line of text.split("\n")) {
    if (line.startsWith("```")) {
      inFence = !inFence;
      out += `<span class="text-fg-faint font-mono text-[90%]">${esc(line)}</span>\n`;
      continue;
    }
    if (inFence) {
      out += `<span class="font-mono text-[90%]">${esc(line)}</span>\n`;
      continue;
    }
    if (/^#{1,4}\s/.test(line)) {
      out += `<span class="font-bold">${esc(line)}</span>\n`;
    } else if (/^[-*+]\s/.test(line)) {
      out += `<span class="text-fg-dim">  · ${esc(line.slice(2))}</span>\n`;
    } else if (/^\d+\.\s/.test(line)) {
      out += `<span class="text-fg-dim">  ${esc(line)}</span>\n`;
    } else if (/^>\s/.test(line)) {
      out += `<span class="text-fg-faint">│ ${esc(line.slice(2))}</span>\n`;
    } else if (line.trim() === "") {
      out += "\n";
    } else {
      out += esc(line) + "\n";
    }
  }
  return out;
}

function esc(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

/**
 * MemoMarkdown — 流式友好的 Markdown 渲染器。
 *
 * 流式期间显示轻量结构预览（保留标题、代码块、列表的视觉结构），
 * 避免每个 text chunk 触发全量 ReactMarkdown + KaTeX + highlight.js 重解析。
 * 流式结束后切换为完整 Markdown 渲染。流式末尾带闪烁光标。
 */
export const MemoMarkdown = memo(function MemoMarkdown({ text, streaming }: MemoMarkdownProps) {
  return (
    <div className="break-words overflow-wrap-break-word">
      {streaming ? (
        <div
          className="!font-sans whitespace-pre-wrap !bg-transparent !p-0 !m-0 !text-[inherit] !border-0 leading-relaxed"
          dangerouslySetInnerHTML={{ __html: (streamingPreview(text || "") + '<span class="inline-block w-[2px] h-[1em] bg-accent align-middle ml-px animate-pulse" />') }}
        />
      ) : (
        <Markdown text={text || ""} />
      )}
    </div>
  );
}, (prev, next) => prev.text === next.text && prev.streaming === next.streaming);
