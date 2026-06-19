import { FileText } from "lucide-react";
import { useT } from "../lib/i18n";
import { MemoMarkdown } from "./MemoMarkdown";
import { ResizableDrawer } from "./ResizableDrawer";

// PlanPanel — V5.20: 右侧计划面板 (Kun PlanPanel 移植)
// 显示当前 plan.md 内容，支持 Markdown 渲染。
// planContent 由 App 从 AI 响应中提取（create_plan 工具调用后的 assistant 消息）。
export function PlanPanel({
  planContent,
  onClose,
}: {
  planContent: string;
  onClose: () => void;
}) {
  const tr = useT();

  if (!planContent.trim()) {
    return (
      <ResizableDrawer onClose={onClose}>
        <header className="flex items-center justify-between px-4 py-3.5 bg-bg-elev border-b border-border">
          <div className="text-[15px] font-semibold text-fg">
            <FileText size={15} />
            <span>{tr("plan.title")}</span>
          </div>
          <button className="inline-flex items-center gap-[5px] h-[26px] px-[11px] border border-border bg-bg-soft text-fg-dim text-xs rounded-[7px] cursor-pointer transition-[color,border-color,background] duration-[0.12s] hover:text-fg hover:border-fg-faint disabled:opacity-40 disabled:cursor-default disabled:hover:text-fg-dim disabled:hover:border-border no-drag" onClick={onClose} title={tr("common.close")}>
            ✕
          </button>
        </header>
        <div className="overflow-y-auto px-4 py-3.5 flex flex-col gap-[22px]" style={{ padding: "16px", color: "var(--fg-faint)", fontSize: "13px" }}>
          {tr("plan.empty")}
        </div>
      </ResizableDrawer>
    );
  }

  return (
    <ResizableDrawer onClose={onClose}>
      <header className="flex items-center justify-between px-4 py-3.5 bg-bg-elev border-b border-border">
        <div className="text-[15px] font-semibold text-fg">
          <FileText size={15} />
          <span>{tr("plan.title")}</span>
        </div>
        <button className="inline-flex items-center gap-[5px] h-[26px] px-[11px] border border-border bg-bg-soft text-fg-dim text-xs rounded-[7px] cursor-pointer transition-[color,border-color,background] duration-[0.12s] hover:text-fg hover:border-fg-faint disabled:opacity-40 disabled:cursor-default disabled:hover:text-fg-dim disabled:hover:border-border no-drag" onClick={onClose} title={tr("common.close")}>
          ✕
        </button>
      </header>
      <div className="overflow-y-auto px-4 py-3.5 flex flex-col gap-[22px] plan-panel__body">
        <MemoMarkdown text={planContent} streaming={false} />
      </div>
    </ResizableDrawer>
  );
}
