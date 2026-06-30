import { memo, useCallback, useRef, useState } from "react";
import { Check, ChevronRight, Copy } from "lucide-react";
import { MemoMarkdown } from "./MemoMarkdown";
import { useT } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";
import { displayReasoningText } from "../lib/reasoningDisplay";
import { useNow } from "../lib/useNow";
import { useTurnStartAt } from "../lib/store";
import type { Item } from "../lib/store";

type AssistantItem = Extract<Item, { kind: "assistant" }>;

// ── 头像图标 ──────────────────────────────────────────────────────────

function AiAvatar({ size = 14 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 2a4 4 0 0 1 4 4v2a4 4 0 0 1-8 0V6a4 4 0 0 1 4-4z" />
      <path d="M8 14v-2a4 4 0 0 1 8 0v2" />
      <path d="M4 22h16a2 2 0 0 0 2-2v-2a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2v2a2 2 0 0 0 2 2z" />
      <circle cx="9" cy="19" r="1" fill="currentColor" />
      <circle cx="15" cy="19" r="1" fill="currentColor" />
    </svg>
  );
}

function UserAvatar({ size = 14 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="8" r="4" />
      <path d="M4 22c0-4.4 3.6-8 8-8s8 3.6 8 8" />
    </svg>
  );
}

// ── 复制按钮 ──────────────────────────────────────────────────────────

function CopyBtn({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const copy = useCallback(async () => {
    try { await navigator.clipboard.writeText(text); } catch { /* noop */ }
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }, [text]);
  return (
    <button
      className="inline-flex items-center gap-1 px-1.5 py-0.5 border-0 rounded bg-transparent text-fg-faint/50 text-[10px] cursor-pointer hover:text-fg hover:bg-bg-soft transition-colors"
      onClick={copy}
      title="复制"
    >
      {copied ? <Check size={10} className="text-ok" /> : <Copy size={10} />}
      {copied ? "已复制" : "复制"}
    </button>
  );
}

// ── 推理图标 ──────────────────────────────────────────────────────────

function BrainIcon({ size = 12 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9.5 2A2.5 2.5 0 0 1 12 4.5v15a2.5 2.5 0 0 1-4.95.5A3.5 3.5 0 0 1 2 17a3.5 3.5 0 0 1 2.67-3.38A3 3 0 0 1 4 8a3 3 0 0 1 3.22-2.98A2.5 2.5 0 0 1 9.5 2Z" />
      <path d="M14.5 2A2.5 2.5 0 0 0 12 4.5v15a2.5 2.5 0 0 0 4.95.5A3.5 3.5 0 0 0 22 17a3.5 3.5 0 0 0-2.67-3.38A3 3 0 0 0 20 8a3 3 0 0 0-3.22-2.98A2.5 2.5 0 0 0 14.5 2Z" />
      <path d="M12 13a3 3 0 0 1-1.5-2.5 3 3 0 0 1 1.5-2.5" />
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
    <div className="flex justify-end my-2 group" data-entrance={turn != null ? `u${turn}` : undefined}>
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

// ── AssistantMessage ──────────────────────────────────────────────────────

export const AssistantMessage = memo(function AssistantMessage({ item }: { item: AssistantItem; onCollapse?: () => void }) {
  const t = useT();
  const compact = useCompact();
  const now = useNow();
  const turnStartAt = useTurnStartAt();
  const reasoningBodyRef = useRef<HTMLDivElement>(null);

  const reasoningRunning = !!(item.streaming && !item.text && item.reasoning);
  const [userToggled, setUserToggled] = useState(false);
  const [reasoningOpenState, setReasoningOpenState] = useState(false);
  const reasoningOpen = userToggled ? reasoningOpenState : !!item.streaming;
  useGSAPCollapse(reasoningBodyRef, reasoningOpen);
  const toggleReasoning = useCallback(() => {
    setUserToggled(true);
    setReasoningOpenState((v) => !v);
  }, []);

  const reasoningDisplay = displayReasoningText(item.reasoning ?? "", {
    streaming: item.streaming ?? false,
    truncateStreaming: true,
  });
  const reasoningTruncated = !!(item.reasoning && reasoningDisplay !== item.reasoning);
  const reasoningLines = item.reasoning ? item.reasoning.split("\n").filter(l => l.trim()).length : 0;

  const elapsed = turnStartAt > 0 ? Math.max(0, now - Math.floor(turnStartAt / 1000)) : 0;
  const elapsedStr = elapsed < 60 ? `${elapsed}s` : `${Math.floor(elapsed / 60)}m${elapsed % 60}s`;

  // 流式处理中的纯文本（不渲染 Markdown）
  const streaming = item.streaming ?? false;

  return (
    <div className="flex justify-start my-2" data-entrance={item.id}>
      <div className="flex items-start gap-2 max-w-[92%] min-w-[120px]">
        {/* AI 头像 */}
        <span className={`shrink-0 rounded-full flex items-center justify-center mt-0.5 ${
          streaming ? "w-7 h-7 bg-accent/20 text-accent animate-pulse" : "w-7 h-7 bg-bg-soft text-fg-faint"
        }`}>
          <AiAvatar size={14} />
        </span>

        <div className="flex-1 min-w-0">
          {/* 推理区 */}
          {item.reasoning && (
            <div className="mb-1.5">
              <button
                type="button"
                className={`flex items-center gap-1.5 w-full px-2.5 py-1 rounded-lg border transition-colors ${
                  reasoningOpen ? "border-accent/20 bg-accent/5" : "border-transparent hover:bg-bg-soft"
                } text-fg-faint text-[11px] cursor-pointer`}
                onClick={toggleReasoning}
                aria-expanded={reasoningOpen}
              >
                <BrainIcon size={11} />
                <span className="font-medium">{reasoningRunning ? t("msg.thinkingRunning") : t("msg.thinking")}</span>
                <span className="text-fg-faint/50 text-[10px] ml-auto">
                  {reasoningRunning
                    ? reasoningTruncated ? `…${reasoningDisplay.length}c` : elapsedStr
                    : `${reasoningLines} 行`}
                </span>
                <ChevronRight
                  className={`transition-transform duration-200 ${reasoningOpen ? "rotate-90" : ""}`}
                  size={11}
                />
              </button>
              <div ref={reasoningBodyRef} style={{ overflow: "hidden" }}>
                <div className={`mt-1 px-2.5 py-1.5 border-l-2 border-accent/20 ml-1 text-fg-dim/80 text-[11px] leading-relaxed whitespace-pre-wrap ${
                  compact ? "max-h-[160px] overflow-y-auto" : ""
                }`}>
                  {reasoningDisplay}
                </div>
              </div>
            </div>
          )}

          {/* 正文区 */}
          {item.text && (
            <div className="min-w-0">
              <MemoMarkdown text={item.text} streaming={streaming} />
            </div>
          )}

          {/* 操作栏 — 流式完成后显示 */}
          {!streaming && item.text && (
            <div className="flex items-center gap-1 mt-1 opacity-0 hover:opacity-100 transition-opacity">
              <CopyBtn text={item.text} />
            </div>
          )}

          {/* 纯推理无正文时显示提示 */}
          {!item.text && item.reasoning && !streaming && (
            <div className="text-fg-faint/40 text-[11px] italic mt-1">{t("msg.reasoningOnly")}</div>
          )}
        </div>
      </div>
    </div>
  );
});
