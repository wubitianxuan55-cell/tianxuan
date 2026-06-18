import { useEffect, useRef, useState } from "react";
import type { Item } from "../lib/store";

type Stage = "idle" | "preparing" | "streaming" | "stalled";

/**
 * StreamingIndicator renders a compact "preparing → streaming → stalled"
 * status badge inside the transcript while the model is generating a response.
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

  const stageColors: Record<Stage, string> = {
    idle: "",
    preparing: "text-(--color-warning)",
    streaming: "text-(--color-info)",
    stalled: "text-(--color-error)",
  };

  return (
    <div className={`flex items-center gap-2 py-2 px-3 text-[12px] ${stageColors[stage]}`}>
      <span className={`w-2 h-2 rounded-full animate-pulse ${stage === "streaming" ? "bg-(--color-info)" : "bg-(--color-warning)"}`} />
      <span>
        {stage === "preparing" && "Preparing…"}
        {stage === "streaming" && "Streaming"}
        {stage === "stalled" && "Still working…"}
      </span>
      {stage === "preparing" && (
        <span className="text-(--color-fg-faint) text-[11px] ml-auto">15"</span>
      )}
    </div>
  );
}
