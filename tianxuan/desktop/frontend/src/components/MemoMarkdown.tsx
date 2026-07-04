import { memo, useRef, useState, useEffect } from "react";
import { Markdown } from "./Markdown";

interface MemoMarkdownProps {
  text: string;
  streaming: boolean;
}

function esc(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

const CURSOR = '<span class="inline-block w-[2px] h-[1em] bg-accent align-middle ml-px animate-pulse" />';

interface CacheState {
  html: string;
  processedLen: number;
  inFence: boolean;
}

// processLine renders a single line with the current fence state.
// Returns [html, newInFence].
function processLine(line: string, inFence: boolean): [string, boolean] {
  if (line.startsWith("```")) {
    return [`<span class="text-fg-faint font-mono text-[90%]">${esc(line)}</span>\n`, !inFence];
  }
  if (inFence) {
    return [`<span class="font-mono text-[90%]">${esc(line)}</span>\n`, true];
  }
  if (/^#{1,4}\s/.test(line)) {
    return [`<span class="font-bold">${esc(line)}</span>\n`, false];
  } else if (/^[-*+]\s/.test(line)) {
    return [`<span class="text-fg-dim">  · ${esc(line.slice(2))}</span>\n`, false];
  } else if (/^\d+\.\s/.test(line)) {
    return [`<span class="text-fg-dim">  ${esc(line)}</span>\n`, false];
  } else if (/^>\s/.test(line)) {
    return [`<span class="text-fg-faint">│ ${esc(line.slice(2))}</span>\n`, false];
  } else if (line.trim() === "") {
    return ["\n", false];
  }
  return [esc(line) + "\n", false];
}

// processPending renders the incomplete last line (no newline at end).
function processPending(line: string, inFence: boolean): string {
  if (inFence) {
    return `<span class="font-mono text-[90%]">${esc(line)}</span>`;
  }
  if (/^#{1,4}\s/.test(line)) {
    return `<span class="font-bold">${esc(line)}</span>`;
  } else if (/^[-*+]\s/.test(line)) {
    return `<span class="text-fg-dim">  · ${esc(line.slice(2))}</span>`;
  } else if (/^\d+\.\s/.test(line)) {
    return `<span class="text-fg-dim">  ${esc(line)}</span>`;
  } else if (/^>\s/.test(line)) {
    return `<span class="text-fg-faint">│ ${esc(line.slice(2))}</span>`;
  }
  return esc(line);
}

/**
 * useStreamingPreview — 增量流式预览。
 *
 * 只处理新增的完整行（以 \n 结尾），已处理部分缓存为 HTML。
 * 最后一行（可能不完整）每次都重新渲染，但只有一行，O(1)。
 * 避免 streamingPreview 每次 O(n) 遍历全部文本的问题。
 */
function useStreamingPreview(text: string, streaming: boolean): string {
  const cache = useRef<CacheState>({ html: "", processedLen: 0, inFence: false });

  // Reset cache when streaming starts
  const prevStreaming = useRef(streaming);
  if (streaming && !prevStreaming.current) {
    cache.current = { html: "", processedLen: 0, inFence: false };
  }
  prevStreaming.current = streaming;

  if (!streaming) return "";

  const delta = text.slice(cache.current.processedLen);
  if (delta.length === 0) {
    // No new content; re-render pending line only
    const pending = text.slice(cache.current.processedLen);
    if (!pending) return cache.current.html + CURSOR;
    return cache.current.html + processPending(pending, cache.current.inFence) + CURSOR;
  }

  // Process up to the last complete line boundary
  const lastNL = delta.lastIndexOf("\n");
  const toProcess = lastNL >= 0 ? delta.slice(0, lastNL + 1) : "";
  const newProcessedLen = cache.current.processedLen + (lastNL >= 0 ? lastNL + 1 : 0);

  let out = "";
  let fence = cache.current.inFence;
  if (toProcess) {
    for (const line of toProcess.split("\n").slice(0, -1)) {
      const [html, newFence] = processLine(line, fence);
      out += html;
      fence = newFence;
    }
  }

  cache.current.html += out;
  cache.current.processedLen = newProcessedLen;
  cache.current.inFence = fence;

  // Render the pending (incomplete) last line
  const pending = text.slice(newProcessedLen);
  const pendingHtml = pending ? processPending(pending, fence) : "";

  return cache.current.html + pendingHtml + CURSOR;
}

/**
 * MemoMarkdown — 流式友好的 Markdown 渲染器。
 *
 * 流式期间显示增量轻量结构预览（保留标题、代码块、列表的视觉结构），
 * 只处理新增行，避免每个 text chunk 触发 O(n) 全量重处理。
 * 流式结束后切换为完整 Markdown 渲染。
 */
export const MemoMarkdown = memo(function MemoMarkdown({ text, streaming }: MemoMarkdownProps) {
  // RAF 节流：每帧最多更新一次，避免高频 chunk 压垮浏览器
  const [visible, setVisible] = useState(text);
  const rafRef = useRef(0);

  useEffect(() => {
    if (!streaming) {
      setVisible(text);
      return;
    }
    cancelAnimationFrame(rafRef.current);
    rafRef.current = requestAnimationFrame(() => setVisible(text));
    return () => cancelAnimationFrame(rafRef.current);
  }, [text, streaming]);

  const previewHtml = useStreamingPreview(visible, streaming);

  return (
    <div className="break-words overflow-wrap-break-word">
      {streaming ? (
        <div
          className="!font-sans whitespace-pre-wrap !bg-transparent !p-0 !m-0 !text-[inherit] !border-0 leading-relaxed"
          dangerouslySetInnerHTML={{ __html: previewHtml }}
        />
      ) : (
        <Markdown text={text || ""} />
      )}
    </div>
  );
}, (prev, next) => prev.text === next.text && prev.streaming === next.streaming);
