import { useEffect, useRef, useState } from "react";
import type { Item } from "../lib/store";

type Stage = "idle" | "preparing" | "streaming" | "stalled";

const stageConfig: Record<Stage, { label: string; barClass: string; dotClass: string; textClass: string }> = {
  idle:      { label: "",          barClass: "",                    dotClass: "",                          textClass: "" },
  preparing: { label: "准备中",     barClass: "bg-warning/60",       dotClass: "bg-warning animate-pulse",  textClass: "text-warning" },
  streaming: { label: "生成中",     barClass: "bg-info",             dotClass: "bg-info",                   textClass: "text-info" },
  stalled:   { label: "仍在处理…",  barClass: "bg-err/50",           dotClass: "bg-err animate-pulse",      textClass: "text-err" },
};

/**
 * StreamingIndicator renders a compact "preparing → streaming → stalled"
 * status bar inside the transcript while the model is generating a response.
 */
export function StreamingIndicator({
  running,
  items,
}: {
  running: boolean;
  items: Item[];
}) {
  const last = items[items.length - 1];
  const [stage, setStage] = useState<Stage>("idle");
  const stallTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!running) {
      setStage("idle");
      if (stallTimer.current) clearTimeout(stallTimer.current);
      return;
    }
    if (last?.kind === "assistant" && last.streaming) {
      setStage("streaming");
      if (stallTimer.current) clearTimeout(stallTimer.current);
      return;
    }
    setStage("preparing");
    if (stallTimer.current) clearTimeout(stallTimer.current);
    stallTimer.current = setTimeout(() => {
      setStage((s) => (s === "preparing" ? "stalled" : s));
    }, 15_000);
    return () => {
      if (stallTimer.current) clearTimeout(stallTimer.current);
    };
  }, [running, last?.id, last?.kind === "assistant" && last.streaming]);

  if (!running || stage === "idle") return null;

  const cfg = stageConfig[stage];

  return (
    <div className="sticky top-0 z-10 flex items-center gap-2.5 px-3 py-2 border-b border-border-soft bg-bg-soft/50">
      {/* 滚动色条 — 高 3px，顶对齐 */}
      <div className="absolute top-0 left-0 right-0 h-[3px] bg-border-soft overflow-hidden">
        <div className={`h-full rounded-r-sm animate-pulse ${
          stage === "preparing" ? "bg-warning w-1/4" :
          stage === "stalled" ? "bg-err w-[40%]" :
          "bg-info w-[60%]"
        }`} />
      </div>

      <span className={`w-2 h-2 rounded-full shrink-0 ${cfg.dotClass}`} />
      <span className={`text-[12px] font-medium ${cfg.textClass}`}>{cfg.label}</span>

      {stage === "preparing" && (
        <span className="text-fg-faint text-[11px] ml-auto tabular-nums">15"</span>
      )}
    </div>
  );
}
