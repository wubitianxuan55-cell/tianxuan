import { CheckCircle2, Circle, FileText, ListTodo, Loader2 } from "lucide-react";
import { useT } from "../lib/i18n";
import { DrawerHeader, DrawerTitle } from "./DrawerHeader";
import { MemoMarkdown } from "./MemoMarkdown";
import { ResizableDrawer } from "./ResizableDrawer";
import type { Todo } from "../lib/tools";

function ProgressBar({ done, total }: { done: number; total: number }) {
  const pct = total > 0 ? Math.round((done / total) * 100) : 0;
  return (
    <div className="h-1.5 bg-bg-soft rounded-full overflow-hidden">
      <div
        className={`h-full rounded-full transition-all duration-700 ease-out ${
          pct === 100 ? "bg-ok" : ""
        }`}
        style={
          pct === 0
            ? { width: 0 }
            : pct === 100
            ? { width: "100%" }
            : { width: `${pct}%`, background: "linear-gradient(90deg, var(--accent), var(--ok))" }
        }
      />
    </div>
  );
}

function TodoIcon({ status }: { status: string }) {
  switch (status) {
    case "completed":
      return <CheckCircle2 size={13} className="text-ok shrink-0" />;
    case "in_progress":
      return <Loader2 size={13} className="text-accent shrink-0 animate-spin" />;
    default:
      return <Circle size={13} className="text-fg-faint/40 shrink-0" />;
  }
}

function TodoItem({ todo, isPhase }: { todo: Todo; isPhase: boolean }) {
  const done = todo.status === "completed";
  const active = todo.status === "in_progress";
  const label = active && todo.activeForm ? todo.activeForm : todo.content;
  return (
    <div
      className={`flex items-start gap-2 py-1 ${
        isPhase ? "pl-0" : "pl-5"
      } ${active ? "bg-accent-soft/30 -mx-1 px-1 rounded" : ""}`}
    >
      <TodoIcon status={todo.status} />
      <span
        className={`text-[12.5px] leading-[1.4] min-w-0 flex-1 ${
          done ? "text-fg-faint line-through" : active ? "text-fg font-medium" : isPhase ? "text-fg font-semibold" : "text-fg-dim"
        }`}
      >
        {label}
      </span>
    </div>
  );
}

function PhaseBlock({ todos, startIdx }: { todos: Todo[]; startIdx: number }) {
  // Take the phase (level 0) and all following sub-steps until next phase
  const phase = todos[startIdx];
  const items: Todo[] = [phase];
  for (let i = startIdx + 1; i < todos.length; i++) {
    if ((todos[i].level ?? 0) === 0) break;
    items.push(todos[i]);
  }
  const done = items.filter((t) => t.status === "completed").length;
  const active = items.find((t) => t.status === "in_progress");

  return (
    <div className="border border-border-soft rounded-lg overflow-hidden">
      {/* Phase header */}
      <div className="flex items-center gap-2 px-3 py-2 bg-bg-soft border-b border-border-soft">
        <div className="flex items-center gap-1.5 flex-1 min-w-0">
          <TodoIcon status={phase.status} />
          <span
            className={`text-[12.5px] font-semibold truncate ${
              phase.status === "completed" ? "text-fg-faint line-through" : "text-fg"
            }`}
          >
            {phase.activeForm || phase.content}
          </span>
        </div>
        <span className="text-[10px] font-mono text-fg-faint shrink-0">{done}/{items.length}</span>
      </div>
      {/* Sub-steps */}
      {items.length > 1 && (
        <div className="px-3 py-1.5 flex flex-col">
          {items.slice(1).map((t, i) => (
            <TodoItem key={i} todo={t} isPhase={false} />
          ))}
        </div>
      )}
      {active && items.length === 1 && phase.status === "in_progress" && (
        <div className="px-3 py-1.5">
          <div className="text-[11px] text-fg-faint">执行中…</div>
        </div>
      )}
    </div>
  );
}

export function PlanPanel({
  planContent,
  todos,
  onClose,
}: {
  planContent: string;
  todos?: Todo[];
  onClose: () => void;
}) {
  const tr = useT();
  const hasContent = planContent.trim().length > 0;
  const total = todos?.length ?? 0;
  const done = todos?.filter((t) => t.status === "completed").length ?? 0;
  const active = todos?.find((t) => t.status === "in_progress");
  const progress = total > 0 ? Math.round((done / total) * 100) : 0;

  // Extract phases (level 0 items)
  const phases = todos?.filter((t) => (t.level ?? 0) === 0) ?? [];
  const phaseIndices = todos
    ? todos.reduce<number[]>((acc, t, i) => {
        if ((t.level ?? 0) === 0) acc.push(i);
        return acc;
      }, [])
    : [];

  return (
    <ResizableDrawer onClose={onClose}>
      <DrawerHeader onClose={onClose}>
        <FileText size={16} className="text-accent shrink-0" />
        <DrawerTitle text={tr("plan.title")} />
        {total > 0 && (
          <span className="text-[11px] font-mono text-fg-faint bg-bg-soft px-1.5 py-0.5 rounded shrink-0">
            {done}/{total}
          </span>
        )}
      </DrawerHeader>

      {/* Progress + active step */}
      {total > 0 && (
        <div className="shrink-0 px-4 pt-3 pb-2 flex flex-col gap-2">
          <ProgressBar done={done} total={total} />
          {active && (
            <div className="flex items-center gap-1.5 text-[11.5px]">
              <Loader2 size={12} className="text-accent animate-spin shrink-0" />
              <span className="text-fg-faint">当前：</span>
              <span className="text-fg font-medium truncate">{active.activeForm || active.content}</span>
            </div>
          )}
        </div>
      )}

      {/* Content */}
      {!hasContent && total === 0 ? (
        <div className="flex-1 flex flex-col items-center justify-center gap-3 text-fg-faint">
          <FileText size={36} className="opacity-20" />
          <div className="flex flex-col items-center gap-1">
            <span className="text-[13px] font-medium">暂无计划内容</span>
            <span className="text-[11px] opacity-60">启动计划模式后，对话中产生的方案将在此展示</span>
          </div>
        </div>
      ) : (
        <div className="flex-1 min-h-0 overflow-y-auto">
          {/* Todo phases */}
          {phases.length > 0 && (
            <div className="px-4 pt-3 pb-1 flex flex-col gap-2">
              {phaseIndices.map((idx) => (
                <PhaseBlock key={idx} todos={todos!} startIdx={idx} />
              ))}
            </div>
          )}

          {/* Plan markdown body */}
          {hasContent && (
            <div className="px-5 py-4">
              <MemoMarkdown text={planContent} streaming={false} />
            </div>
          )}
        </div>
      )}

      {/* Footer */}
      <footer className="shrink-0 flex items-center gap-2 px-4 py-2.5 border-t border-border-soft text-fg-faint text-[11px]">
        {total > 0 ? (
          <>
            <CheckCircle2 size={12} className={progress === 100 ? "text-ok" : "text-fg-faint"} />
            <span>
              {progress === 100
                ? "计划已完成"
                : progress > 0
                ? `执行中 · ${done}/${total}`
                : "等待审批通过"}
            </span>
          </>
        ) : hasContent ? (
          <>
            <span className="w-1.5 h-1.5 rounded-full bg-accent" />
            <span>审批通过后开始执行各步骤</span>
          </>
        ) : (
          <>
            <ListTodo size={12} />
            <span>输入 /plan 或选择计划模式开始制定方案</span>
          </>
        )}
      </footer>
    </ResizableDrawer>
  );
}
