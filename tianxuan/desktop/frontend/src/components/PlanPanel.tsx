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
        <header className="drawer__head">
          <div className="drawer__title">
            <FileText size={15} />
            <span>{tr("plan.title")}</span>
          </div>
          <button className="chip" onClick={onClose} title={tr("common.close")}>
            ✕
          </button>
        </header>
        <div className="drawer__body" style={{ padding: "16px", color: "var(--fg-faint)", fontSize: "13px" }}>
          {tr("plan.empty")}
        </div>
      </ResizableDrawer>
    );
  }

  return (
    <ResizableDrawer onClose={onClose}>
      <header className="drawer__head">
        <div className="drawer__title">
          <FileText size={15} />
          <span>{tr("plan.title")}</span>
        </div>
        <button className="chip" onClick={onClose} title={tr("common.close")}>
          ✕
        </button>
      </header>
      <div className="drawer__body plan-panel__body">
        <MemoMarkdown text={planContent} streaming={false} />
      </div>
    </ResizableDrawer>
  );
}
