import { memo, useRef } from "react";
import { ChevronRight } from "lucide-react";
import { MemoMarkdown } from "./MemoMarkdown";
import { useT } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";
import { useAutoCollapse } from "../lib/useAutoCollapse";
import { displayReasoningText } from "../lib/reasoningDisplay";
import { useTurnStartAt } from "../lib/store";
import { ProcessBrainIcon } from "./ProcessCard";
import type { Item } from "../lib/store";

type AssistantItem = Extract<Item, { kind: "assistant" }>;

function UserAvatar({ size = 14 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="8" r="4" />
      <path d="M4 22c0-4.4 3.6-8 8-8s8 3.6 8 8" />
    </svg>
  );
}

// ── UserMessage ───────────────────────────────────────────────────────────

export const UserMessage = memo(function UserMessage({
  text,
  turn,
  open,
  onToggle,
  onRewind,
}: {
  text: string;
  turn?: number;
  open?: boolean;
  onToggle?: () => void;
  onRewind?: (turn: number, scope: string) => void;
}) {
  const t = useT();
  const compact = useCompact();
  const canRewind = onRewind != null && turn != null;
  const rewind = (scope: string) => onRewind?.(turn as number, scope);
  const displayText = text.replace(/@\.tianxuan\/attachments\/[^\s]+/g, "[image]");
  return (
    <div className={`flex justify-end group ${compact ? "my-1" : "my-2"}`} data-entrance={turn != null ? `u${turn}` : undefined}>
      <div className={`flex items-start gap-2 max-w-[85%] ${compact ? "min-w-[120px]" : "min-w-[160px]"}`}>
        <div className="flex-1">
          <div className={`rounded-2xl rounded-br-md px-3.5 py-2 bg-accent/10 border border-accent/15 ${
            compact ? "text-[13px]" : "text-[14px]"
          } text-fg leading-relaxed`}>
            {displayText}
          </div>
          {canRewind && (
            <div className="flex justify-end mt-0.5">
              <button
                className="opacity-0 group-hover:opacity-100 px-1.5 py-0.5 border-0 rounded bg-transparent text-fg-faint/50 text-[10px] cursor-pointer hover:text-fg transition-opacity"
                onClick={onToggle}
                title={t("rewind.label")}
              >
                ⟲ 回退
              </button>
              {open && (
                <div className="absolute bottom-full right-0 mb-1 z-30 min-w-[140px] py-1 bg-bg-elev-2 border border-border rounded-lg" style={{boxShadow: "var(--ds-shadow-dropdown)"}}>
                  {(["both","conversation","code","fork","summ-from","summ-upto"] as const).map(scope => {
                    const key = scope === "summ-from" ? "rewind.summFrom" as const : scope === "summ-upto" ? "rewind.summUpto" as const : `rewind.${scope}` as const;
                    return (
                    <button key={scope} className="w-full text-left px-3 py-1.5 border-0 bg-transparent text-fg-dim text-[12px] cursor-pointer hover:bg-bg-soft hover:text-fg" onClick={() => rewind(scope)}>
                      {t(key)}
                    </button>
                    );
                  })}
                </div>
              )}
            </div>
          )}
        </div>
        <span className="shrink-0 w-7 h-7 rounded-full bg-accent/15 flex items-center justify-center text-accent mt-0.5">
          <UserAvatar size={14} />
        </span>
      </div>
    </div>
  );
});

// ── ReasoningProcess ────────────────────────────────────────────────────
// Standalone reasoning fold — renders ABOVE the answer text, not inside it.
// Auto-expands during streaming, auto-collapses when done (unless user
// toggled). Pattern ported from DeepSeek-Reasonix TurnCollapse.

export function ReasoningProcess({
  item,
}: {
  item: AssistantItem;
}) {
  const compact = useCompact();
  const t = useT();
  const turnStartAt = useTurnStartAt();
  const reasoningBodyRef = useRef<HTMLDivElement>(null);
  const reasoningRunning = !!(item.streaming && !item.text);
  const finalElapsedRef = useRef(0);
  const prevRunningRef = useRef(reasoningRunning);

  const { open, toggleOpen } = useAutoCollapse(reasoningRunning);

  useGSAPCollapse(reasoningBodyRef, open);

  // Freeze elapsed when reasoning ends
  if (!reasoningRunning && prevRunningRef.current) {
    finalElapsedRef.current = turnStartAt > 0 ? Math.max(0, Date.now() - turnStartAt) : 0;
  }
  prevRunningRef.current = reasoningRunning;

  const reasoningDisplay = displayReasoningText(item.reasoning ?? "", {
    streaming: item.streaming ?? false,
    truncateStreaming: true,
  });
  const reasoningLines = item.reasoning
    ? item.reasoning.split("\n").filter((l) => l.trim()).length
    : 0;
  const elapsed = reasoningRunning
    ? (turnStartAt > 0 ? Math.max(0, Date.now() - turnStartAt) : 0)
    : finalElapsedRef.current;
  const elapsedStr = elapsed < 60000
    ? `${Math.round(elapsed / 1000)}s`
    : `${Math.floor(elapsed / 60000)}m${Math.round((elapsed % 60000) / 1000)}s`;

  const label = reasoningRunning ? t("msg.thinkingRunning") : t("msg.thinking");
  const meta = reasoningRunning
    ? elapsedStr
    : `${reasoningLines} 行 · ${elapsedStr}`;

  return (
    <div className={`reasoning${compact ? " reasoning--compact" : ""}`}>
      <button
        type="button"
        className="reasoning__head"
        data-running={reasoningRunning ? "" : undefined}
        onClick={toggleOpen}
        aria-expanded={open}
      >
        <ProcessBrainIcon size={12} />
        <span>{label}</span>
        {meta && <span className="reasoning__meta">{meta}</span>}
        <ChevronRight className={`reasoning__chevron${open ? " reasoning__chevron--open" : ""}`} size={12} />
      </button>
      {open && (
        <div ref={reasoningBodyRef} className="reasoning__body">{reasoningDisplay}</div>
      )}
    </div>
  );
}

// ── AssistantMessage ──────────────────────────────────────────────────────
// Renders the assistant's answer text. If the item also carries reasoning,
// ReasoningProcess renders ABOVE as a standalone fold (Reasonix TurnCollapse
// pattern) — reasoning is NOT nested inside the answer bubble.

export const AssistantMessage = memo(function AssistantMessage({
  item,
  hideReasoning = false,
}: {
  item: AssistantItem;
  onCollapse?: () => void;
  hideReasoning?: boolean;
}) {
  const compact = useCompact();
  const streaming = item.streaming ?? false;
  const showReasoning = !hideReasoning && item.reasoning;

  return (
    <div className={`flex justify-start ${compact ? "my-1" : "my-2"}`} data-entrance={item.id}>
      <div className="flex-1 min-w-0">
        {showReasoning && (
        <ReasoningProcess item={item} />
        )}
        {item.text && (
          <div className="min-w-0">
            <MemoMarkdown text={item.text} streaming={streaming} />
          </div>
        )}
      </div>
    </div>
  );
});
