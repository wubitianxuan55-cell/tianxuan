import { memo, useCallback, useRef, useState } from "react";
import { ChevronRight } from "lucide-react";
import { MemoMarkdown } from "./MemoMarkdown";
import { useT } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";
import { displayReasoningText } from "../lib/reasoningDisplay";
import type { Item } from "../lib/store";

type AssistantItem = Extract<Item, { kind: "assistant" }>;

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
    <div className={`relative flex flex-row-reverse items-center gap-2 group ${compact ? "px-3 py-1.5" : "px-4 py-2.5"}`}>
      <span className={`text-accent font-mono font-semibold leading-none shrink-0 ${compact ? "text-base" : "text-lg"}`}>›</span>
      <div className={`bg-accent-soft text-fg rounded-xl rounded-br-md shadow-sm leading-relaxed whitespace-pre-wrap break-words max-w-[85%] ${compact ? "px-3 py-1.5 text-[13px]" : "px-4 py-2 text-[14px]"}`}>{displayText}</div>
      {canRewind && (
        <div className="relative shrink-0 ml-auto">
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

export const AssistantMessage = memo(function AssistantMessage({ item }: { item: AssistantItem; onCollapse?: () => void }) {
  const t = useT();
  const compact = useCompact();
  const thinkOnly = !!item.reasoning && !item.text;
  const reasoningBodyRef = useRef<HTMLDivElement>(null);

  // 流式推理阶段自动展开，完成后折叠（用户手动操作后以用户为准）
  const reasoningRunning = item.streaming && !item.text;
  const [userToggled, setUserToggled] = useState(false);
  const [reasoningOpenState, setReasoningOpenState] = useState(false);
  const reasoningOpen = userToggled ? reasoningOpenState : reasoningRunning;
  useGSAPCollapse(reasoningBodyRef, reasoningOpen);
  const toggleReasoning = useCallback(() => {
    setUserToggled(true);
    setReasoningOpenState((v) => !v);
  }, []);

  const reasoningLines = item.reasoning ? item.reasoning.split("\n").filter(l => l.trim()).length : 0;

  // 流式推理文本截断——防止超长思维链卡顿 UI
  const reasoningDisplay = displayReasoningText(item.reasoning ?? "", {
    streaming: item.streaming ?? false,
    truncateStreaming: true,
  });
  const reasoningTruncated = item.reasoning && reasoningDisplay !== item.reasoning;

  return (
    <div className={`relative ${compact ? "py-0.5" : "py-1"} ${thinkOnly ? "bg-bg-soft rounded-md px-3 py-2" : ""}`}>
      {item.reasoning && (
        <div className="mb-1">
          <button
            type="button"
            className="flex items-center gap-1.5 text-fg-faint text-[11px] font-medium bg-transparent border-0 cursor-pointer py-0.5 hover:text-fg-dim select-none"
            data-running={reasoningRunning ? "" : undefined}
            onClick={toggleReasoning}
            aria-expanded={reasoningOpen}
          >
            <ChevronRight
              className={`shrink-0 transition-transform duration-200 ${reasoningOpen ? "rotate-90" : ""}`}
              size={12}
            />
            <span>{t("msg.thinking")}</span>
            <span className="text-fg-faint/50">
              {reasoningRunning
                ? reasoningTruncated
                  ? `…${reasoningDisplay.length} 字符`
                  : t("msg.thinkingRunning") ?? "…"
                : `${reasoningLines} 段`}
            </span>
          </button>
          <div ref={reasoningBodyRef} style={{ overflow: "hidden" }}>
            <div className={`pl-3 ml-1 border-l-2 border-accent/30 bg-accent/[0.03] rounded-sm text-fg-dim/80 text-xs leading-relaxed whitespace-pre-wrap overflow-y-auto ${compact ? "max-h-[300px] py-1" : "max-h-[500px] py-1.5"}`}>
              {reasoningDisplay}
            </div>
          </div>
        </div>
      )}
      {item.text && (
        <div className={item.streaming ? "ds-shiny-text" : ""}>
        <MemoMarkdown text={item.text} streaming={item.streaming} />
        </div>
      )}
    </div>
  );
});
