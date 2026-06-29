import { memo, useCallback, useRef, useState } from "react";
import { ChevronRight } from "lucide-react";
import { MemoMarkdown } from "./MemoMarkdown";
import { useT } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";
import { displayReasoningText } from "../lib/reasoningDisplay";
import { useNow } from "../lib/useNow";
import { useTurnStartAt } from "../lib/store";
import type { Item } from "../lib/store";

type AssistantItem = Extract<Item, { kind: "assistant" }>;

// ── Brain icon (compact, legible at 12px) ────────────────────────────────
function BrainIcon({ size = 13 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9.5 2A2.5 2.5 0 0 1 12 4.5v15a2.5 2.5 0 0 1-4.95.5A3.5 3.5 0 0 1 2 17a3.5 3.5 0 0 1 2.67-3.38A3 3 0 0 1 4 8a3 3 0 0 1 3.22-2.98A2.5 2.5 0 0 1 9.5 2Z" />
      <path d="M14.5 2A2.5 2.5 0 0 0 12 4.5v15a2.5 2.5 0 0 0 4.95.5A3.5 3.5 0 0 0 22 17a3.5 3.5 0 0 0-2.67-3.38A3 3 0 0 0 20 8a3 3 0 0 0-3.22-2.98A2.5 2.5 0 0 0 14.5 2Z" />
      <path d="M12 14a4 4 0 0 1-2-3.5 4 4 0 0 1 2-3.5" />
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
    <div className={`msg msg--user ${compact ? "px-3 py-1.5" : "px-4 py-2.5"}`} data-entrance={turn != null ? `u${turn}` : undefined}>
      <div className={`msg__body ${compact ? "px-3 py-1.5" : "px-4 py-2"}`}>
        <div className={`msg__text ${compact ? "text-[13px]" : "text-[14px]"}`}>{displayText}</div>
      </div>
      {canRewind && (
        <div className="relative shrink-0 mt-1">
          <button className="opacity-0 group-hover:opacity-100 w-6 h-6 flex items-center justify-center border-0 rounded bg-transparent text-fg-faint cursor-pointer hover:text-fg hover:bg-bg-elev transition-all duration-[var(--dur-fast)] active:scale-90" title={t("rewind.label")} onClick={onToggle}>
            ⟲
          </button>
          {open && (
            <div className="absolute bottom-full right-0 mb-1 z-30 min-w-[140px] py-1 bg-bg-elev-2 border border-border rounded-lg" style={{boxShadow: "var(--ds-shadow-dropdown)"}}>
              <button className="w-full text-left px-3 py-1.5 border-0 bg-transparent text-fg-dim text-[12px] cursor-pointer hover:bg-bg-soft hover:text-fg" onClick={() => rewind("both")}>{t("rewind.both")}</button>
              <button className="w-full text-left px-3 py-1.5 border-0 bg-transparent text-fg-dim text-[12px] cursor-pointer hover:bg-bg-soft hover:text-fg" onClick={() => rewind("conversation")}>{t("rewind.conversation")}</button>
              <button className="w-full text-left px-3 py-1.5 border-0 bg-transparent text-fg-dim text-[12px] cursor-pointer hover:bg-bg-soft hover:text-fg" onClick={() => rewind("code")}>{t("rewind.code")}</button>
              <button className="w-full text-left px-3 py-1.5 border-0 bg-transparent text-fg-dim text-[12px] cursor-pointer hover:bg-bg-soft hover:text-fg" onClick={() => rewind("fork")}>{t("rewind.fork")}</button>
              <button className="w-full text-left px-3 py-1.5 border-0 bg-transparent text-fg-dim text-[12px] cursor-pointer hover:bg-bg-soft hover:text-fg" onClick={() => rewind("summ-from")}>{t("rewind.summFrom")}</button>
              <button className="w-full text-left px-3 py-1.5 border-0 bg-transparent text-fg-dim text-[12px] cursor-pointer hover:bg-bg-soft hover:text-fg" onClick={() => rewind("summ-upto")}>{t("rewind.summUpto")}</button>
            </div>
          )}
        </div>
      )}
    </div>
  );
});

// ── AssistantMessage ──────────────────────────────────────────────────────
export const AssistantMessage = memo(function AssistantMessage({ item }: { item: AssistantItem; onCollapse?: () => void }) {
  const t = useT();
  const compact = useCompact();
  const now = useNow();
  const turnStartAt = useTurnStartAt();
  const thinkOnly = !!item.reasoning && !item.text;
  const reasoningBodyRef = useRef<HTMLDivElement>(null);

  // 推理运行中：正文未到但仍在流式
  const reasoningRunning = !!(item.streaming && !item.text && item.reasoning);
  const [userToggled, setUserToggled] = useState(false);
  const [reasoningOpenState, setReasoningOpenState] = useState(false);
  const reasoningOpen = userToggled ? reasoningOpenState : false;
  useGSAPCollapse(reasoningBodyRef, reasoningOpen);
  const toggleReasoning = useCallback(() => {
    setUserToggled(true);
    setReasoningOpenState((v) => !v);
  }, []);

  // 文本截断保护
  const reasoningDisplay = displayReasoningText(item.reasoning ?? "", {
    streaming: item.streaming ?? false,
    truncateStreaming: true,
  });
  const reasoningTruncated = !!(item.reasoning && reasoningDisplay !== item.reasoning);
  const reasoningLines = item.reasoning ? item.reasoning.split("\n").filter(l => l.trim()).length : 0;

  // 实时计时
  const elapsed = turnStartAt > 0 ? Math.max(0, now - Math.floor(turnStartAt / 1000)) : 0;
  const elapsedStr = elapsed < 60 ? `${elapsed}s` : `${Math.floor(elapsed / 60)}m${elapsed % 60}s`;

  return (
    <div className={`msg msg--assistant ${compact ? "py-0.5" : "py-1"} ${thinkOnly ? "bg-bg-soft rounded-md px-2.5" : ""}`} data-entrance={item.id}>
      {item.reasoning && (
        <div className="reasoning">
          <button
            type="button"
            className="reasoning__head"
            data-running={reasoningRunning ? "" : undefined}
            onClick={toggleReasoning}
            aria-expanded={reasoningOpen}
          >
            <span className="reasoning__icon"><BrainIcon size={12} /></span>
            <span className="reasoning__label">{reasoningRunning ? t("msg.thinkingRunning") : t("msg.thinking")}</span>
            <span className="reasoning__meta">
              {reasoningRunning
                ? reasoningTruncated
                  ? `…${reasoningDisplay.length}c`
                  : elapsedStr
                : `${reasoningLines} 行`}
            </span>
            <ChevronRight
              className={`reasoning__chevron ${reasoningOpen ? "reasoning__chevron--open" : ""}`}
              size={12}
            />
          </button>
          <div ref={reasoningBodyRef} style={{ overflow: "hidden" }}>
            <div className={`reasoning__body ${compact ? "max-h-[200px]" : ""}`}>
              {reasoningDisplay}
            </div>
          </div>
        </div>
      )}
      {item.text && (
        <div className="msg__body">
          <MemoMarkdown text={item.text} streaming={item.streaming} />
        </div>
      )}
    </div>
  );
});
