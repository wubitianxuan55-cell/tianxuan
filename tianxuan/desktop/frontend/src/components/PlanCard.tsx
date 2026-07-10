import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Check, ChevronDown, FileCode, ListChecks, RotateCcw, FileEdit, Link } from "lucide-react";
import { useT } from "../lib/i18n";
import { Markdown } from "./Markdown";
import { parsePlan, type ParsedPlan } from "../lib/planParser";
import type { QuestionAnswer, WireAsk } from "../lib/types";

/** 卡片至少保留在屏幕内的边距 (px) */
const DRAG_MARGIN = 40;

function PlanSummaryBar({ parsed }: { parsed: ParsedPlan }) {
  const t = useT();
  return (
    <div className="flex items-center gap-2 px-5 py-2">
      <div className="flex items-center gap-1.5 text-[12px] text-fg-dim bg-bg-soft rounded-lg px-2.5 py-1">
        <ListChecks size={13} strokeWidth={1.5} />
        <span>{t(parsed.steps.length === 1 ? "plan.stepCount_one" : "plan.stepCount_other", { n: parsed.steps.length })}</span>
      </div>
      <div className="flex items-center gap-1.5 text-[12px] text-fg-dim bg-bg-soft rounded-lg px-2.5 py-1">
        <FileCode size={13} strokeWidth={1.5} />
        <span>{t(parsed.allFiles.length === 1 ? "plan.fileCount_one" : "plan.fileCount_other", { n: parsed.allFiles.length })}</span>
      </div>
      {parsed.allFiles.length > 0 && (
        <div className="flex-1 flex items-center gap-1.5 overflow-x-auto no-scrollbar justify-end">
          {parsed.allFiles.map((f) => {
            const name = f.split("/").pop() || f;
            return (
              <span
                key={f}
                className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-accent/10 text-accent text-[11px] font-medium whitespace-nowrap"
              >
                {name}
              </span>
            );
          })}
        </div>
      )}
    </div>
  );
}

