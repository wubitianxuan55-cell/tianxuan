import { CheckCircle2, FileText, ListTodo, X } from "lucide-react";
import { useT } from "../lib/i18n";
import { MemoMarkdown } from "./MemoMarkdown";
import { ResizableDrawer } from "./ResizableDrawer";
import type { Todo } from "../lib/tools";

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

  return (
    <ResizableDrawer onClose={onClose}>
      {/* ── Header ── */}
      <header className="flex items-center justify-between shrink-0 px-4 py-3.5 bg-bg-elev border-b border-border">
        <div className="flex items-center gap-2.5 min-w-0">
          <FileText size={16} className="text-accent shrink-0" />
          <span className="text-[15px] font-semibold text-fg truncate">{tr("plan.title")}</span>
          {total > 0 && (
            <span className="text-[11px] font-mono text-fg-faint bg-bg-soft px-1.5 py-0.5 rounded shrink-0">
              {done}/{total}
            </span>
          )}
        </div>
        <button
          className="inline-flex items-center justify-center w-[26px] h-[26px] border border-border bg-bg-soft text-fg-faint rounded-[7px] cursor-pointer transition-[color,border-color,background] duration-[0.12s] hover:text-fg hover:border-fg-faint no-drag"
          onClick={onClose}
          title={tr("common.close")}
        >
          <X size={14} />
        </button>
      </header>

      {/* ── Progress bar ── */}
      {total > 0 && (
        <div className="shrink-0 px-4 pt-3 pb-1">
          <div className="h-1.5 bg-bg-soft rounded-full overflow-hidden">
            <div
              className="h-full rounded-full transition-all duration-500 ease-out"
              style={{
                width: `${progress}%`,
                background: progress === 100
                  ? "var(--ok)"
                  : progress > 0
                    ? "linear-gradient(90deg, var(--ok), var(--accent))"
                    : "transparent",
              }}
            />
          </div>
        </div>
      )}

      {/* ── Active step ── */}
      {active && (
        <div className="shrink-0 flex items-center gap-2 px-4 pt-2 pb-1 text-[12px]">
          <span className="w-1.5 h-1.5 rounded-full bg-accent animate-pulse shrink-0" />
          <span className="text-fg-faint">进行中：</span>
          <span className="text-fg font-medium truncate">{active.content}</span>
        </div>
      )}

      {/* ── Body ── */}
      {!hasContent ? (
        <div className="flex-1 flex flex-col items-center justify-center gap-3 text-fg-faint">
          <FileText size={36} className="opacity-20" />
          <div className="flex flex-col items-center gap-1">
            <span className="text-[13px] font-medium">暂无计划内容</span>
            <span className="text-[11px] opacity-60">启动计划模式后，对话中产生的方案将在此展示</span>
          </div>
        </div>
      ) : (
        <div className="flex-1 min-h-0 overflow-y-auto">
          <div className="px-5 py-4">
            <MemoMarkdown text={planContent} streaming={false} />
          </div>
        </div>
      )}

      {/* ── Footer ── */}
      <footer className="shrink-0 flex items-center gap-2 px-4 py-2.5 border-t border-border-soft text-fg-faint text-[11px]">
        {total > 0 ? (
          <>
            <CheckCircle2 size={12} className={progress === 100 ? "text-ok" : "text-fg-faint"} />
            <span>
              {progress === 100
                ? "计划已完成 — 所有步骤执行完毕"
                : progress > 0
                  ? `计划执行中 — ${done}/${total} 步骤已完成`
                  : "计划就绪 — 等待审批通过后开始执行"}
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
