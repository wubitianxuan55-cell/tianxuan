import { useEffect, useRef, useState } from "react";
import { useT } from "../lib/i18n";
import type { WireApproval } from "../lib/types";

export function ApprovalModal({
  approval,
  onAnswer,
  onRevisePlan,
}: {
  approval: WireApproval;
  onAnswer: (allow: boolean, session: boolean) => void;
  onRevisePlan?: (text: string) => void;
}) {
  const t = useT();
  const [revisionOpen, setRevisionOpen] = useState(false);
  const [revisionText, setRevisionText] = useState("");
  const cardRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const isPlanApproval = approval.tool === "exit_plan_mode";

  const choosePlanAction = (key: string) => {
    if (key === "1") onAnswer(false, false);
    else if (key === "2") onAnswer(true, false);
    else if (key === "3") setRevisionOpen((open) => !open);
    else if (key === "Escape") onAnswer(false, false);
  };

  const chooseToolAction = (key: string) => {
    if (key === "1" || key === "Escape") onAnswer(false, false);
    else if (key === "2") onAnswer(true, false);
    else if (key === "3") onAnswer(true, true);
  };

  useEffect(() => {
    cardRef.current?.focus();
  }, [approval.id]);

  useEffect(() => {
    const onKeyDown = (event: globalThis.KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const tag = target?.tagName.toLowerCase();
      if (tag === "input" || tag === "textarea" || target?.isContentEditable) return;
      if (event.key !== "1" && event.key !== "2" && event.key !== "3" && event.key !== "Escape") return;
      event.preventDefault();
      if (isPlanApproval) choosePlanAction(event.key);
      else chooseToolAction(event.key);
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [isPlanApproval, onAnswer]);

  useEffect(() => {
    if (revisionOpen) inputRef.current?.focus();
  }, [revisionOpen]);

  const submitRevision = () => {
    const text = revisionText.trim();
    if (!text) {
      inputRef.current?.focus();
      return;
    }
    onRevisePlan?.(text);
  };

  // The plan is already shown above as the assistant's reply; this is just the gate.
  if (isPlanApproval) {
    return (
      <div className="plan-approval-dock" aria-live="polite">
        <div
          ref={cardRef}
          className="border border-border rounded-[10px] bg-bg-elev shadow-[0_12px_30px_rgba(0,0,0,0.18)] p-3 outline-none focus-visible:border-accent"
          role="dialog"
          aria-modal="false"
          aria-labelledby="plan-approval-title"
          tabIndex={-1}
        >
          <div className="flex items-start justify-between gap-3 mb-2.5">
            <div>
              <div id="plan-approval-title" className="text-fg font-semibold leading-[1.35]">
                {t("approval.planTitle")}
              </div>
              <div className="text-fg-dim text-[12.5px] leading-[1.45] mt-0.5">{t("approval.planNote")}</div>
            </div>
          </div>
          <div className="grid grid-cols-1 gap-1.5">
            <button className="grid grid-cols-[28px_1fr] items-center gap-2.5 w-full min-h-[46px] border border-border-soft rounded-lg bg-bg-soft text-fg p-[7px_9px] text-left cursor-pointer transition-[border-color,background,transform] duration-[0.12s] hover:border-fg-faint hover:bg-bg-elev-2 hover:-translate-y-px" onClick={() => onAnswer(false, false)}>
              <span className="inline-flex items-center justify-center w-[26px] h-[26px] border border-border rounded-[7px] text-fg-dim bg-bg font-mono text-xs font-bold">1</span>
              <span className="flex min-w-0 flex-col gap-px">
                <span className="text-fg text-[13px] font-semibold leading-[1.25]">{t("approval.keepPlanning")}</span>
                <span className="text-fg-faint text-[11.5px] leading-[1.3]">{t("approval.keepPlanningHint")}</span>
              </span>
            </button>
            <button className="grid grid-cols-[28px_1fr] items-center gap-2.5 w-full min-h-[46px] border rounded-lg text-fg p-[7px_9px] text-left cursor-pointer transition-[border-color,background,transform] duration-[0.12s] hover:border-fg-faint hover:bg-bg-elev-2 hover:-translate-y-px border-[color-mix(in_srgb,var(--accent)_55%,var(--border))] bg-accent-soft" onClick={() => onAnswer(true, false)}>
              <span className="inline-flex items-center justify-center w-[26px] h-[26px] border rounded-[7px] font-mono text-xs font-bold border-accent bg-accent text-accent-fg">2</span>
              <span className="flex min-w-0 flex-col gap-px">
                <span className="text-fg text-[13px] font-semibold leading-[1.25]">{t("approval.startExecution")}</span>
                <span className="text-fg-faint text-[11.5px] leading-[1.3]">{t("approval.startExecutionHint")}</span>
              </span>
            </button>
            <button className="grid grid-cols-[28px_1fr] items-center gap-2.5 w-full min-h-[46px] border border-border-soft rounded-lg bg-bg-soft text-fg p-[7px_9px] text-left cursor-pointer transition-[border-color,background,transform] duration-[0.12s] hover:border-fg-faint hover:bg-bg-elev-2 hover:-translate-y-px" onClick={() => setRevisionOpen((open) => !open)}>
              <span className="inline-flex items-center justify-center w-[26px] h-[26px] border border-border rounded-[7px] text-fg-dim bg-bg font-mono text-xs font-bold">3</span>
              <span className="flex min-w-0 flex-col gap-px">
                <span className="text-fg text-[13px] font-semibold leading-[1.25]">{t("approval.revisePlan")}</span>
                <span className="text-fg-faint text-[11.5px] leading-[1.3]">{t("approval.revisePlanHint")}</span>
              </span>
            </button>
          </div>
          {revisionOpen && (
            <div className="mt-2.5 border-t border-border-soft pt-2.5">
              <textarea
                ref={inputRef}
                className="w-full min-h-[72px] resize-y border border-border rounded-lg bg-bg text-fg text-[13px] leading-[1.45] px-2.5 py-[9px] focus:border-accent outline-none placeholder:text-fg-faint"
                value={revisionText}
                rows={3}
                placeholder={t("approval.revisePlanPlaceholder")}
                onChange={(event) => setRevisionText(event.target.value)}
                onKeyDown={(event) => {
                  if ((event.metaKey || event.ctrlKey) && event.key === "Enter") submitRevision();
                  event.stopPropagation();
                }}
              />
              <div className="flex justify-end gap-2 mt-2 flex-wrap">
                <button onClick={() => setRevisionOpen(false)}>
                  {t("common.cancel")}
                </button>
                <button onClick={submitRevision}>
                  {t("approval.sendRevision")}
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="plan-approval-dock" aria-live="polite">
      <div
        ref={cardRef}
        className="border border-border rounded-[10px] bg-bg-elev shadow-[0_12px_30px_rgba(0,0,0,0.18)] p-3 outline-none focus-visible:border-accent"
        role="dialog"
        aria-modal="false"
        aria-labelledby="tool-approval-title"
        tabIndex={-1}
      >
        <div className="flex items-start justify-between gap-3 mb-2.5">
          <div>
            <div id="tool-approval-title" className="text-fg font-semibold leading-[1.35]">
              {t("approval.toolTitle")}
            </div>
            <div className="text-fg-dim text-[12.5px] leading-[1.45] mt-0.5">{t("approval.toolNote")}</div>
          </div>
        </div>
        <div className="flex items-center gap-2 min-h-[34px] mx-0 mb-2 mt-0 border border-border-soft rounded-lg bg-bg-soft py-[5px] px-2">
          <span className="shrink-0 text-fg-faint font-mono text-[11px] uppercase tracking-[0.04em]">{t("approval.toolLabel")}</span>
          <span className="font-mono text-xs font-medium text-fg">{approval.tool}</span>
        </div>
        {approval.subject && <pre className="m-0 mb-2.5 px-[11px] py-[9px] bg-bg-soft border border-border-soft rounded-lg font-mono text-[12.5px] whitespace-pre-wrap break-words max-h-[140px] overflow-auto">{approval.subject}</pre>}
        <div className="grid grid-cols-1 gap-1.5">
          <button className="grid grid-cols-[28px_1fr] items-center gap-2.5 w-full min-h-[46px] border border-border-soft rounded-lg bg-bg-soft text-fg p-[7px_9px] text-left cursor-pointer transition-[border-color,background,transform] duration-[0.12s] hover:border-fg-faint hover:bg-bg-elev-2 hover:-translate-y-px" onClick={() => onAnswer(false, false)}>
            <span className="inline-flex items-center justify-center w-[26px] h-[26px] border border-border rounded-[7px] text-fg-dim bg-bg font-mono text-xs font-bold">1</span>
            <span className="flex min-w-0 flex-col gap-px">
              <span className="text-fg text-[13px] font-semibold leading-[1.25]">{t("approval.deny")}</span>
              <span className="text-fg-faint text-[11.5px] leading-[1.3]">{t("approval.denyHint")}</span>
            </span>
          </button>
          <button className="grid grid-cols-[28px_1fr] items-center gap-2.5 w-full min-h-[46px] border rounded-lg text-fg p-[7px_9px] text-left cursor-pointer transition-[border-color,background,transform] duration-[0.12s] hover:border-fg-faint hover:bg-bg-elev-2 hover:-translate-y-px border-[color-mix(in_srgb,var(--accent)_55%,var(--border))] bg-accent-soft" onClick={() => onAnswer(true, false)}>
            <span className="inline-flex items-center justify-center w-[26px] h-[26px] border rounded-[7px] font-mono text-xs font-bold border-accent bg-accent text-accent-fg">2</span>
            <span className="flex min-w-0 flex-col gap-px">
              <span className="text-fg text-[13px] font-semibold leading-[1.25]">{t("approval.allowOnce")}</span>
              <span className="text-fg-faint text-[11.5px] leading-[1.3]">{t("approval.allowOnceHint")}</span>
            </span>
          </button>
          <button className="grid grid-cols-[28px_1fr] items-center gap-2.5 w-full min-h-[46px] border border-border-soft rounded-lg bg-bg-soft text-fg p-[7px_9px] text-left cursor-pointer transition-[border-color,background,transform] duration-[0.12s] hover:border-fg-faint hover:bg-bg-elev-2 hover:-translate-y-px" onClick={() => onAnswer(true, true)}>
            <span className="inline-flex items-center justify-center w-[26px] h-[26px] border border-border rounded-[7px] text-fg-dim bg-bg font-mono text-xs font-bold">3</span>
            <span className="flex min-w-0 flex-col gap-px">
              <span className="text-fg text-[13px] font-semibold leading-[1.25]">{t("approval.allowSession")}</span>
              <span className="text-fg-faint text-[11.5px] leading-[1.3]">{t("approval.allowSessionHint")}</span>
            </span>
          </button>
        </div>
      </div>
    </div>
  );
}
