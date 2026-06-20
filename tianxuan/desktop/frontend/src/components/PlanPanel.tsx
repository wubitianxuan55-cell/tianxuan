import { FileText, X } from "lucide-react";
import { useT } from "../lib/i18n";
import { MemoMarkdown } from "./MemoMarkdown";
import { ResizableDrawer } from "./ResizableDrawer";

export function PlanPanel({ planContent, onClose }: { planContent: string; onClose: () => void }) {
  const tr = useT();
  const hasContent = planContent.trim().length > 0;

  return (
    <ResizableDrawer onClose={onClose}>
      {/* ── Header ── */}
      <header className="flex items-center justify-between shrink-0 px-4 py-3.5 bg-bg-elev border-b border-border">
        <div className="flex items-center gap-2.5 text-[15px] font-semibold text-fg">
          <FileText size={16} className="text-accent shrink-0" />
          <span>{tr("plan.title")}</span>
        </div>
        <button
          className="inline-flex items-center justify-center w-[26px] h-[26px] border border-border bg-bg-soft text-fg-faint rounded-[7px] cursor-pointer transition-[color,border-color,background] duration-[0.12s] hover:text-fg hover:border-fg-faint no-drag"
          onClick={onClose}
          title={tr("common.close")}
        >
          <X size={14} />
        </button>
      </header>

      {/* ── Body ── */}
      {!hasContent ? (
        <div className="flex-1 flex items-center justify-center text-fg-faint text-[13px]">
          {tr("plan.empty")}
        </div>
      ) : (
        <div className="flex-1 min-h-0 overflow-y-auto">
          <div className="px-5 py-4 plan-panel__body">
            <MemoMarkdown text={planContent} streaming={false} />
          </div>
        </div>
      )}

      {/* ── Footer status ── */}
      <footer className="shrink-0 flex items-center gap-2 px-4 py-2.5 border-t border-border-soft text-fg-faint text-[11px]">
        <span className="w-1.5 h-1.5 rounded-full bg-accent" />
        <span>{hasContent ? "执行计划进行中 — 审批通过后开始执行各步骤" : "暂无计划内容"}</span>
      </footer>
    </ResizableDrawer>
  );
}