function StepCard({
  step,
}: {
  step: ParsedPlan["steps"][0];
}) {
  const [open, setOpen] = useState(false);
  const t = useT();
  const hasDetails = step.change || step.dependsOn || step.success || step.riskRecovery;

  return (
    <div className="rounded-lg border border-border-soft bg-bg-soft overflow-hidden">
      {/* 步骤标题行 — 点击切换折叠 */}
      <button
        className={`w-full flex items-start gap-2.5 px-3 py-2.5 text-left cursor-pointer transition-colors duration-150 hover:bg-bg/40 active:bg-bg/60 ${
          step.dependsOn && step.dependsOn !== "无" ? "border-l-2 border-accent/30" : ""
        }`}
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        {/* 步骤编号圆形标记 */}
        <span className="shrink-0 flex items-center justify-center w-5 h-5 rounded-full bg-accent text-accent-fg text-[11px] font-bold leading-none mt-0.5">
          {step.number}
        </span>

        <div className="flex-1 min-w-0">
          {/* 标题 */}
          <div className="text-[13px] font-medium text-fg leading-snug">
            {step.title}
          </div>

          {/* 文件标签行 */}
          {step.files.length > 0 && (
            <div className="flex flex-wrap gap-1 mt-1">
              {step.files.map((f) => {
                const name = f.split("/").pop() || f;
                return (
                  <span
                    key={f}
                    className="inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded bg-accent/8 text-accent text-[10px] font-mono"
                  >
                    {name}
                  </span>
                );
              })}
            </div>
          )}
        </div>

        {/* 展开图标 */}
        {hasDetails && (
          <ChevronDown
            size={14}
            strokeWidth={1.5}
            className={`shrink-0 mt-0.5 text-fg-faint transition-transform duration-200 ${
              open ? "rotate-0" : "-rotate-90"
            }`}
          />
        )}
      </button>

      {/* 折叠详情区 */}
      {hasDetails && open && (
        <div className="px-3 pb-3 pt-0 animate-[fadeIn_.12s_ease-out]">
          <div className="border-t border-border-soft/50 pt-2 space-y-1.5">
            {step.change && (
              <div className="flex items-start gap-1.5 text-[12px] text-fg-dim">
                <FileEdit size={12} strokeWidth={1.5} className="shrink-0 mt-0.5 text-sky-500" />
                <span>
                  <span className="text-fg-muted">{t("plan.change")}</span>
                  <span className="text-fg">{step.change}</span>
                </span>
              </div>
            )}
            {step.dependsOn && (
              <div className="flex items-start gap-1.5 text-[12px] text-fg-dim">
                <Link size={12} strokeWidth={1.5} className="shrink-0 mt-0.5 text-purple-500" />
                <span>
                  <span className="text-fg-muted">{t("plan.dependsOn")}</span>
                  {step.dependsOn === "无" ? (
                    <span className="text-fg-faint italic">{t("plan.dependsNone")}</span>
                  ) : (
                    <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-full bg-accent/10 text-accent text-[11px] font-medium">
                      {step.dependsOn}
                    </span>
                  )}
                </span>
              </div>
            )}
            {step.success && (
              <div className="flex items-start gap-1.5 text-[12px] text-fg-dim">
                <span className="shrink-0 mt-0.5 text-green-500">✓</span>
                <span>
                  <span className="text-fg-muted">{t("plan.success")}</span>
                  <code className="text-[11px] bg-bg px-1 py-0.5 rounded">{step.success}</code>
                </span>
              </div>
            )}
            {step.riskRecovery && (
              <div className="flex items-start gap-1.5 text-[12px] text-fg-dim">
                <RotateCcw size={12} strokeWidth={1.5} className="shrink-0 mt-0.5 text-amber-500" />
                <span>
                  <span className="text-fg-muted">{t("plan.riskRecovery")}</span>
                  <span className="text-fg">{step.riskRecovery}</span>
                </span>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

export function PlanCard({
  ask,
  onAnswer,
}: {
  ask: WireAsk;
  onAnswer: (id: string, answers: QuestionAnswer[]) => void;
  onDismiss: () => void;
}) {
  const t = useT();
  const q = ask.questions[0];
  const plan = q.plan ?? "";
  const parsed = parsePlan(plan);

  const [note, setNote] = useState("");
  const [chatOnly, setChatOnly] = useState(false);

  // ── 拖拽：位置状态 ──────────────────────────────────────────────
  const cardRef = useRef<HTMLDivElement>(null);
  const cardSize = useRef({ w: 0, h: 0 });
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null);
  const dragging = useRef(false);
  const dragStart = useRef({ x: 0, y: 0 });
  const posStart = useRef({ x: 0, y: 0 });

  const clamp = useCallback((x: number, y: number) => {
    const { w, h } = cardSize.current;
    const M = DRAG_MARGIN;
    return {
      x: Math.min(window.innerWidth - M, Math.max(-w + M, x)),
      y: Math.min(window.innerHeight - M, Math.max(-h + M, y)),
    };
  }, []);

  useLayoutEffect(() => {
    const card = cardRef.current;
    if (!card) return;
    const r = card.getBoundingClientRect();
    cardSize.current = { w: r.width, h: r.height };
    setPos(clamp((window.innerWidth - r.width) / 2, (window.innerHeight - r.height) / 2));
  }, [clamp]);

  useEffect(() => {
    const onResize = () => {
      setPos((p) => {
        if (!p) return p;
        const r = cardRef.current?.getBoundingClientRect();
        if (r) cardSize.current = { w: r.width, h: r.height };
        return clamp(p.x, p.y);
      });
    };
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, [clamp]);

  // ── 拖拽事件 ──────────────────────────────────────────────────
  const startDrag = useCallback(
    (e: React.PointerEvent) => {
      if (e.button !== 0) return;
      e.preventDefault();
      dragging.current = true;
      dragStart.current = { x: e.clientX, y: e.clientY };
      setPos((p) => {
        posStart.current = p ?? { x: 0, y: 0 };
        return p;
      });

      const onMove = (me: PointerEvent) => {
        if (!dragging.current) return;
        setPos(
          clamp(
            posStart.current.x + (me.clientX - dragStart.current.x),
            posStart.current.y + (me.clientY - dragStart.current.y),
          ),
        );
      };
      const onUp = () => {
        dragging.current = false;
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
        window.removeEventListener("pointermove", onMove);
        window.removeEventListener("pointerup", onUp);
        window.removeEventListener("pointercancel", onUp);
      };
      document.body.style.cursor = "grabbing";
      document.body.style.userSelect = "none";
      window.addEventListener("pointermove", onMove);
      window.addEventListener("pointerup", onUp);
      window.addEventListener("pointercancel", onUp);
    },
    [clamp],
  );

  // ── 键盘：数字键 + Enter ──────────────────────────────────────
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || (e.target as HTMLElement)?.isContentEditable) return;
      if (e.key === "1") { e.preventDefault(); handleSubmit(); }
      if (e.key === "2") { e.preventDefault(); submit("取消"); }
      if (e.key === "3") { e.preventDefault(); setChatOnly(v => !v); }
      if (e.key === "Enter" && !e.shiftKey && note.trim()) {
        e.preventDefault();
        handleSubmit();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [note, chatOnly]);

  const submit = (selected: string) => {
    onAnswer(ask.id, [{ questionId: q.id, selected: [selected] }]);
  };
  const submitRevise = () => {
    const sel = note.trim() ? ["按用户意见修改计划", note.trim()] : ["按用户意见修改计划"];
    onAnswer(ask.id, [{ questionId: q.id, selected: sel }]);
  };

  const hasNote = note.trim() !== "";
  const handleSubmit = () => {
    if (chatOnly) {
      submit("仅聊天");
    } else if (hasNote) {
      submitRevise();
    } else {
      submit("提交执行");
    }
  };
  const btnLabel = chatOnly ? t("plan.submitChatOnly") : hasNote ? t("plan.submitRevise") : t("plan.submit");

  return (
    <div className="fixed inset-0 bg-bg/60 z-50 p-6 animate-[fadeIn_.15s_ease-out] pointer-events-none">
      <div
        ref={cardRef}
        className="relative flex flex-col gap-0 w-full max-w-xl max-h-[88vh] bg-bg-elev border border-border rounded-xl animate-[scaleIn_.2s_ease-out] pointer-events-auto"
        style={{
          boxShadow: "var(--ds-shadow-panel)",
          ...(pos
            ? { position: "absolute", left: pos.x, top: pos.y }
            : { visibility: "hidden" })
        }}
      >
        {/* 拖拽手柄 */}
        <div
          className="absolute top-0 left-0 right-0 h-7 cursor-grab flex items-start justify-center pt-2 select-none group z-10"
          onPointerDown={startDrag}
          title={t("ask.dragHint")}
        >
          <span className="w-8 h-1 rounded-full bg-fg-faint/25 group-hover:bg-fg-faint/50 group-hover:w-10 transition-all duration-200" />
        </div>

        {/* 标题 */}
        <div className="flex items-center gap-2 px-5 pt-5 pb-1">
          <span className="w-1 h-4 rounded-full bg-accent shrink-0" />
          <span className="text-fg text-sm font-semibold leading-tight">{t("plan.title")}</span>
        </div>

        {/* 任务描述 */}
        {q.prompt && (
          <div className="px-5 pb-1 text-fg-dim text-[13px] leading-relaxed line-clamp-2">{q.prompt}</div>
        )}

        {/* 摘要栏 */}
        {parsed && <PlanSummaryBar parsed={parsed} />}

        {/* 计划内容区 — 结构化卡片或降级 Markdown */}
        <div className="px-5 pb-3 flex-1 min-h-0">
          <div className="max-h-[45vh] overflow-y-auto space-y-2">
            {parsed ? (
              parsed.steps.map((step) => (
                <StepCard key={step.number} step={step} />
              ))
            ) : plan ? (
              <div className="relative bg-bg-soft rounded-lg border border-border-soft p-4">
                <span className="absolute left-0 top-3 bottom-3 w-[3px] rounded-r-full bg-accent/40" />
                <Markdown text={plan} />
              </div>
            ) : (
              <div className="bg-bg-soft rounded-lg border border-border-soft p-4 text-center text-fg-faint text-[13px] italic">
                {t("plan.noContent")}
              </div>
            )}
          </div>
        </div>

        {/* 修改意见 */}
        <div className="px-5 pb-1">
          <input
            className="w-full border border-border-soft rounded-lg bg-bg text-fg text-xs px-3 py-2 outline-none placeholder:text-fg-faint/40 transition-all duration-150 focus:border-accent focus:shadow-[0_0_0_2px_var(--accent-soft)]"
            placeholder={t("plan.modifyPlaceholder")}
            value={note}
            onChange={(e) => setNote(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                handleSubmit();
              }
            }}
          />
        </div>

        {/* 兜底 checkbox */}
        <div className="px-5 pb-2">
          <label className="flex items-center gap-2 text-[12px] text-fg-dim cursor-pointer select-none hover:text-fg transition-colors duration-150">
            <span className={`relative flex items-center justify-center w-[18px] h-[18px] rounded border-2 transition-colors duration-150 ${
              chatOnly ? "border-accent bg-accent" : "border-fg-faint hover:border-fg-dim"
            }`}>
              {chatOnly && <Check size={11} strokeWidth={3} className="text-accent-fg" />}
            </span>
            <input
              type="checkbox"
              className="sr-only"
              checked={chatOnly}
              onChange={(e) => setChatOnly(e.target.checked)}
            />
            <span>{t("plan.justChat")}</span>
          </label>
        </div>

        {/* 底部按钮：取消 + 提交 */}
        <div className="flex justify-end gap-2 px-5 pb-4 pt-2 border-t border-border-soft">
          <button
            className="px-4 py-2 border border-border-soft rounded-lg bg-transparent text-fg-dim text-xs font-medium cursor-pointer transition-all duration-150 focus-visible:ring-2 focus-visible:ring-accent/40 focus-visible:outline-none hover:text-fg hover:border-border hover:bg-bg-soft hover:shadow-sm active:scale-[0.98]"
            onClick={() => submit("取消")}
          >
            {t("plan.cancel")}
          </button>
          <button
            className="px-4 py-2 border-0 rounded-lg bg-accent text-accent-fg text-xs font-semibold cursor-pointer transition-all duration-150 focus-visible:ring-2 focus-visible:ring-accent/40 focus-visible:outline-none hover:brightness-110 hover:shadow-md active:scale-[0.98]"
            onClick={handleSubmit}
          >
            {btnLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
