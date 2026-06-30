import { memo, useCallback, useState } from "react";
import ReactMarkdown from "react-markdown";
import type { Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import rehypeKatex from "rehype-katex";
import { Check, Copy } from "lucide-react";
import "katex/dist/katex.min.css";
import { CodeViewer } from "./CodeViewer";
import { openExternal } from "../lib/bridge";

// ── 代码块复制按钮 ──────────────────────────────────────────────────

function CodeBlockHeader({ language, text }: { language?: string; text: string }) {
  const [copied, setCopied] = useState(false);
  const copy = useCallback(async () => {
    try { await navigator.clipboard.writeText(text); } catch { /* noop */ }
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }, [text]);
  return (
    <div className="flex items-center justify-between px-3 py-1 bg-bg-soft/80 border-b border-border-soft/50 rounded-t-md text-[10px] select-none">
      <span className="text-fg-faint/60 font-mono font-medium uppercase tracking-wider">
        {language || "text"}
      </span>
      <button
        className="inline-flex items-center gap-1 px-1.5 py-0.5 border-0 rounded bg-transparent text-fg-faint/40 cursor-pointer hover:text-fg transition-colors"
        onClick={copy}
        title="复制代码"
      >
        {copied ? <Check size={10} className="text-ok" /> : <Copy size={10} />}
        {copied ? "已复制" : "复制"}
      </button>
    </div>
  );
}

// ── Markdown 组件 ────────────────────────────────────────────────────

const components: Components = {
  pre: ({ children }) => <>{children}</>,
  code: ({ className, children }) => {
    const text = String(children ?? "").replace(/\n$/, "");
    const match = /language-([\w-]+)/.exec(className ?? "");
    const lang = match?.[1];
    const isBlock = match !== null || text.includes("\n");
    if (isBlock) {
      return (
        <div className="my-3 rounded-md border border-border-soft overflow-hidden">
          <CodeBlockHeader language={lang} text={text} />
          <CodeViewer value={text} language={lang} maxHeight={400} />
        </div>
      );
    }
    return <code className="px-1 py-0.5 rounded bg-bg-soft text-fg text-[0.9em] font-mono border border-border-soft/50">{children}</code>;
  },
  a: ({ href, children }) => (
    <a href={href} onClick={(e) => { e.preventDefault(); if (href) openExternal(href); }}
      className="text-accent hover:underline">
      {children}
    </a>
  ),
  table: ({ children }) => (
    <div className="my-3 overflow-x-auto rounded-md border border-border-soft">
      <table className="min-w-full text-[13px]">{children}</table>
    </div>
  ),
  th: ({ children }) => (
    <th className="px-3 py-2 text-left text-[11px] font-semibold text-fg-dim bg-bg-soft border-b border-border-soft">
      {children}
    </th>
  ),
  td: ({ children }) => (
    <td className="px-3 py-2 border-b border-border-soft/50 text-fg">{children}</td>
  ),
  blockquote: ({ children }) => (
    <blockquote className="my-2 pl-3 border-l-[3px] border-accent/30 text-fg-dim/80 italic">
      {children}
    </blockquote>
  ),
  hr: () => <hr className="my-4 border-border-soft" />,
  ol: ({ children }) => <ol className="my-2 pl-5 list-decimal text-fg space-y-0.5">{children}</ol>,
  ul: ({ children }) => <ul className="my-2 pl-5 list-disc text-fg space-y-0.5">{children}</ul>,
  h1: ({ children }) => <h1 className="mt-5 mb-2 text-[18px] font-bold text-fg">{children}</h1>,
  h2: ({ children }) => <h2 className="mt-4 mb-1.5 text-[16px] font-bold text-fg">{children}</h2>,
  h3: ({ children }) => <h3 className="mt-3 mb-1 text-[14px] font-semibold text-fg">{children}</h3>,
  p: ({ children }) => <p className="my-1.5 leading-relaxed text-fg">{children}</p>,
};

// ── 数学公式标准化 ──────────────────────────────────────────────────

function normalizeMath(s: string): string {
  const lb = "\x00LB\x00";
  let r = s.replace(/\\\\\[/g, lb);
  r = r
    .replace(/\\\[/g, () => "$$")
    .replace(/\\\]/g, () => "$$")
    .replace(/\\\(/g, () => "$")
    .replace(/\\\)/g, () => "$");
  r = r.replace(/\x00LB\x00/g, "\\\\[");
  const vert = (m: string) => m.replace(/\|/g, "\\vert ");
  r = r.replace(/\$\$([\s\S]*?)\$\$/g, (_m, m) => `$$${vert(m)}$$`);
  r = r.replace(/\$([^$\n]+)\$/g, (_m, m) => `$${vert(m)}$`);
  return r;
}

export const Markdown = memo(function Markdown({ text }: { text: string }) {
  return (
    <div className="md text-[14px] leading-relaxed">
      <ReactMarkdown remarkPlugins={[remarkGfm, remarkMath]} rehypePlugins={[rehypeKatex]} components={components}>
        {normalizeMath(text)}
      </ReactMarkdown>
    </div>
  );
});
