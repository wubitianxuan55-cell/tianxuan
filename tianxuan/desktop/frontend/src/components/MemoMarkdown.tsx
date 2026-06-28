import { memo, useMemo } from "react";
import { Markdown } from "./Markdown";

interface MemoMarkdownProps {
  text: string;
  streaming: boolean;
}

/**
 * MemoMarkdown — 流式友好的 Markdown 渲染器。
 *
 * 核心思路：流式输出时，文本主体（前 N 字符）不变，只有尾部在增长。
 * 对主体做 useMemo 缓存 react-markdown 的解析结果，尾部用纯文本追加，
 * 避免每次 token 都触发完整的 Markdown re-parse + KaTeX re-render。
 *
 * 非流式时直接全量渲染。
 */
export const MemoMarkdown = memo(function MemoMarkdown({ text, streaming }: MemoMarkdownProps) {
  // 流式时：动态尾部窗口 = max(200, 15% 总长)，缓存主体 Markdown 解析
  const tailLen = streaming ? Math.max(200, Math.floor(text.length * 0.15)) : 0;
  const cacheKey = streaming ? text.slice(0, Math.max(0, text.length - tailLen)) : text;
  const tail = streaming ? text.slice(cacheKey.length) : "";

  // 缓存主体部分的 Markdown 渲染
  const cached = useMemo(
    () => (cacheKey ? <Markdown text={cacheKey} /> : null),
    [cacheKey]
  );

  // 尾部也走 Markdown，用粗糙 key 减少 re-parse：每 50 字符增长才刷新
  const tailCacheKey = tail ? tail.slice(0, Math.max(0, tail.length - (tail.length % 50))) : "";
  const tailRendered = useMemo(
    () => (tail ? <Markdown text={tail} /> : null),
    [tailCacheKey]
  );

  // 非流式：完整渲染
  if (!streaming) {
    return (
      <div className="bg-bg-elev rounded-md max-w-full px-3.5 py-2 break-words overflow-wrap-break-word [&>p:first-child]:mt-0 [&>p:last-child]:mb-0">
        <Markdown text={text} />
      </div>
    );
  }

  // 流式：缓存主体 + 尾部 Markdown（粗糙缓存，50字符粒度）+ 闪烁光标
  return (
    <div className="bg-bg-elev rounded-md max-w-full px-3.5 py-2 break-words overflow-wrap-break-word [&>p:first-child]:mt-0 [&>p:last-child]:mb-0" style={{color: "var(--fg)", WebkitTextFillColor: "var(--fg)"}}>
      {cached}
      {tailRendered}
      <span className="inline-block w-1.5 h-[1.05em] ml-0.5 bg-accent align-text-bottom animate-[cursor-blink_1s_steps(1)_infinite]" />
    </div>
  );
});
