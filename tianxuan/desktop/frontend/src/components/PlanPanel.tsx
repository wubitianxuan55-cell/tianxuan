import { useMemo, useState } from "react";
import {
  Check, ChevronDown, ChevronRight, ClipboardList,
  FilePlus, FilePen, X, AlertTriangle,
} from "lucide-react";
import { parsePlan } from "../lib/planParser";
import type { PlanRecord } from "../lib/types";

function FileList({ files, icon, color }: { files: string[]; icon: "new" | "mod"; color: string }) {
  if (files.length === 0) return null;
  const Icon = icon === "new" ? FilePlus : FilePen;
  return (
    <div className={`text-[11px] ${color} space-y-0.5`}>
      {files.map((f, i) => (
        <div key={i} className="flex items-start gap-1">
          <Icon size={10} className="shrink-0 mt-0.5" />
          <span className="font-mono break-all">{f}</span>
        </div>
      ))}
    </div>
  );
}

function StepBadge({ n, done }: { n: number; done: boolean }) {
  return (
    <span className={`inline-flex items-center justify-center w-5 h-5 rounded-full shrink-0 text-[10px] font-bold ${
      done ? "bg-ok/15 text-ok" : "bg-accent/15 text-accent"
    }`}>
      {done ? <Check size={10} /> : n}
    </span>
  );
}

function PlanCard({
  record,
  round,
  open,
  onToggle,
}: {
  record: PlanRecord;
  round: number;
  open: boolean;
  onToggle: () => void;
}) {
  const parsed = useMemo(() => parsePlan(record.plan), [record.plan]);
  const hasPlan = parsed && parsed.steps.length > 0;
  const stepCount = hasPlan ? parsed.steps.length : 0;

  return (
    <div className={`border rounded-lg overflow-hidden transition-colors ${
      record.success
        ? "border-border-soft bg-bg-elev/30"
        : "border-err/20 bg-err/[0.02]"
    }`}>
      <button
        className="flex items-center gap-2 w-full px-3 py-2.5 bg-transparent border-0 cursor-pointer hover:bg-bg-elev/60 text-left transition-colors"
        onClick={onToggle}
      >
        <ChevronRight
          className={`shrink-0 transition-transform duration-200 ${open ? "rotate-90" : ""}`}
          size={14}
        />
        <ClipboardList size={14} className="text-accent shrink-0" />
        <span className="text-fg text-[13px] font-medium flex-1">第 {round} 轮</span>
        {stepCount > 0 && (
          <span className="text-fg-faint text-[11px] font-mono">{stepCount} 步</span>
        )}
        {record.success ? (
          <span className="flex items-center gap-0.5 text-ok text-[11px] font-medium">
            <Check size={12} /> 完成
          </span>
        ) : (
          <span className="flex items-center gap-0.5 text-err text-[11px] font-medium">
            <X size={12} />
            {record.errors.length > 0 ? `${record.errors.length} 错` : "失败"}
          </span>
        )}
      </button>

      {open && (
        <div className="px-3 pb-3 space-y-2.5 border-t border-border-soft pt-2.5">
          {/* 步骤列表 */}
          {hasPlan ? (
            <div className="space-y-2">
              {parsed.steps.map((step) => (
                <div key={step.number} className="flex gap-2">
                  <StepBadge n={step.number} done={record.success} />
                  <div className="flex-1 min-w-0">
                    <div className="text-fg text-[12px] font-medium leading-snug">
                      {step.title}
                    </div>
                    {step.change && (
                      <div className="text-fg-faint text-[11px] mt-0.5 leading-snug">
                        {step.change}
                      </div>
                    )}
                    {step.files && step.files.length > 0 && (
                      <div className="mt-1 text-fg-dim text-[11px] font-mono">
                        {step.files.join(" · ")}
                      </div>
                    )}
                    {step.success && (
                      <div className="mt-1 text-fg-faint text-[10px] border-l-2 border-ok/30 pl-1.5">
                        ✓ {step.success}
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          ) : record.plan ? (
            <div className="text-fg-faint text-[12px] leading-relaxed whitespace-pre-wrap bg-bg-soft rounded p-2 border border-border-soft max-h-[200px] overflow-y-auto">
              {record.plan}
            </div>
          ) : null}

          {/* 执行结果 */}
          <div className="space-y-1.5">
            {record.summary && (
              <div className="text-fg-dim text-[12px] leading-relaxed border-t border-border-soft pt-2">
                {record.summary}
              </div>
            )}

            <FileList files={record.filesCreated} icon="new" color="text-ok" />
            <FileList files={record.filesModified} icon="mod" color="text-warning" />

            {record.errors.map((err, i) => (
              <div key={i} className="flex items-start gap-1.5 text-err text-[11px] leading-snug border-t border-err/10 pt-1.5">
                <AlertTriangle size={11} className="shrink-0 mt-0.5" />
                <span>{err}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

export function PlanPanel({ plans }: { plans: PlanRecord[] }) {
  const [expandAll, setExpandAll] = useState(true);
  const [openSet, setOpenSet] = useState<Set<number>>(new Set());

  const toggle = (i: number) => {
    setOpenSet((prev) => {
      const next = new Set(prev);
      if (next.has(i)) next.delete(i); else next.add(i);
      return next;
    });
  };

  const toggleAll = () => {
    if (expandAll) {
      // Switching from "all open" to "all closed": openSet stays empty
    } else {
      // Switching from "all closed" to "all open": populate openSet
      setOpenSet(new Set(plans.map((_, i) => i)));
    }
    setExpandAll((v) => !v);
  };

  if (plans.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-fg-faint gap-4 px-8">
        <ClipboardList size={44} className="opacity-25" />
        <div className="text-center space-y-1">
          <div className="text-[14px] font-medium">暂无计划</div>
          <div className="text-[12px] leading-relaxed max-w-[200px]">
            双模型模式下，每次任务执行完毕后计划与结果自动汇集于此
          </div>
        </div>
      </div>
    );
  }

  const displayed = [...plans].reverse(); // newest first

  return (
    <div className="flex flex-col h-full overflow-y-auto">
      <div className="flex items-center justify-between px-3 py-2 border-b border-border-soft shrink-0">
        <span className="text-fg-faint text-[11px] font-medium">
          {plans.length} 轮计划
        </span>
        <button
          className="flex items-center gap-1 text-fg-faint text-[10px] border-0 bg-transparent cursor-pointer hover:text-fg transition-colors"
          onClick={toggleAll}
        >
          {expandAll ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
          {expandAll ? "全部折叠" : "全部展开"}
        </button>
      </div>
      <div className="p-2 space-y-2">
        {displayed.map((p, i) => {
          const realIdx = plans.length - 1 - i;
          const isOpen = expandAll || openSet.has(realIdx);
          return (
            <PlanCard
              key={realIdx}
              record={p}
              round={realIdx + 1}
              open={isOpen}
              onToggle={() => toggle(realIdx)}
            />
          );
        })}
      </div>
    </div>
  );
}
