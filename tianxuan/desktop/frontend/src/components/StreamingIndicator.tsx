import { useEffect, useRef, useState } from "react";
import type { Item } from "../lib/store";

type Stage = "idle" | "preparing" | "streaming" | "tool_exec" | "stalled";

const stageConfig: Record<Stage, { label: string; barClass: string; dotClass: string; textClass: string }> = {
  idle:      { label: "",              barClass: "",                    dotClass: "",                         textClass: "" },
  preparing: { label: "准备中",         barClass: "bg-warning/60",       dotClass: "bg-warning animate-pulse", textClass: "text-warning" },
  streaming: { label: "生成中",         barClass: "bg-info",             dotClass: "bg-info",                  textClass: "text-info" },
  tool_exec: { label: "工具执行中…",    barClass: "bg-accent/60",        dotClass: "bg-accent animate-pulse",   textClass: "text-accent" },
  stalled:   { label: "仍在处理…",      barClass: "bg-err/50",           dotClass: "bg-err animate-pulse",     textClass: "text-err" },
};

function hasRunningTools(items: Item[]): boolean {
  for (let i = items.length - 1; i >= 0; i--) {
    const it = items[i];
    if (it.kind === "tool" && it.status === "running") return true;
    if (it.kind === "assistant") break; // past the current turn's tools
  }
  return false;
}

function toolCount(items: Item[]): number {
  let n = 0;
  for (let i = items.length - 1; i >= 0; i--) {
    const it = items[i];
    if (it.kind === "tool") n++;
    if (it.kind === "assistant" || it.kind === "user") break;
  }
  return n;
}

/**
 * StreamingIndicator renders a compact "preparing → streaming → tool_exec → stalled"
 * status bar inside the transcript while the model is generating a response.
 * Always occupies layout space (via visibility) to prevent virtual-list jitter.
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
    // Detect tool execution: model emitted tool calls, tools are running.
    if (hasRunningTools(items)) {
      setStage("tool_exec");
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
  }, [running, last?.id, last?.kind === "assistant" && last.streaming, items]);

  const hidden = !running || stage === "idle";
  const cfg = stageConfig[hidden ? "idle" : stage];
  const tc = toolCount(items);

  return (
    <div className={`sticky top-0 z-10 flex items-center gap-2.5 px-3 py-2 border-b border-border-soft bg-bg-soft/50 ${hidden ? "invisible" : ""}`}>
      {/* 滚动色条 — 高 3px，顶对齐 */}
      <div className="absolute top-0 left-0 right-0 h-[3px] bg-border-soft overflow-hidden">
        <div className={`h-full rounded-r-sm animate-pulse ${
          stage === "preparing" ? "bg-warning w-1/4" :
          stage === "tool_exec" ? "bg-accent w-[50%]" :
          stage === "stalled" ? "bg-err w-[40%]" :
          "bg-info w-[60%]"
        }`} />
      </div>

      <span className={`w-2 h-2 rounded-full shrink-0 ${cfg.dotClass}`} />
      <span className={`text-[12px] font-medium ${cfg.textClass}`}>{cfg.label}</span>

      {stage === "tool_exec" && tc > 0 && (
        <span className="text-fg-faint text-[11px] ml-auto tabular-nums">{tc} 个工具</span>
      )}
      {stage === "preparing" && (
        <span className="text-fg-faint text-[11px] ml-auto tabular-nums">15"</span>
      )}
    </div>
  );
}
