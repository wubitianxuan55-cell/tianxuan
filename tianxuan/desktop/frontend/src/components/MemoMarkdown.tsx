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
  // 流式时：保留最后 200 字符作为"尾部增量"，其余缓存
  const cacheKey = streaming ? text.slice(0, Math.max(0, text.length - 200)) : text;
  const tail = streaming ? text.slice(cacheKey.length) : "";

  // 缓存主体部分的 Markdown 渲染
  const cached = useMemo(
    () => (cacheKey ? <Markdown text={cacheKey} /> : null),
    [cacheKey]
  );

  // 非流式：完整渲染
  if (!streaming) {
    return (
      <div className="msg__body">
        <Markdown text={text} />
      </div>
    );
  }

  // 流式：缓存主体 + 尾部纯文本（避免频繁 re-parse）+ 光标
  return (
    <div className="msg__body">
      {cached}
      {tail && <span>{tail}</span>}
      <span className="cursor" />
    </div>
  );
});
