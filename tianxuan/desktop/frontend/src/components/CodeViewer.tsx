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

  return (
    <div className="relative group/code">
      <CopyButton
        text={props.value}
        className="absolute top-1.5 right-1.5 z-[3] opacity-0 group-hover/code:opacity-100 transition-opacity duration-[var(--dur-fast)]"
      />
      <Suspense
        fallback={
          <pre className="my-2.5 px-3 py-2.5 bg-bg-soft border border-border-soft rounded-lg font-mono text-[12.5px] leading-[1.55] overflow-auto whitespace-pre text-fg opacity-55">
            <span className="text-[10px] text-fg-faint/60 uppercase tracking-wider">
              {props.language && props.language !== "plaintext" ? props.language : "code"}
              {lineCount > 0 && ` · ${lineCount} 行`}
            </span>
            {"\n"}
            <code>{props.value.slice(0, 2000)}{props.value.length > 2000 && "\n…"}</code>
          </pre>
        }
      >
        <Impl {...props} />
      </Suspense>
    </div>
  );
}
