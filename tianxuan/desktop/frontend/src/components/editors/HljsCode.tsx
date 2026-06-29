import type { EditorProps } from "../CodeViewer";
import { highlightToHtml } from "../../lib/highlight";

export default function HljsCode({ value, language, maxHeight }: EditorProps) {
  const html = highlightToHtml(value, language);

  return (
    <pre
      className="px-3 py-2.5 font-mono text-[12.5px] leading-[1.55] overflow-auto whitespace-pre text-fg hljs"
      data-lang={language}
      style={maxHeight ? { maxHeight } : undefined}
    >
      <code dangerouslySetInnerHTML={{ __html: html }} />
    </pre>
  );
}
