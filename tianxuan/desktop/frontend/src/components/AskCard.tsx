import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useT } from "../lib/i18n";
import type { QuestionAnswer, WireAsk, WireAskQuestion } from "../lib/types";

/** 卡片至少保留在屏幕内的边距 (px) */
const DRAG_MARGIN = 40;

export function AskCard({
  ask,
  onAnswer,
  onDismiss,
}: {
  ask: WireAsk;
  onAnswer: (id: string, answers: QuestionAnswer[]) => void;
  onDismiss: () => void;
}) {
  const t = useT();
  const [sel, setSel] = useState<Record<string, string[]>>({});
  const [custom, setCustom] = useState<Record<string, string>>({});

  // ── 拖拽：位置状态 ──────────────────────────────────────────────
  const cardRef = useRef<HTMLDivElement>(null);
  const cardSize = useRef({ w: 0, h: 0 });
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null);
  const dragging = useRef(false);
  const dragStart = useRef({ x: 0, y: 0 });
  const posStart = useRef({ x: 0, y: 0 });

  /** 将坐标约束在可视区域内 */
  const clamp = useCallback((x: number, y: number) => {
    const { w, h } = cardSize.current;
    const M = DRAG_MARGIN;
    return {
      x: Math.min(window.innerWidth - M, Math.max(-w + M, x)),
      y: Math.min(window.innerHeight - M, Math.max(-h + M, y)),
    };
  }, []);

  // 初始居中
  useLayoutEffect(() => {
    const card = cardRef.current;
    if (!card) return;
    const r = card.getBoundingClientRect();
    cardSize.current = { w: r.width, h: r.height };
    setPos(clamp((window.innerWidth - r.width) / 2, (window.innerHeight - r.height) / 2));
  }, [clamp]);

  // 窗口 resize 时重新约束
  useEffect(() => {
    const onResize = () => {
      setPos((p) => {
        if (!p) return p;
        // 重新测量卡片尺寸（max-w-lg 可能触发宽度变化）
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
      if (e.button !== 0) return; // 仅左键
      e.preventDefault();
      dragging.current = true;
      dragStart.current = { x: e.clientX, y: e.clientY };
      setPos((p) => {
        posStart.current = p ?? { x: 0, y: 0 };
        return p; // 保持当前值，不触发额外渲染
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

  // ── 问题交互（不变）────────────────────────────────────────────
  const toggle = (q: WireAskQuestion, label: string) => {
    setCustom((c) => ({ ...c, [q.id]: "" }));
    setSel((s) => {
      const cur = s[q.id] ?? [];
      if (q.multi) {
        return { ...s, [q.id]: cur.includes(label) ? cur.filter((x) => x !== label) : [...cur, label] };
      }
      return { ...s, [q.id]: [label] };
    });
  };

  const setTyped = (q: WireAskQuestion, text: string) => {
    setCustom((c) => ({ ...c, [q.id]: text }));
    if (text.trim()) setSel((s) => ({ ...s, [q.id]: [] }));
  };

  const answered = (q: WireAskQuestion) =>
    (sel[q.id]?.length ?? 0) > 0 || (custom[q.id]?.trim() ?? "") !== "";
  const allAnswered = ask.questions.every(answered);

  const submit = () => {
    onAnswer(
      ask.id,
      ask.questions.map((q) => ({
        questionId: q.id,
        selected: custom[q.id]?.trim() ? [custom[q.id].trim()] : (sel[q.id] ?? []),
      })),
    );
  };

  return (
    <div className="fixed inset-0 bg-bg/60 z-50 p-6 animate-[fadeIn_.15s_ease-out] pointer-events-none">
      <div
        ref={cardRef}
        className="relative flex flex-col gap-4 w-full max-w-lg max-h-[85vh] overflow-y-auto bg-bg-elev border border-border rounded-xl p-5 pt-7 animate-[scaleIn_.2s_ease-out] pointer-events-auto"
        style={{
          boxShadow: "var(--ds-shadow-panel)",
          ...(pos
            ? { position: "absolute", left: pos.x, top: pos.y }
            : { visibility: "hidden" })
        }}
      >
        {/* 拖拽手柄 */}
        <div
          className="absolute top-0 left-0 right-0 h-7 cursor-grab flex items-start justify-center pt-2 select-none"
          onPointerDown={startDrag}
          title="拖拽移动"
        >
          <span className="w-8 h-1 rounded-full bg-fg-faint/25" />
        </div>

        {ask.questions.map((q) => (
          <div className="flex flex-col gap-3" key={q.id}>
            {q.header && (
              <div className="flex items-center gap-2">
                <span className="w-1 h-4 rounded-full bg-accent shrink-0" />
                <span className="text-fg text-[15px] font-semibold leading-tight">{q.header}</span>
              </div>
            )}
            <div className="text-fg-dim text-[13px] leading-relaxed">{q.prompt}</div>
            <div className="flex flex-col gap-1.5">
              {q.options.map((o) => {
                const on = (sel[q.id] ?? []).includes(o.label);
                return (
                  <button
                    key={o.label}
                    className={`flex items-start gap-2.5 w-full px-3 py-2.5 rounded-lg border text-left transition-all duration-150 ${
                      on
                        ? "border-accent bg-accent-soft shadow-[0_0_0_1px_var(--accent)]"
                        : "border-border-soft bg-transparent hover:border-border hover:bg-bg-soft active:scale-[0.99]"
                    }`}
                    onClick={() => toggle(q, o.label)}
                  >
                    <span
                      className={`shrink-0 w-[18px] h-[18px] mt-px rounded-full border-2 flex items-center justify-center transition-colors duration-150 ${
                        on
                          ? "border-accent bg-accent"
                          : "border-fg-faint"
                      }`}
                    >
                      {on && (
                        <span className="w-1.5 h-1.5 rounded-full bg-accent-fg" />
                      )}
                    </span>
                    <span className="flex flex-col gap-0.5 min-w-0">
                      <span className={`text-[13px] leading-snug ${on ? "text-fg font-medium" : "text-fg-dim"}`}>
                        {o.label}
                      </span>
                      {o.description && (
                        <span className="text-fg-faint text-[11px] leading-snug">{o.description}</span>
                      )}
                    </span>
                  </button>
                );
              })}
            </div>
            <input
              className="w-full border border-border-soft rounded-lg bg-bg text-fg text-[12.5px] px-3 py-2 outline-none placeholder:text-fg-faint/40 transition-colors duration-150 focus:border-accent focus:shadow-[0_0_0_2px_var(--accent-soft)]"
              placeholder={t("ask.customPlaceholder")}
              value={custom[q.id] ?? ""}
              onChange={(e) => setTyped(q, e.target.value)}
            />
          </div>
        ))}
        <div className="flex justify-end gap-2 pt-1 border-t border-border-soft">
          <button
            className="px-4 py-2 border border-border-soft rounded-lg bg-transparent text-fg-dim text-[12.5px] cursor-pointer transition-all duration-[var(--dur-fast)] hover:text-fg hover:border-border hover:bg-bg-soft hover:-translate-y-px active:scale-[0.98]"
            onClick={onDismiss}
          >
            {t("ask.justChat")}
          </button>
          <button
            className="px-4 py-2 border-0 rounded-lg bg-accent text-accent-fg text-[12.5px] font-semibold cursor-pointer transition-all duration-[var(--dur-fast)] enabled:hover:brightness-110 enabled:hover:-translate-y-px enabled:active:scale-[0.98] disabled:opacity-40 disabled:cursor-default"
            onClick={submit}
            disabled={!allAnswered}
          >
            {t("common.submit")}
          </button>
        </div>
      </div>
    </div>
  );
}
