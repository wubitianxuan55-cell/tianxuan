import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Check, ChevronDown, FileCode, ListChecks, FileEdit, Link, GripVertical, MessageSquare } from "lucide-react";
import { useT } from "../lib/i18n";
import { Markdown } from "./Markdown";
import { parsePlan, type ParsedPlan } from "../lib/planParser";
import { useDraggableCard } from "../lib/useDraggableCard";
import type { QuestionAnswer, WireAsk } from "../lib/types";

// ── InlineMarkdown ───────────────────────────────────────────────────────────

function InlineMarkdown({ text }: { text: string }) {
  const parts = text.split(/(\*\*[^*]+\*\*|\*[^*]+\*|`[^`]+`)/g);
  return (
    <>
      {parts.map((part, i) => {
        if (part.startsWith("**") && part.endsWith("**"))
          return <strong key={i}>{part.slice(2, -2)}</strong>;
        if (part.startsWith("*") && part.endsWith("*") && part.length > 2)
          return <em key={i}>{part.slice(1, -1)}</em>;
        if (part.startsWith("`") && part.endsWith("`"))
          return <code key={i} className="text-[11px] bg-bg px-1 py-0.5 rounded font-mono">{part.slice(1, -1)}</code>;
        return <>{part}</>;
      })}
    </>
  );
}

// ── PlanSummaryBar ──────────────────────────────────────────────────────────

function PlanSummaryBar({ parsed }: { parsed: ParsedPlan }) {
  const t = useT();
  const maxShow = 8;
  const overflow = parsed.allFiles.length - maxShow;

  return (
    <div className="flex items-center gap-2 px-5 py-2">
      <div className="flex items-center gap-1.5 text-[12px] text-fg-dim bg-bg-soft rounded-lg px-2.5 py-1 shrink-0">
        <ListChecks size={13} />
        <span>{t(parsed.steps.length === 1 ? "plan.stepCount_one" : "plan.stepCount_other", { n: parsed.steps.length })}</span>
      </div>
      <div className="flex items-center gap-1.5 text-[12px] text-fg-dim bg-bg-soft rounded-lg px-2.5 py-1 shrink-0">
        <FileCode size={13} />
        <span>{t(parsed.allFiles.length === 1 ? "plan.fileCount_one" : "plan.fileCount_other", { n: parsed.allFiles.length })}</span>
      </div>
      {parsed.allFiles.length > 0 && (
        <div className="flex-1 flex items-center gap-1 overflow-hidden justify-end min-w-0">
          {parsed.allFiles.slice(0, maxShow).map((f) => {
            const name = f.split("/").pop() || f;
            return (
              <span key={f} className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-accent/10 text-accent text-[11px] font-medium whitespace-nowrap truncate max-w-[140px]">{name}</span>
            );
          })}
          {overflow > 0 && (
            <span className="text-fg-faint text-[10px] shrink-0">+{overflow}</span>
          )}
        </div>
      )}
    </div>
  );
}

// ── StepCard ─────────────────────────────────────────────────────────────────

function resolveDepNumber(dependsOn: string, steps: ParsedPlan["steps"]): string | null {
  const n = parseInt(dependsOn, 10);
  if (!isNaN(n) && steps.some((s) => s.number === n)) return `步骤 ${n}`;
  if (dependsOn === "无" || dependsOn === "None") return null;
  const m = dependsOn.match(/步骤\s*(\d+)/);
  if (m && steps.some((s) => s.number === parseInt(m[1], 10))) return `步骤 ${m[1]}`;
  return dependsOn || null;
}

function StepCard({ step, steps, total }: { step: ParsedPlan["steps"][0]; steps: ParsedPlan["steps"]; total: number }) {
  const [open, setOpen] = useState(false);
  const t = useT();
  const depLabel = step.dependsOn ? resolveDepNumber(step.dependsOn, steps) : null;
  const isDependent = depLabel && depLabel !== "无" && depLabel !== "None";
  const hasDetails = step.change || step.dependsOn;

  return (
    <div className="rounded-lg border border-border-soft bg-bg-soft overflow-hidden">
      <button
        className="w-full flex items-start gap-2.5 px-3 py-2.5 text-left cursor-pointer transition-colors duration-150 hover:bg-bg/40"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        <span className="shrink-0 flex items-center justify-center w-5 h-5 rounded-full bg-accent text-accent-fg text-[11px] font-bold leading-none mt-0.5">{step.number}</span>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5">
            <span className="text-[13px] font-medium text-fg leading-snug"><InlineMarkdown text={step.title} /></span>
            {isDependent && (
              <span className="inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded-full bg-purple-500/10 text-purple-500 text-[10px] font-medium whitespace-nowrap shrink-0">
                <Link size={9} />{depLabel}
              </span>
            )}
          </div>
          {step.files.length > 0 && (
            <div className="flex flex-wrap gap-1 mt-1">
              {step.files.map((f) => {
                const name = f.split("/").pop() || f;
                return <span key={f} className="inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded bg-accent/8 text-accent text-[10px] font-mono">{name}</span>;
              })}
            </div>
          )}
        </div>
        {(hasDetails || total > 1) && <ChevronDown size={14} className={`shrink-0 mt-0.5 text-fg-faint transition-transform duration-200 ${open ? "rotate-0" : "-rotate-90"}`} />}
      </button>
      {hasDetails && open && (
        <div className="px-3 pb-3 pt-0 animate-[fadeIn_.12s_ease-out]">
          <div className="border-t border-border-soft/50 pt-2 space-y-1.5 text-[12px] text-fg-dim">
            {step.change && (
              <div className="flex items-start gap-1.5"><FileEdit size={12} className="shrink-0 mt-0.5 text-sky-500" /><span><InlineMarkdown text={step.change} /></span></div>
            )}
            {step.dependsOn && (
              <div className="flex items-start gap-1.5"><Link size={12} className="shrink-0 mt-0.5 text-purple-500" /><span><span className="text-fg-faint">{t("plan.dependsOn")}</span> <span className="text-fg">{step.dependsOn}</span></span></div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// ── PlanCard ─────────────────────────────────────────────────────────────────

export function PlanCard({ ask, onAnswer }: { ask: WireAsk; onAnswer: (id: string, answers: QuestionAnswer[]) => void; onDismiss?: () => void }) {
  const t = useT();
  const q = ask.questions[0];
  const plan = q.plan ?? "";
  const parsed = useMemo(() => parsePlan(plan), [plan]);

  const { cardRef, style, onPointerDown } = useDraggableCard();
  const [note, setNote] = useState("");
  const [showNote, setShowNote] = useState(false);
  const [chatOnly, setChatOnly] = useState(false);
  const noteRef = useRef<HTMLInputElement>(null);

  useEffect(() => { if (showNote) noteRef.current?.focus(); }, [showNote]);

  const submit = useCallback((selected: string) => onAnswer(ask.id, [{ questionId: q.id, selected: [selected] }]), [ask.id, q.id, onAnswer]);
  const submitRevise = useCallback(() => {
    const sel = note.trim() ? ["按用户意见修改计划", note.trim()] : ["按用户意见修改计划"];
    onAnswer(ask.id, [{ questionId: q.id, selected: sel }]);
  }, [ask.id, q.id, note, onAnswer]);

  const hasNote = note.trim() !== "";
  const handleSubmit = useCallback(() => {
    if (chatOnly) submit("仅聊天"); else if (hasNote) submitRevise(); else submit("提交执行");
  }, [chatOnly, hasNote, submit, submitRevise]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || (e.target as HTMLElement)?.isContentEditable) return;
      if (e.key === "Enter" && !e.ctrlKey && !e.metaKey) { e.preventDefault(); handleSubmit(); }
      if (e.key === "m" || e.key === "M") { e.preventDefault(); setShowNote((v) => !v); }
      if (e.key === "c" || e.key === "C") { e.preventDefault(); setChatOnly((v) => !v); }
      if (e.key === "Escape") { e.preventDefault(); submit("取消"); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [handleSubmit, submit]);

  const btnLabel = chatOnly ? t("plan.submitChatOnly") : hasNote ? t("plan.submitRevise") : t("plan.submitShort");
  const taskPrompt = q.prompt || "";

  return (
    <div className="fixed inset-0 bg-bg/60 z-50 p-6 animate-[fadeIn_.15s_ease-out] pointer-events-none flex items-start justify-center">
      <div ref={cardRef} className="relative flex flex-col w-full max-w-2xl max-h-[88vh] bg-bg-elev border border-border rounded-xl animate-[scaleIn_.2s_ease-out] pointer-events-auto" style={{ boxShadow: "var(--ds-shadow-panel)", ...style }}>
        <div className="absolute top-0 left-0 right-0 h-7 cursor-grab flex items-start justify-center pt-2.5 select-none group z-10" onPointerDown={onPointerDown} title={t("ask.dragHint")}>
          <GripVertical size={12} className="text-fg-faint/20 group-hover:text-fg-faint/40 transition-colors" />
        </div>

        {/* Header */}
        <div className="flex items-center gap-2 px-5 pt-5 pb-1">
          <span className="w-1 h-4 rounded-full bg-accent shrink-0" />
          <span className="text-fg text-sm font-semibold leading-tight">{t("plan.title")}</span>
          {parsed && <span className="ml-auto text-[11px] text-fg-faint font-mono">{parsed.steps.length} 步 · {parsed.allFiles.length} 文件</span>}
        </div>

        {/* Task prompt */}
        {taskPrompt && (
          <div className="px-5 pb-1 text-fg-dim text-[13px] leading-relaxed line-clamp-2">{taskPrompt}</div>
        )}

        {/* Summary bar */}
        {parsed && <PlanSummaryBar parsed={parsed} />}

        {/* Step list */}
        <div className="px-5 pb-3 flex-1 min-h-0">
          <div className="max-h-[40vh] overflow-y-auto space-y-2 scrollbar-thin">
            {parsed ? parsed.steps.map((step) => <StepCard key={step.number} step={step} steps={parsed.steps} total={parsed.steps.length} />)
              : plan ? (
                <div className="relative bg-bg-soft rounded-lg border border-border-soft p-4 max-h-[30vh] overflow-y-auto">
                  <span className="absolute left-0 top-3 bottom-3 w-[3px] rounded-r-full bg-accent/40" />
                  <Markdown text={plan} />
                </div>
              ) : (
                <div className="bg-bg-soft rounded-lg border border-border-soft p-4 text-center text-fg-faint text-[13px] italic">{t("plan.noContent")}</div>
              )}
          </div>
        </div>

        {/* Modify note */}
        <div className="px-5 pb-1">
          {!showNote ? (
            <button
              className="w-full flex items-center gap-2 px-3 py-2 rounded-lg border border-dashed border-border-soft bg-transparent text-fg-faint text-[12px] cursor-pointer hover:border-accent/30 hover:text-fg-dim hover:bg-bg-soft transition-colors"
              onClick={() => setShowNote(true)}
            >
              <MessageSquare size={13} />
              {t("plan.modifyHint")}
            </button>
          ) : (
            <input
              ref={noteRef}
              className="w-full border border-accent/30 rounded-lg bg-bg text-fg text-xs px-3 py-2 outline-none transition-colors placeholder:text-fg-faint/50"
              placeholder={t("plan.modifyPlaceholder")}
              value={note}
              onChange={(e) => setNote(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); handleSubmit(); }
                if (e.key === "Escape") { setShowNote(false); setNote(""); }
              }}
            />
          )}
        </div>

        {/* Actions footer */}
        <div className="flex items-center gap-3 px-5 pb-4 pt-2 border-t border-border-soft">
          <label className="flex items-center gap-1.5 text-[12px] text-fg-dim cursor-pointer select-none hover:text-fg transition-colors shrink-0">
            <span className={`relative flex items-center justify-center w-[16px] h-[16px] rounded border-2 transition-colors ${chatOnly ? "border-accent bg-accent" : "border-fg-faint hover:border-fg-dim"}`}>
              {chatOnly && <Check size={10} strokeWidth={3} className="text-accent-fg" />}
            </span>
            <input type="checkbox" className="sr-only" checked={chatOnly} onChange={(e) => setChatOnly(e.target.checked)} />
            <span>{t("plan.justChatShort")}</span>
          </label>

          <div className="flex-1" />

          <div className="hidden sm:flex items-center gap-3 text-[10px] text-fg-faint/40">
            <span><kbd className="px-1 py-0.5 rounded bg-bg-soft border border-border-soft text-[10px]">Enter</kbd> 确认</span>
            <span><kbd className="px-1 py-0.5 rounded bg-bg-soft border border-border-soft text-[10px]">M</kbd> 修改</span>
            <span><kbd className="px-1 py-0.5 rounded bg-bg-soft border border-border-soft text-[10px]">C</kbd> 聊天</span>
            <span><kbd className="px-1 py-0.5 rounded bg-bg-soft border border-border-soft text-[10px]">Esc</kbd> 取消</span>
          </div>

          <div className="flex gap-2">
            <button
              className="px-3 py-1.5 border border-border-soft rounded-lg bg-transparent text-fg-dim text-[12px] font-medium cursor-pointer transition-all hover:text-fg hover:border-border hover:bg-bg-soft active:scale-[0.98]"
              onClick={() => submit("取消")}
            >
              {t("plan.cancelShort")}
            </button>
            <button
              className={`px-4 py-1.5 border-0 rounded-lg text-[12px] font-semibold cursor-pointer transition-all active:scale-[0.98] ${
                hasNote ? "bg-purple-500 text-white hover:brightness-110 hover:shadow-md" :
                chatOnly ? "bg-bg-soft border border-border-soft text-fg-dim hover:border-accent/30 hover:text-fg" :
                "bg-accent text-accent-fg hover:brightness-110 hover:shadow-md"
              }`}
              onClick={handleSubmit}
            >
              {btnLabel}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
