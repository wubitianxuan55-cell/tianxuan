import { useState } from "react";
import {
  Ban,
  Check,
  ChevronRight,
  FilePen,
  FileText,
  FolderOpen,
  Globe,
  Loader2,
  ListTree,
  Search,
  SquareTerminal,
  Wrench,
  X,
  Eye,
  EyeOff,
  type LucideIcon,
} from "lucide-react";
import { CodeViewer } from "./CodeViewer";
import { DiffView } from "./DiffView";
import { useT } from "../lib/i18n";
import { diffsFor, subjectOf, summarize } from "../lib/tools";
import type { Item } from "../lib/store";

type ToolItem = Extract<Item, { kind: "tool" }>;

const ICONS: Record<string, LucideIcon> = {
  edit_file: FilePen,
  multi_edit: FilePen,
  write_file: FilePen,
  read_file: FileText,
  bash: SquareTerminal,
  ls: FolderOpen,
  glob: Search,
  grep: Search,
  web_fetch: Globe,
  task: ListTree,
};

function pretty(json: string): string {
  try {
    return JSON.stringify(JSON.parse(json), null, 2);
  } catch {
    return json;
  }
}

function StatusGlyph({ status }: { status: ToolItem["status"] }) {
  if (status === "running") return <Loader2 className="ico spin" size={13} />;
  if (status === "error") return <X className="ico ico--err" size={13} />;
  if (status === "stopped") return <Ban className="ico ico--stopped" size={13} />;
  return <Check className="ico ico--ok" size={13} />;
}

// ToolCard renders one tool call. V5.30: two-level expansion —
// first click shows args/structure; second click (eye icon) shows full output.
export function ToolCard({ item, subcalls }: { item: ToolItem; subcalls?: ToolItem[] }) {
  const t = useT();
  const diffs = diffsFor(item.name, item.args);
  const subject = subjectOf(item.name, item.args);
  const Icon = ICONS[item.name] ?? Wrench;
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

  const quiet =
    item.readOnly && !hasNested && item.status !== "error" && item.status !== "stopped";

  // 估算输出行数
  const outputLines = item.output ? item.output.split("\n").length : 0;

  return (
    <div className={`tool tool--${item.status} ${quiet ? "tool--quiet" : ""}`}>
      <div
        className={`tool__row ${expandable ? "tool__row--clickable" : ""}`}
        onClick={expandable ? () => setOpen((v) => !v) : undefined}
      >
        {expandable ? (
          <ChevronRight className={`tool__chevron ${open ? "tool__chevron--open" : ""}`} size={13} />
        ) : (
          <span className="tool__chevron tool__chevron--placeholder" />
        )}
        <Icon className="tool__icon" size={14} />
        <span className="tool__name">{item.name}</span>
        {subject && <span className="tool__subject">{subject}</span>}
        {summary && <span className="tool__summary">{summary}</span>}
        <span className="tool__meta">
          <StatusGlyph status={item.status} />
        </span>
      </div>

      {/* 一级展开：diff + args（要求和结构） */}
      {open && diffs.map((d, i) => (
        <div className="tool__body" key={i}>
          {d.label && <div className="tool__difflabel">{d.label}</div>}
          <DiffView original={d.original} modified={d.modified} language={d.lang} maxHeight={260} />
        </div>
      ))}

      {open && hasNested && (
        <div className="tool__nested">
          {nested.map((c) => (
            <ToolCard key={c.id} item={c} />
          ))}
        </div>
      )}

      {open && hasArgs && (
        <div className="tool__body">
          {item.args && <CodeViewer value={pretty(item.args)} language="json" maxHeight={120} />}
        </div>
      )}

      {/* 输出切换按钮 — 只有存在输出时显示 */}
      {open && hasOutput && (
        <div
          className="tool__output-toggle"
          onClick={(e) => { e.stopPropagation(); setShowOutput((v) => !v); }}
        >
          {showOutput ? <EyeOff size={11} /> : <Eye size={11} />}
          <span>
            {showOutput ? "隐藏输出" : `显示输出`}
            {outputLines > 0 && ` (${outputLines} 行)`}
          </span>
        </div>
      )}

      {/* 二级展开：输出内容 */}
      {open && showOutput && hasOutput && (
        <div className="tool__body">
          <CodeViewer value={item.output!} maxHeight={280} />
          {item.truncated && <div className="tool__note">{t("tool.truncated")}</div>}
        </div>
      )}

      {open && item.error && <div className="tool__err">{item.error}</div>}
    </div>
  );
}
