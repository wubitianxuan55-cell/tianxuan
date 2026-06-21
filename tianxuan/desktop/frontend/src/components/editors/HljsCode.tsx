import type { EditorProps } from "../CodeViewer";
import { highlightToHtml } from "../../lib/highlight";

export default function HljsCode({ value, language, maxHeight }: EditorProps) {
  const html = highlightToHtml(value, language);
  const langLabel = language && language !== "plaintext" && language !== "text" ? language : undefined;
  const lineCount = value ? value.split("\n").length : 0;

  return (
    <div className="relative">
      {/* Language badge — top-left */}
      {langLabel && (
        <span className="absolute top-1.5 left-2.5 text-[10px] font-mono text-fg-faint/50 uppercase tracking-wider pointer-events-none select-none z-[1]">
          {langLabel}
        </span>
      )}
      <pre
        className="my-2.5 px-3 pt-[26px] pb-2.5 bg-bg-soft border border-border-soft rounded-lg font-mono text-[12.5px] leading-[1.55] overflow-auto whitespace-pre text-fg hljs"
        data-lang={language}
        style={maxHeight ? { maxHeight } : undefined}
      >
        <code dangerouslySetInnerHTML={{ __html: html }} />
      </pre>
      {/* Line count badge — top-right */}
      {lineCount > 3 && (
        <span className="absolute top-1.5 right-8 text-[10px] font-mono text-fg-faint/40 tabular-nums pointer-events-none select-none z-[1]">
          {lineCount} 行
        </span>
      )}
    </div>
  );
}
