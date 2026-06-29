import { lazy, Suspense } from "react";
import { CopyButton } from "./CopyButton";

export interface EditorProps {
  value: string;
  language?: string;
  readOnly?: boolean;
  maxHeight?: number;
}

// ── EDITOR SEAM (code) ───────────────────────────────────────────────────────
const Impl = lazy(() => import("./editors/HljsCode"));

export function CodeViewer(props: EditorProps) {
  const lineCount = props.value ? props.value.split("\n").length : 0;
  const langLabel = props.language && props.language !== "plaintext" && props.language !== "text" ? props.language : undefined;

  return (
    <div className="my-2.5 rounded-lg border border-border-soft bg-bg-soft overflow-hidden">
      {/* ── top bar: language + line count + copy ── */}
      <div className="flex items-center gap-2 px-3 py-1.5 border-b border-border-soft/60 bg-bg-elev/40">
        <span className="text-[10px] font-mono text-fg-faint/60 uppercase tracking-wider select-none">
          {langLabel ?? "text"}
        </span>
        {lineCount > 0 && (
          <span className="text-[10px] font-mono text-fg-faint/40 tabular-nums select-none">
            {lineCount} 行
          </span>
        )}
        <div className="ml-auto">
          <CopyButton text={props.value} showInlineLabel={false} ghost />
        </div>
      </div>
      {/* ── code body ── */}
      <Suspense
        fallback={
          <pre className="px-3 py-2.5 font-mono text-[12.5px] leading-[1.55] overflow-auto whitespace-pre text-fg opacity-55">
            <code>{props.value.slice(0, 2000)}{props.value.length > 2000 && "\n…"}</code>
          </pre>
        }
      >
        <Impl {...props} />
      </Suspense>
    </div>
  );
}
