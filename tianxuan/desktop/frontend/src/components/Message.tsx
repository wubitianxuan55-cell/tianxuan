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
    <div className="msg msg--user">
      <span className="msg__caret">›</span>
      <div className="msg__bubble">{displayText}</div>
      {canRewind && (
        <div className="rewind">
          <button className="rewind__btn" title={t("rewind.label")} onClick={onToggle}>
            ⟲
          </button>
          {open && (
            <div className="rewind__menu">
              <button onClick={() => rewind("both")}>{t("rewind.both")}</button>
              <button onClick={() => rewind("conversation")}>{t("rewind.conversation")}</button>
              <button onClick={() => rewind("code")}>{t("rewind.code")}</button>
              <button onClick={() => rewind("fork")}>{t("rewind.fork")}</button>
              <button onClick={() => rewind("summ-from")}>{t("rewind.summFrom")}</button>
              <button onClick={() => rewind("summ-upto")}>{t("rewind.summUpto")}</button>
            </div>
          )}
        </div>
      )}
    </div>
  );
});

export const AssistantMessage = memo(function AssistantMessage({ item, onCollapse }: { item: AssistantItem; onCollapse?: () => void }) {
  const t = useT();
  const thinkOnly = !!item.reasoning && !item.text;
  const [open, setOpen] = useState(false);
  // 默认折叠，用户手动切换后保持状态
  const effectiveOpen = open;

  return (
    <div className={`msg msg--assistant${thinkOnly ? " msg--thinking" : ""}`}>
      {item.reasoning && (
        <div className="reasoning">
          <button
            className="reasoning__toggle"
            onClick={() => { setOpen((v) => !v); onCollapse?.(); }}
          >
            <ChevronRight
              className={`reasoning__chevron ${effectiveOpen ? "reasoning__chevron--open" : ""}`}
              size={12}
            />
            {item.streaming
              ? `💭 ${t("msg.thinking")}…`
              : `💭 ${t("msg.thinking")} (${item.reasoning.split("\n").filter(l => l.trim()).length} 段)`}
          </button>
          {effectiveOpen && <div className="reasoning__body reasoning__body--open">{item.reasoning}</div>}
        </div>
      )}
      {item.text && (
        <MemoMarkdown text={item.text} streaming={item.streaming} />
      )}

    </div>
  );
});
