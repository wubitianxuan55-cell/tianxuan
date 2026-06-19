import { lazy, Suspense } from "react";

export interface DiffProps {
  original: string;
  modified: string;
  language?: string;
  maxHeight?: number;
}

// ── EDITOR SEAM (diff) ───────────────────────────────────────────────────────
// before/after rendering for edit tools, mirroring CodeViewer's seam. Swap the
// lazily-imported module to upgrade:
//
//   ./editors/HljsDiff         current — highlight.js line diff (LCS)
//   ./editors/MonacoDiff       monaco DiffEditor via @monaco-editor/react
//   ./editors/CodeMirrorMerge  @codemirror/merge
//
// The replacement only has to honor DiffProps. See desktop/README.md.
const Impl = lazy(() => import("./editors/HljsDiff"));

export function DiffView(props: DiffProps) {
  return (
    <Suspense fallback={<pre className="my-2.5 px-[13px] py-[11px] bg-bg-soft border border-border-soft rounded-lg font-mono text-[12.5px] leading-[1.55] overflow-auto whitespace-pre text-fg opacity-55">{props.modified}</pre>}>
      <Impl {...props} />
    </Suspense>
  );
}
