import { memo, useState } from "react";
import { ChevronRight } from "lucide-react";
import { MemoMarkdown } from "./MemoMarkdown";
import { useT } from "../lib/i18n";
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
  open?: boolean; // whether this message's rewind menu is the open one (lifted to Transcript)
  onToggle?: () => void;
  onRewind?: (turn: number, scope: string) => void;
}) {
  const t = useT();
  const canRewind = onRewind != null && turn != null;
  const rewind = (scope: string) => onRewind?.(turn as number, scope);
  const displayText = text.replace(/@\.tianxuan\/attachments\/[^\s]+/g, "[image]");
  return (
    <div className="relative flex flex-row-reverse items-center gap-2 px-4 py-2.5 group">
      <span className="text-accent font-mono font-semibold text-lg leading-none shrink-0">›</span>
      <div className="bg-accent-soft text-fg rounded-xl rounded-br-md px-4 py-2 text-[14px] leading-relaxed whitespace-pre-wrap break-words max-w-[85%]">{displayText}</div>
      {canRewind && (
        <div className="relative shrink-0 ml-auto">
          <button className="opacity-0 group-hover:opacity-100 w-6 h-6 flex items-center justify-center border-0 rounded bg-transparent text-fg-faint cursor-pointer hover:text-fg hover:bg-bg-elev transition-opacity" title={t("rewind.label")} onClick={onToggle}>
            ⟲
          </button>
          {open && (
            <div className="absolute bottom-full right-0 mb-1 z-30 min-w-[140px] py-1 bg-bg-elev-2 border border-border rounded-lg shadow-lg">
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
  const thinkOnly = !!item.reasoning && !item.text;
  const [open, setOpen] = useState(false);
  const reasoningLines = item.reasoning ? item.reasoning.split("\n").filter(l => l.trim()).length : 0;

  return (
    <div className={`relative py-1 ${thinkOnly ? "bg-bg-soft rounded-md px-3 py-2" : ""}`}>
      {item.reasoning && (
        <div className="mb-1">
          <button
            className="flex items-center gap-1.5 text-fg-faint text-[11px] font-medium bg-transparent border-0 cursor-pointer py-0.5 hover:text-fg-dim select-none"
            onClick={() => setOpen((v) => !v)}
          >
            <ChevronRight
              className={`shrink-0 transition-transform duration-200 ${open ? "rotate-90" : ""}`}
              size={12}
            />
            <span className="text-fg-faint/70">
              {item.streaming ? `${t("msg.thinking")}…` : `${t("msg.thinking")} (${reasoningLines} 段)`}
            </span>
          </button>
          <div
            className={`grid transition-all duration-300 ease-in-out ${
              open ? "grid-rows-[1fr] opacity-100 mt-2" : "grid-rows-[0fr] opacity-0 mt-0"
            }`}
          >
            <div className="overflow-hidden">
              <div className="pl-3 border-l-2 border-border-soft text-fg-dim text-xs leading-relaxed whitespace-pre-wrap max-h-[500px] overflow-y-auto">
                {item.reasoning}
              </div>
            </div>
          </div>
        </div>
      )}
      {item.text && (
        <MemoMarkdown text={item.text} streaming={item.streaming} />
      )}

    </div>
  );
});
