import { lazy, Suspense } from "react";
import { CopyButton } from "./CopyButton";

export interface EditorProps {
  value: string;
  language?: string;
  readOnly?: boolean;
  maxHeight?: number;
}

// ── EDITOR SEAM (code) ───────────────────────────────────────────────────────
// Every code view in the app renders through this component, so upgrading the
// editor is a one-line change here — swap the lazily-imported module:
//
//   ./editors/HljsCode         current — highlight.js read-only view
//   ./editors/MonacoCode       pnpm add @monaco-editor/react monaco-editor
//   ./editors/CodeMirrorCode   pnpm add @uiw/react-codemirror @codemirror/lang-*
//
// The replacement only has to honor EditorProps. It's lazy-loaded so a heavy
// editor (~MBs) never lands in the initial bundle — it streams in the first time
// a code block or tool result is shown. See desktop/README.md ("Editor seam").
const Impl = lazy(() => import("./editors/HljsCode"));

export function CodeViewer(props: EditorProps) {
  return (
    <div className="relative group/code">
      <CopyButton text={props.value} className="absolute top-[7px] right-[7px] z-[2] opacity-0 group-hover/code:opacity-100 transition-opacity duration-[0.12s]" />
      <Suspense
        fallback={
          <pre className="my-2.5 px-[13px] py-[11px] bg-bg-soft border border-border-soft rounded-lg font-mono text-[12.5px] leading-[1.55] overflow-auto whitespace-pre text-fg opacity-55">
            <code>{props.value}</code>
          </pre>
        }
      >
        <Impl {...props} />
      </Suspense>
    </div>
  );
}
