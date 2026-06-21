import { useRef, useState } from "react";
import { Check, ChevronDown, ChevronRight, Circle, Loader, X } from "lucide-react";
import { useT } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";
import type { Todo } from "../lib/tools";

const statusIcon = (status: string) => {
  switch (status) {
    case "completed":
      return <Check size={13} className="text-ok shrink-0" />;
    case "in_progress":
      return <Loader size={13} className="text-accent shrink-0 animate-spin" />;
    default:
      return <Circle size={13} className="text-fg-faint shrink-0" />;
  }
};

export function TodoPanel({ todos, onDismiss }: { todos: Todo[]; onDismiss: () => void }) {
  const t = useT();
  const compact = useCompact();
  const [open, setOpen] = useState(true);
  const listRef = useRef<HTMLUListElement>(null);
  useGSAPCollapse(listRef, open);
  if (todos.length === 0) return null;

  const done = todos.filter((t) => t.status === "completed").length;
  const current = todos.find((t) => t.status === "in_progress");
  const pct = todos.length > 0 ? Math.round((done / todos.length) * 100) : 0;

  const itemPy = compact ? "py-[5px]" : "py-[7px]";
  const itemPx = compact ? "px-[7px] pl-[9px]" : "px-[7px] pl-[11px]";
  const itemTextSize = compact ? "text-[11.5px]" : "text-[12.5px]";

  return (
    <div className="max-w-[--maxw] mx-auto mb-2 border border-border rounded-[9px] bg-bg-soft overflow-hidden" style={{boxShadow: "var(--ds-shadow-card)"}}>
      {/* Thin progress bar */}
      <div className="h-[3px] bg-border-soft">
        <div
          className={`h-full transition-[width] duration-500 ease-out ${pct >= 100 ? "bg-ok" : "bg-accent"}`}
          style={{ width: `${pct}%` }}
        />
      </div>

      {/* Header */}
      <div className={`flex items-center ${itemPx}`}>
        <button
          className={`flex items-center gap-[7px] flex-1 min-w-0 ${itemPy} bg-transparent border-0 text-fg-dim ${itemTextSize} cursor-pointer no-drag`}
          onClick={() => setOpen((v) => !v)}
        >
          {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
          <span className="shrink-0 font-medium text-fg">{t("todo.title")}</span>
          <span className="shrink-0 text-fg-faint font-mono text-[11px] tabular-nums">
            {done}/{todos.length}
          </span>
          {!open && current && (
            <span className={`text-fg-faint truncate ${compact ? "text-[10px]" : "text-[11px]"}`}>
              {current.activeForm || current.content}
            </span>
          )}
        </button>
        <button
          className="ml-auto border-0 bg-transparent text-fg-faint cursor-pointer p-1.5 rounded hover:text-err hover:bg-bg-soft no-drag"
          onClick={onDismiss}
          title={t("todo.dismiss")}
        >
          <X size={13} />
        </button>
      </div>

      {/* List */}
      <ul ref={listRef} className="m-0 p-0 list-none border-t border-border-soft" style={{ overflow: "hidden" }}>
        {todos.map((t, i) => {
          const isPhase = t.level === 0;
          const isSub = t.level != null && t.level > 0;
          return (
            <li
              key={i}
              className={`relative flex items-center gap-2.5 ${itemPx} ${itemPy} border-b border-border-soft last:border-b-0 transition-colors duration-200 ${
                t.status === "in_progress"
                  ? "bg-accent-soft"
                  : "bg-transparent hover:bg-bg-elev"
              } ${isSub ? (compact ? "pl-8" : "pl-9") : ""}`}
            >
              {/* Left accent strip for in-progress items */}
              {t.status === "in_progress" && !isSub && (
                <div className="absolute left-0 top-0 bottom-0 w-[3px] bg-accent rounded-r-sm" />
              )}
              {/* Sub-item border trail */}
              {isSub && (
                <div className="absolute left-[11px] top-0 bottom-0 w-[2px] bg-border-soft" />
              )}

              {statusIcon(t.status)}

              <span
                className={`min-w-0 leading-relaxed ${
                  isPhase ? "font-medium text-fg" : "text-fg-dim"
                } ${
                  t.status === "completed"
                    ? "line-through text-fg-faint"
                    : t.status === "in_progress"
                      ? "text-fg font-medium"
                      : ""
                } ${itemTextSize}`}
              >
                {t.status === "in_progress" && t.activeForm ? t.activeForm : t.content}
              </span>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
