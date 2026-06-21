import { useState } from "react";
import { useT } from "../lib/i18n";
import type { QuestionAnswer, WireAsk, WireAskQuestion } from "../lib/types";

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
    <div className="fixed inset-0 flex items-center justify-center bg-bg/60 z-50 p-6 animate-[fadeIn_.15s_ease-out] pointer-events-none">
      <div className="flex flex-col gap-4 w-full max-w-lg max-h-[85vh] overflow-y-auto bg-bg-elev border border-border rounded-xl shadow-[0_16px_48px_rgba(0,0,0,0.35)] p-5 animate-[scaleIn_.2s_ease-out] pointer-events-auto">
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
            className="px-4 py-2 border border-border-soft rounded-lg bg-transparent text-fg-dim text-[12.5px] cursor-pointer transition-all duration-[0.12s] hover:text-fg hover:border-border hover:bg-bg-soft hover:-translate-y-px active:scale-[0.98]"
            onClick={onDismiss}
          >
            {t("ask.justChat")}
          </button>
          <button
            className="px-4 py-2 border-0 rounded-lg bg-accent text-accent-fg text-[12.5px] font-semibold cursor-pointer transition-all duration-[0.12s] enabled:hover:brightness-110 enabled:hover:-translate-y-px enabled:active:scale-[0.98] disabled:opacity-40 disabled:cursor-default"
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
