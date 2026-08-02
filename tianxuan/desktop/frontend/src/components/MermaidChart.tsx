// MermaidChart — Codex Visualizations 蒸馏：把 agent 输出的 ```mermaid
// 代码块渲染为 SVG 图表（流程图/时序图/架构图等）。mermaid 按需懒加载
// （约 2MB，避免拖慢首屏），深色主题适配 tianxuan 科幻风。
import { useEffect, useRef, useState } from "react";

let mermaidReady: Promise<typeof import("mermaid").default> | null = null;

function getMermaid(): Promise<typeof import("mermaid").default> {
  if (!mermaidReady) {
    mermaidReady = import("mermaid").then((m) => {
      m.default.initialize({
        startOnLoad: false,
        securityLevel: "strict",
        theme: "base",
        themeVariables: {
          background: "transparent",
          primaryColor: "rgba(34,211,238,0.12)",
          primaryTextColor: "#cbd5e1",
          primaryBorderColor: "rgba(34,211,238,0.40)",
          lineColor: "#64748b",
          secondaryColor: "rgba(139,92,246,0.12)",
          tertiaryColor: "rgba(148,163,184,0.10)",
          fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
          fontSize: "13px",
        },
      });
      return m.default;
    });
  }
  return mermaidReady;
}

export function MermaidChart({ code }: { code: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const [error, setError] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [id] = useState(() => `mmd-${Math.random().toString(36).slice(2, 10)}`);

  useEffect(() => {
    let cancelled = false;
    void getMermaid()
      .then((mermaid) => mermaid.render(id, code))
      .then(({ svg }) => {
        if (cancelled || !ref.current) return;
        ref.current.innerHTML = svg;
        setLoaded(true);
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e));
      });
    return () => { cancelled = true; };
  }, [id, code]);

  return (
    <div className="my-3 rounded-md border border-border-soft overflow-x-auto bg-bg-soft/30 p-3">
      <div ref={ref} className="flex justify-center min-w-[320px]" />
      {error && (
        <pre className="mt-2 text-[11px] text-err font-mono whitespace-pre-wrap break-all">{error}</pre>
      )}
      {!loaded && !error && (
        <div className="py-4 text-center text-fg-faint text-[11px] font-mono">渲染图表…</div>
      )}
    </div>
  );
}
