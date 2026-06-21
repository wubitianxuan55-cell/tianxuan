import { useEffect, useRef, useState } from "react";
import { useT } from "../lib/i18n";
import type { WireApproval } from "../lib/types";

// ─── 审批选项按钮的共享布局 ─────────────────────────────────────

const btnBase = "grid grid-cols-[28px_1fr] items-center gap-2.5 w-full min-h-[46px] rounded-lg text-fg p-[7px_9px] text-left cursor-pointer transition-all duration-[0.12s]";

function PlanBtn({ num, active, title, hint, onClick }: {
  num: number; active?: boolean; title: string; hint: string; onClick: () => void;
}) {
  return (
    <button
      className={`${btnBase} border ${
        active
          ? "border-[color-mix(in_srgb,var(--accent)_55%,var(--border))] bg-accent-soft hover:border-accent hover:scale-[1.01]"
          : "border-border-soft bg-bg-soft hover:border-fg-faint hover:bg-bg-elev-2 hover:scale-[1.01]"
      }`}
      onClick={onClick}
    >
      <span className={`inline-flex items-center justify-center w-[26px] h-[26px] border rounded-lg font-mono text-xs font-bold ${
        active ? "border-accent bg-accent text-accent-fg" : "border-border text-fg-dim bg-bg"
      }`}>
        {num}
      </span>
      <span className="flex min-w-0 flex-col gap-px">
        <span className="text-fg text-[13px] font-semibold leading-[1.25]">{title}</span>
        <span className="text-fg-faint text-[11.5px] leading-[1.3]">{hint}</span>
      </span>
    </button>
  );
}

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

  useEffect(() => { cardRef.current?.focus(); }, [approval.id]);

  useEffect(() => {
    const onKeyDown = (event: globalThis.KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (target?.tagName === "INPUT" || target?.tagName === "TEXTAREA" || target?.isContentEditable) return;
      if (!["1", "2", "3", "Escape"].includes(event.key)) return;
      event.preventDefault();
      if (isPlanApproval) {
        if (event.key === "1") onAnswer(false, false);
        else if (event.key === "2") onAnswer(true, false);
        else if (event.key === "3") setRevisionOpen((o) => !o);
        else if (event.key === "Escape") onAnswer(false, false);
      } else {
        if (event.key === "1" || event.key === "Escape") onAnswer(false, false);
        else if (event.key === "2") onAnswer(true, false);
        else if (event.key === "3") onAnswer(true, true);
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [isPlanApproval, onAnswer]);

  useEffect(() => { if (revisionOpen) inputRef.current?.focus(); }, [revisionOpen]);

  const submitRevision = () => {
    if (!revisionText.trim()) { inputRef.current?.focus(); return; }
    onRevisePlan?.(revisionText.trim());
  };

  return (
    <div className="plan-approval-dock" aria-live="polite">
      <div
        ref={cardRef}
        className="border border-border rounded-xl bg-bg-elev shadow-[0_8px_32px_rgba(0,0,0,0.14)] p-4 outline-none focus-visible:border-accent transition-shadow duration-200 hover:shadow-[0_12px_40px_rgba(0,0,0,0.18)]"
        role="dialog" aria-modal="false" tabIndex={-1}
        aria-labelledby={isPlanApproval ? "plan-approval-title" : "tool-approval-title"}
      >
        {/* ── 标题 ── */}
        <div className="mb-3">
          <div id={isPlanApproval ? "plan-approval-title" : "tool-approval-title"} className="text-fg font-semibold leading-[1.35] text-[14px]">
            {isPlanApproval ? t("approval.planTitle") : t("approval.toolTitle")}
          </div>
          <div className="text-fg-dim text-[12.5px] leading-[1.45] mt-0.5">
            {isPlanApproval ? t("approval.planNote") : t("approval.toolNote")}
          </div>
        </div>

        {/* ── 工具名标签（仅非 plan 模式）── */}
        {!isPlanApproval && (
          <div className="flex items-center gap-2 min-h-[34px] mb-3 border border-border-soft rounded-lg bg-bg-soft py-[5px] px-2">
            <span className="shrink-0 text-fg-faint font-mono text-[11px] uppercase tracking-[0.04em]">{t("approval.toolLabel")}</span>
            <span className="font-mono text-xs font-medium text-fg">{approval.tool}</span>
          </div>
        )}

        {/* ── 操作预览 ── */}
        {approval.subject && (
          <pre className="m-0 mb-3 px-[11px] py-[9px] bg-bg-soft border border-border-soft rounded-lg font-mono text-[12.5px] whitespace-pre-wrap break-words max-h-[140px] overflow-auto">
            {approval.subject}
          </pre>
        )}

        {/* ── 选项按钮 ── */}
        <div className="flex flex-col gap-1.5">
          {isPlanApproval ? (
            <>
              <PlanBtn num={1} title={t("approval.keepPlanning")} hint={t("approval.keepPlanningHint")} onClick={() => onAnswer(false, false)} />
              <PlanBtn num={2} active title={t("approval.startExecution")} hint={t("approval.startExecutionHint")} onClick={() => onAnswer(true, false)} />
              <PlanBtn num={3} title={t("approval.revisePlan")} hint={t("approval.revisePlanHint")} onClick={() => setRevisionOpen((o) => !o)} />
            </>
          ) : (
            <>
              <PlanBtn num={1} title={t("approval.deny")} hint={t("approval.denyHint")} onClick={() => onAnswer(false, false)} />
              <PlanBtn num={2} active title={t("approval.allowOnce")} hint={t("approval.allowOnceHint")} onClick={() => onAnswer(true, false)} />
              <PlanBtn num={3} title={t("approval.allowSession")} hint={t("approval.allowSessionHint")} onClick={() => onAnswer(true, true)} />
            </>
          )}
        </div>

        {/* ── 修订计划 ── */}
        {revisionOpen && (
          <div className="mt-3 border-t border-border-soft pt-3">
            <textarea
              ref={inputRef}
              className="w-full min-h-[72px] resize-y border border-border rounded-lg bg-bg text-fg text-[13px] leading-[1.45] px-2.5 py-[9px] focus:border-accent outline-none placeholder:text-fg-faint"
              value={revisionText}
              rows={3}
              placeholder={t("approval.revisePlanPlaceholder")}
              onChange={(e) => setRevisionText(e.target.value)}
              onKeyDown={(e) => {
                if ((e.metaKey || e.ctrlKey) && e.key === "Enter") submitRevision();
                e.stopPropagation();
              }}
            />
            <div className="flex justify-end gap-2 mt-2 flex-wrap">
              <button
                className="px-4 py-2 border border-border-soft rounded-lg bg-transparent text-fg-dim text-[12.5px] cursor-pointer transition-all duration-150 hover:text-fg hover:border-border hover:bg-bg-soft active:scale-[0.98]"
                onClick={() => setRevisionOpen(false)}
              >{t("common.cancel")}</button>
              <button
                className="px-4 py-2 border-0 rounded-lg bg-accent text-accent-fg text-[12.5px] font-semibold cursor-pointer transition-all duration-150 hover:brightness-110 active:scale-[0.98] disabled:opacity-40 disabled:cursor-default"
                onClick={submitRevision}
                disabled={!revisionText.trim()}
              >{t("approval.sendRevision")}</button>
              <span className="w-full text-fg-faint text-[10px] text-right">Ctrl+Enter 发送</span>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
