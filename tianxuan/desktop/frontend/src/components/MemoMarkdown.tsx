import { memo, useRef, useState, useEffect, useMemo } from "react";
import { Markdown } from "./Markdown";

interface MemoMarkdownProps {
  text: string;
  streaming: boolean;
}

function esc(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

/**
 * findStableCut — 找到"稳定"切分点。
 *
 * 规则（按优先级）：
 *   1. 如果最后一个 \n\n 之后的区间内包含未闭合的代码围栏（奇数个 ```），
 *      回退到代码围栏开始前。
 *   2. 否则在最后一个 \n\n 处切分。
 *   3. 没有 \n\n 则全部视为不稳定。
 *
 * 返回 [stablePrefix, unstableSuffix]。
 */
function findStableCut(text: string): [string, string] {
  const lastGap = text.lastIndexOf("\n\n");
  if (lastGap < 0) return ["", text];

  // Check for unclosed code fence in the suffix
  const suffix = text.slice(lastGap + 2);
  let fenceCount = 0;
  let i = 0;
  while (i < suffix.length) {
    const idx = suffix.indexOf("```", i);
    if (idx < 0) break;
    // A fence is a line that starts with ``` (possibly with a language tag)
    const lineStart = suffix.lastIndexOf("\n", idx - 1);
    const before = lineStart >= 0 ? suffix.slice(lineStart + 1, idx) : suffix.slice(0, idx);
    if (before.trim() === "") {
      fenceCount++;
      if (fenceCount % 2 !== 0) {
        // Found opening fence; roll back to before this block
        const fenceStart = lastGap + 2 + idx;
        const preFenceNL = text.lastIndexOf("\n\n", fenceStart - 1);
        if (preFenceNL >= 0) return [text.slice(0, preFenceNL + 2), text.slice(preFenceNL + 2)];
        return ["", text];
      }
    }
    i = idx + 3;
  }

  return [text.slice(0, lastGap + 2), text.slice(lastGap + 2)];
}

/**
 * renderPending — 对不稳定尾部做简单 HTML 渲染。
 *
 * 处理标题、列表、引用、代码块，与旧版类似但只用于最后的不稳定部分。
 */
function renderPending(text: string): string {
  const lines = text.split("\n");
  let inFence = false;
  const out: string[] = [];

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const isLast = i === lines.length - 1;

    if (line.startsWith("```")) {
      inFence = !inFence;
      out.push(`<span class="text-fg-faint font-mono text-[90%]">${esc(line)}</span>`);
    } else if (inFence) {
      out.push(`<span class="font-mono text-[90%]">${esc(line)}</span>`);
    } else if (/^#{1,4}\s/.test(line)) {
      out.push(`<span class="font-bold">${esc(line)}</span>`);
    } else if (/^[-*+]\s/.test(line)) {
      out.push(`<span class="text-fg-dim">  · ${esc(line.slice(2))}</span>`);
    } else if (/^\d+\.\s/.test(line)) {
      out.push(`<span class="text-fg-dim">  ${esc(line)}</span>`);
    } else if (/^>\s/.test(line)) {
      out.push(`<span class="text-fg-faint">│ ${esc(line.slice(2))}</span>`);
    } else if (line.trim() === "" && !isLast) {
      out.push("");
    } else {
      out.push(esc(line));
    }
  }

  return out.join("\n");
}

/**
 * useProgressiveMarkdown — 渐进式 Markdown。
 *
 * 找到稳定切分点后，前缀用 Markdown 渲染，后缀用简单 HTML。
 */
function useProgressiveMarkdown(text: string): { stable: string; pending: string } {
  return useMemo(() => {
    const [s, p] = findStableCut(text);
    return { stable: s, pending: p };
  }, [text]);
}

/**
 * MemoMarkdown — 流式友好的 Markdown 渲染器。
 *
 * 流式期间：找到稳定段落边界（\n\n），前缀用完整 Markdown 渲染，
 * 未完成尾部用简单样式。流式结束后全量 Markdown 渲染。
 */
export const MemoMarkdown = memo(function MemoMarkdown({ text, streaming }: MemoMarkdownProps) {
  // RAF 节流：每帧最多更新一次
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

  const { stable, pending } = useProgressiveMarkdown(visible);

  // 流式结束：全量 Markdown
  if (!streaming) {
    return (
      <div className="break-words overflow-wrap-break-word">
        <Markdown text={text || ""} />
      </div>
    );
  }

  // 流式中：稳定部分 Markdown + 不稳定部分简单样式 + 闪烁光标
  return (
    <div className="break-words overflow-wrap-break-word">
      {stable && (
        <div className="md text-[14px] leading-relaxed">
          <Markdown text={stable} />
        </div>
      )}
      {pending && (
        <div
          className="!font-sans whitespace-pre-wrap !bg-transparent !p-0 !m-0 !text-[inherit] !border-0 leading-relaxed text-[14px]"
          dangerouslySetInnerHTML={{ __html: renderPending(pending) }}
        />
      )}
      <span
        className="inline-block w-[2px] h-[1em] bg-accent align-middle ml-px animate-pulse"
        aria-hidden
      />
    </div>
  );
}, (prev, next) => prev.text === next.text && prev.streaming === next.streaming);
