import { useEffect, useRef, useState } from "react";
import { Check, Circle, Loader, X } from "lucide-react";
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
  const summary = current?.activeForm || current?.content || todos[todos.length - 1]?.content || "";
  const pct = todos.length > 0 ? Math.round((done / todos.length) * 100) : 0;

  // 自动滚动到进行中任务
  useEffect(() => {
    if (open && currentRef.current) {
      currentRef.current.scrollIntoView({ block: "nearest", behavior: "smooth" });
    }
  }, [open]);

  const itemPy = compact ? "py-1" : "py-2";
  const itemPx = compact ? "px-2 pl-2" : "px-2 pl-3";
  const itemTextSize = compact ? "text-[11px]" : "text-xs";

  return (
    <PromptShelf
      titleId="todo-shelf-title"
      title={t("todo.title")}
      badges={
        <PromptBadge>
          {done}/{todos.length}
          {pct > 0 && pct < 100 && ` · ${pct}%`}
        </PromptBadge>
      }
      meta={!open ? summary : undefined}
      role="region"
      headerActions={
        <>
          <PromptHeaderAction onClick={() => setOpen((v) => !v)}>
            {open ? t("common.collapse") : t("common.expand")}
          </PromptHeaderAction>
          <PromptHeaderAction onClick={onDismiss}>
            <X size={11} />
          </PromptHeaderAction>
        </>
      }
    >
      {/* 进度条 */}
      <div className="h-[5px] bg-border-soft relative">
        <div
          className={`h-full transition-[width] duration-500 ease-in-out rounded-r-sm ${
            pct >= 100
              ? "bg-ok"
              : "bg-gradient-to-r from-accent via-accent to-ok/70"
          }`}
          style={{ width: `${pct}%` }}
        />
        {pct >= 100 && (
          <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
            <span className="text-[9px] font-bold text-ok tracking-wider">✓ 全部完成</span>
          </div>
        )}
      </div>

      {/* 任务列表 */}
      <ul ref={listRef} className="m-0 p-0 list-none" style={{ overflow: "hidden" }}>
        {todos.map((td, i) => {
          const isPhase = td.level === 0;
          const isSub = td.level != null && td.level > 0;
          const isCurrent = td.status === "in_progress";
          return (
            <li
              key={i}
              ref={isCurrent ? currentRef : undefined}
              className={`relative flex items-center gap-2.5 ${itemPx} ${itemPy} border-b border-border-soft last:border-b-0 transition-colors duration-200 ${
                isCurrent
                  ? "bg-accent-soft/70"
                  : "bg-transparent hover:bg-bg-elev"
              } ${isSub ? (compact ? "pl-8" : "pl-9") : ""}`}
            >
              {/* 左强调条 — 进行中 */}
              {isCurrent && !isSub && (
                <div className="absolute left-0 top-0 bottom-0 w-[3px] bg-accent rounded-r-sm animate-[breathe_2s_ease-in-out_infinite]" />
              )}
              {/* 子任务连接线 */}
              {isSub && (
                <div className="absolute left-3 top-0 bottom-0 w-[2px] bg-border-soft" />
              )}

              {statusIcon(td.status)}

              <span
                className={`min-w-0 leading-relaxed ${
                  isPhase ? "font-medium text-fg" : "text-fg-dim"
                } ${
                  td.status === "completed"
                    ? "line-through text-fg-faint/60"
                    : isCurrent
                      ? "text-fg font-semibold"
                      : ""
                } ${itemTextSize}`}
              >
                {isCurrent && td.activeForm ? td.activeForm : td.content}
              </span>
            </li>
          );
        })}
      </ul>
    </PromptShelf>
  );
}
