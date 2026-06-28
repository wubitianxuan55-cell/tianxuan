import { useRef, useState } from "react";
import { Check, Circle, Loader } from "lucide-react";
import { useT } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";
import type { Todo } from "../lib/tools";
import { PromptBadge, PromptHeaderAction, PromptShelf } from "./PromptShelf";

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
  const currentRef = useRef<HTMLLIElement | null>(null);
  useGSAPCollapse(listRef, open);
  if (todos.length === 0) return null;

  const done = todos.filter((td) => td.status === "completed").length;
  const current = todos.find((td) => td.status === "in_progress");
  const allDone = todos.length > 0 && done === todos.length;
  const summary = current?.activeForm || current?.content || todos[todos.length - 1]?.content || "";
  const pct = todos.length > 0 ? Math.round((done / todos.length) * 100) : 0;

  const itemPy = compact ? "py-[5px]" : "py-[7px]";
  const itemPx = compact ? "px-[7px] pl-[9px]" : "px-[7px] pl-[11px]";
  const itemTextSize = compact ? "text-[11.5px]" : "text-[12.5px]";

  return (
    <PromptShelf
      titleId="todo-shelf-title"
      title={t("todo.title")}
      badges={<PromptBadge>{done}/{todos.length}</PromptBadge>}
      meta={!open ? summary : undefined}
      role="region"
      headerActions={
        <>
          <PromptHeaderAction onClick={() => setOpen((v) => !v)}>
            {open ? t("common.collapse") : t("common.expand")}
          </PromptHeaderAction>
          {allDone && (
            <PromptHeaderAction onClick={onDismiss}>
              {t("common.close")}
            </PromptHeaderAction>
          )}
        </>
      }
    >
      {/* Thin progress bar */}
      <div className="h-[3px] bg-border-soft">
        <div
          className={`h-full transition-[width] duration-500 ease-out ${pct >= 100 ? "bg-ok" : "bg-accent"}`}
          style={{ width: `${pct}%` }}
        />
      </div>

      {/* List */}
      <ul ref={listRef} className="m-0 p-0 list-none" style={{ overflow: "hidden" }}>
        {todos.map((td, i) => {
          const isPhase = td.level === 0;
          const isSub = td.level != null && td.level > 0;
          return (
            <li
              key={i}
              ref={td.status === "in_progress" ? currentRef : undefined}
              className={`relative flex items-center gap-2.5 ${itemPx} ${itemPy} border-b border-border-soft last:border-b-0 transition-colors duration-200 ${
                td.status === "in_progress"
                  ? "bg-accent-soft"
                  : "bg-transparent hover:bg-bg-elev"
              } ${isSub ? (compact ? "pl-8" : "pl-9") : ""}`}
            >
              {/* Left accent strip */}
              {td.status === "in_progress" && !isSub && (
                <div className="absolute left-0 top-0 bottom-0 w-[3px] bg-accent rounded-r-sm" />
              )}
              {/* Sub-item trail */}
              {isSub && (
                <div className="absolute left-[11px] top-0 bottom-0 w-[2px] bg-border-soft" />
              )}

              {statusIcon(td.status)}

              <span
                className={`min-w-0 leading-relaxed ${
                  isPhase ? "font-medium text-fg" : "text-fg-dim"
                } ${
                  td.status === "completed"
                    ? "line-through text-fg-faint"
                    : td.status === "in_progress"
                      ? "text-fg font-medium"
                      : ""
                } ${itemTextSize}`}
              >
                {td.status === "in_progress" && td.activeForm ? td.activeForm : td.content}
              </span>
            </li>
          );
        })}
      </ul>
    </PromptShelf>
  );
}
