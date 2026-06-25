import { memo, useRef, useState } from "react";
import {
  Ban,
  Check,
  ChevronRight,
  Eye,
  EyeOff,
  Loader2,
  X,
} from "lucide-react";
import { CodeViewer } from "./CodeViewer";
import { DiffView } from "./DiffView";
import { ICONS, mcpOr } from "./tool_icons";
import { useT } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";
import { diffsFor, subjectOf, summarize } from "../lib/tools";
import type { Item } from "../lib/store";

type ToolItem = Extract<Item, { kind: "tool" }>;

function pretty(json: string): string {
  try {
    return JSON.stringify(JSON.parse(json), null, 2);
  } catch {
    return json;
  }
}

function StatusGlyph({ status, recoverable }: { status: ToolItem["status"]; recoverable?: boolean }) {
  if (status === "running") return <Loader2 className="animate-spin" size={13} />;
  if (status === "error") return <X className={recoverable ? "text-fg-faint/60" : "text-err"} size={13} />;
  if (status === "stopped") return <Ban className="text-fg-faint" size={13} />;
  return <Check className="text-ok" size={13} />;
}

// ToolCard renders one tool call. V5.30: two-level expansion —
// first click shows args/structure; second click (eye icon) shows full output.
export const ToolCard = memo(function ToolCard({ item, subcalls }: { item: ToolItem; subcalls?: ToolItem[] }) {
  const t = useT();
  const compact = useCompact();
  const diffs = diffsFor(item.name, item.args);
  const subject = subjectOf(item.name, item.args);
  const Icon = ICONS[item.name] ?? mcpOr(item.name);
  const nested = subcalls ?? [];
  const hasNested = nested.length > 0;

  const summary =
    item.status === "running"
      ? ""
      : hasNested
        ? t(nested.length === 1 ? "tool.stepOne" : "tool.stepOther", { n: nested.length })
        : summarize(item.name, item.args, item.output, item.error);

  const hasArgs = diffs.length > 0 || !!item.args;
  const hasOutput = !!item.output;
  const expandable = hasArgs || hasOutput;

  const [open, setOpen] = useState(false);
  const [showOutput, setShowOutput] = useState(false);

  // 用户点击展开，不做自动展开/折叠
  const effectiveOpen = open;

  const bodyRef = useRef<HTMLDivElement>(null);
  useGSAPCollapse(bodyRef, effectiveOpen && expandable);

  const quiet =
    item.readOnly && !hasNested && item.status !== "error" && item.status !== "stopped";

  // 估算输出行数
  const outputLines = item.output ? item.output.split("\n").length : 0;

  const iconSize = compact ? 12 : 14;
  const rowPy = compact ? "py-1" : "py-1.5";
  const rowPx = compact ? "px-2" : "px-2.5";
  const fontSize = compact ? "text-[11.5px]" : "text-[12.5px]";
  const chevronSize = compact ? 11 : 13;
  const summarySize = compact ? "text-[10px]" : "text-[11px]";

  return (
    <div className={`my-0.5 rounded-lg overflow-hidden border transition-colors duration-300 ${
      item.status === "error" && !item.recoverable ? "border-err/40" :
      item.status === "error" && item.recoverable ? "border-fg-faint/30" :
      item.status === "running" ? "border-accent/30 bg-accent/[0.02] shadow-[0_0_8px_var(--accent-soft)]" :
      item.status === "stopped" ? "border-border-soft opacity-70" :
      "border-border-soft"
    } ${quiet ? "border-transparent bg-transparent" : ""}`}
    style={item.status === "error" && !item.recoverable ? {background: "var(--ds-danger-soft)"} : undefined} data-tone={item.status === "error" && !item.recoverable ? "danger" : item.status === "running" ? "info" : item.status === "done" ? "success" : item.status === "stopped" ? "warning" : undefined}>
      <div
        className={`flex items-center gap-2 ${rowPx} ${rowPy} text-fg-dim ${fontSize} select-none ${
          expandable ? "cursor-pointer hover:bg-bg-soft" : ""
        }`}
        onClick={expandable ? () => setOpen((v) => !v) : undefined}
      >
        {expandable ? (
          <ChevronRight
            className={`shrink-0 transition-transform duration-200 ${effectiveOpen ? "rotate-90" : ""}`}
            size={chevronSize}
          />
        ) : (
          <ChevronRight className="shrink-0 invisible" size={chevronSize} />
        )}
        <Icon
          className={`shrink-0 ${item.status === "error" && !item.recoverable ? "text-err" : item.status === "error" && item.recoverable ? "text-fg-faint/60" : item.status === "running" ? "text-accent" : "text-fg-faint"}`}
          size={iconSize}
        />
        <span className={`font-mono font-medium ${item.status === "error" && !item.recoverable ? "text-err" : item.status === "error" && item.recoverable ? "text-fg-dim/60 line-through" : "text-fg"} ${compact ? "text-[11px]" : "text-xs"}`}>
          {item.name}
        </span>
        {subject && (
          <span className={`text-fg-faint truncate ${summarySize}`}>{subject}</span>
        )}
        {summary && (
          <span className={`text-fg-faint italic ml-1 ${summarySize}`}>{summary}</span>
        )}
        <span className="ml-auto shrink-0 flex items-center gap-1">
          <StatusGlyph status={item.status} recoverable={item.recoverable} />
        </span>
      </div>

      <div ref={bodyRef} style={{ overflow: "hidden" }}>
        <div>
          {diffs.map((d, i) => (
            <div className="px-2 pb-2" key={i}>
              {d.label && <div className="text-[10px] text-fg-faint uppercase tracking-wider mb-1">{d.label}</div>}
              <DiffView original={d.original} modified={d.modified} language={d.lang} maxHeight={260} />
            </div>
          ))}

          {hasNested && (
            <div className="pl-4 border-l border-border-soft ml-4">
              {nested.map((c) => (
                <ToolCard key={c.id} item={c} />
              ))}
            </div>
          )}

          {hasArgs && (
            <div className="px-2 pb-2">
              {item.args && <CodeViewer value={pretty(item.args)} language="json" maxHeight={120} />}
            </div>
          )}

          {hasOutput && (
            <div
              className="flex items-center gap-1.5 px-2.5 py-1.5 text-[11px] text-fg-faint cursor-pointer hover:text-fg hover:bg-bg-soft border-t border-border-soft"
              onClick={(e) => { e.stopPropagation(); setShowOutput((v) => !v); }}
            >
              {showOutput ? <EyeOff size={11} /> : <Eye size={11} />}
              <span>
                {showOutput ? "隐藏输出" : "显示输出"}
                {outputLines > 0 && ` (${outputLines} 行)`}
              </span>
            </div>
          )}

          {showOutput && hasOutput && (
            <div className="px-2 pb-2">
              <CodeViewer value={item.output!} maxHeight={280} />
              {item.truncated && (
                <div className="mt-1 px-2 py-1 border border-border-soft rounded bg-bg-soft text-fg-dim text-[11px]">
                  {t("tool.truncated")}
                </div>
              )}
            </div>
          )}

          {item.error && !item.recoverable && (
            <div className="px-2.5 py-1.5 text-err text-[12px] leading-relaxed border-t border-err/20">
              {item.error}
            </div>
          )}
          {item.error && item.recoverable && (
            <div className="px-2.5 py-1.5 text-fg-faint/60 text-[12px] leading-relaxed border-t border-fg-faint/15">
              {item.error}
            </div>
          )}
        </div>
      </div>
    </div>
  );
});
