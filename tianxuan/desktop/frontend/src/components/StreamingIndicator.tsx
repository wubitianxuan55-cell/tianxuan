import { useEffect, useRef, useState } from "react";
import type { Item } from "../lib/store";

type Stage = "idle" | "preparing" | "streaming" | "stalled";

/**
 * StreamingIndicator renders a compact "preparing → streaming → stalled"
 * status badge inside the transcript while the model is generating a response.
 *
 * - **preparing**: shown when the turn just started and no tokens have arrived yet.
 * - **streaming**: shown once the first reasoning/text token arrives.
 * - **stalled**: shown after 15s in preparing without any token.
 *
 * It sits at the bottom of the transcript, below the last user message but
 * before the first assistant token.
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

    // If the last item is already an assistant (streaming), we're streaming.
    if (last?.kind === "assistant" && last.streaming) {
      setStage("streaming");
      if (stallTimer.current) clearTimeout(stallTimer.current);
      return;
    }

    // If we're running and the last item is user (or empty), we're preparing.
    // Show the preparing state and start a stall timer.
    setStage("preparing");
    if (stallTimer.current) clearTimeout(stallTimer.current);
    stallTimer.current = setTimeout(() => {
      // Only escalate to stalled if we're still in preparing state.
      setStage((s) => (s === "preparing" ? "stalled" : s));
    }, 15_000);

    return () => {
      if (stallTimer.current) clearTimeout(stallTimer.current);
    };
  }, [running, last?.id, last?.kind === "assistant" && last.streaming]);

  if (!running || stage === "idle") return null;

  return (
    <div className={`streaming-indicator streaming-indicator--${stage}`}>
      <span className="streaming-indicator__dot" />
      <span className="streaming-indicator__label">
        {stage === "preparing" && "Preparing…"}
        {stage === "streaming" && "Streaming"}
        {stage === "stalled" && "Still working…"}
      </span>
      {stage === "preparing" && <span className="streaming-indicator__eta">15"</span>}
    </div>
  );
}
