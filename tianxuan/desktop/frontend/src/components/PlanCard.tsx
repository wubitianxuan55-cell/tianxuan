import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useT } from "../lib/i18n";
import { Markdown } from "./Markdown";
import type { QuestionAnswer, WireAsk } from "../lib/types";

/** 卡片至少保留在屏幕内的边距 (px) */
const DRAG_MARGIN = 40;

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
  const [note, setNote] = useState("");

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
      if (e.key === "1") { e.preventDefault(); submit("提交执行"); }
      if (e.key === "2") { e.preventDefault(); submit("取消"); }
      if (e.key === "Enter" && !e.shiftKey && note.trim()) {
        e.preventDefault();
        submit(note.trim());
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const submit = (selected: string) => {
    onAnswer(ask.id, [{ questionId: q.id, selected: [selected] }]);
  };

  const hasNote = note.trim() !== "";
  const submitLabel = hasNote ? "提交并附修改意见" : "提交执行";

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
          <span className="text-fg text-[15px] font-semibold leading-tight">计划确认</span>
        </div>

        {/* 任务描述 */}
        {q.prompt && (
          <div className="px-5 pb-2 text-fg-dim text-[13px] leading-relaxed">{q.prompt}</div>
        )}

        {/* 计划内容区 — Markdown 渲染，可滚动 */}
        <div className="px-5 pb-3 flex-1 min-h-0">
          <div className="max-h-[50vh] overflow-y-auto bg-bg-soft rounded-lg border border-border-soft p-4">
            {plan ? (
              <Markdown text={plan} />
            ) : (
              <span className="text-fg-faint text-[13px] italic">（无计划内容）</span>
            )}
          </div>
        </div>

        {/* 修改意见 */}
        <div className="px-5 pb-2">
          <input
            className="w-full border border-border-soft rounded-lg bg-bg text-fg text-[12.5px] px-3 py-2 outline-none placeholder:text-fg-faint/40 transition-colors duration-150 focus:border-accent focus:shadow-[0_0_0_2px_var(--accent-soft)]"
            placeholder="输入修改意见后提交…"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey && note.trim()) {
                e.preventDefault();
                submit(note.trim());
              }
            }}
          />
        </div>

        {/* 底部按钮 */}
        <div className="flex justify-end gap-2 px-5 pb-4 pt-2 border-t border-border-soft">
          <button
            className="px-4 py-2 border border-border-soft rounded-lg bg-transparent text-fg-dim text-[12.5px] cursor-pointer transition-all duration-[var(--dur-fast)] hover:text-fg hover:border-border hover:bg-bg-soft hover:-translate-y-px active:scale-[0.98]"
            onClick={() => submit("取消")}
          >
            取消
          </button>
          <button
            className="px-4 py-2 border-0 rounded-lg bg-accent text-accent-fg text-[12.5px] font-semibold cursor-pointer transition-all duration-[var(--dur-fast)] hover:brightness-110 hover:-translate-y-px active:scale-[0.98]"
            onClick={() => submit(hasNote ? note.trim() : "提交执行")}
          >
            {submitLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
