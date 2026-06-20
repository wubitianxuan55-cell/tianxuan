import { lazy, Suspense } from "react";

export interface DiffProps {
  original: string;
  modified: string;
  language?: string;
  maxHeight?: number;
}

// ── EDITOR SEAM (diff) ───────────────────────────────────────────────────────
const Impl = lazy(() => import("./editors/HljsDiff"));

export function DiffView(props: DiffProps) {
  const aLines = props.original ? props.original.split("\n").length : 0;
  const bLines = props.modified ? props.modified.split("\n").length : 0;

  return (
    <Suspense
      fallback={
        <pre className="my-2.5 px-[13px] py-[11px] bg-bg-soft border border-border-soft rounded-lg font-mono text-[12.5px] leading-[1.55] overflow-auto whitespace-pre text-fg opacity-55">
          <span className="text-[10px] text-fg-faint uppercase tracking-wider">
            {aLines} → {bLines} 行
          </span>
          {"\n"}
          {props.modified.slice(0, 2000)}
          {props.modified.length > 2000 && "\n…"}
        </pre>
      }
    >
      <Impl {...props} />
    </Suspense>
  );
}
