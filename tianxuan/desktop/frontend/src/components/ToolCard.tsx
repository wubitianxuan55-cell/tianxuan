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
  if (status === "running") return <Loader2 className="animate-spin" size={13} />;
  if (status === "error") return <X className="text-err" size={13} />;
  if (status === "stopped") return <Ban className="text-fg-faint" size={13} />;
  return <Check className="text-ok" size={13} />;
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
    <div className={`my-0.5 rounded-lg overflow-hidden border ${
      item.status === "error" ? "border-err/30 bg-[rgba(242,139,130,0.06)]" :
      item.status === "running" ? "border-accent/30" :
      item.status === "stopped" ? "border-border-soft opacity-70" :
      "border-border-soft"
    } ${quiet ? "border-transparent bg-transparent" : ""}`}>
      <div
        className={`flex items-center gap-2 px-2.5 py-1.5 text-fg-dim text-[12.5px] ${
          expandable ? "cursor-pointer hover:bg-bg-soft" : ""
        }`}
        onClick={expandable ? () => setOpen((v) => !v) : undefined}
      >
        {expandable ? (
          <ChevronRight className={`shrink-0 transition-transform duration-150 ${open ? "rotate-90" : ""}`} size={13} />
        ) : (
          <span className="w-[13px] shrink-0" />
        )}
        <Icon className={`shrink-0 ${item.status === "error" ? "text-err" : item.status === "running" ? "text-accent" : "text-fg-faint"}`} size={14} />
        <span className={`font-mono text-xs font-medium ${item.status === "error" ? "text-err" : "text-fg"}`}>{item.name}</span>
        {subject && <span className="text-[11px] text-fg-faint truncate">{subject}</span>}
        {summary && <span className="text-[11px] text-fg-faint italic ml-1 hidden group-hover:inline">{summary}</span>}
        <span className="ml-auto shrink-0 flex items-center gap-1">
          <StatusGlyph status={item.status} />
        </span>
      </div>

      {open && diffs.map((d, i) => (
        <div className="px-2 pb-2" key={i}>
          {d.label && <div className="text-[10px] text-fg-faint uppercase tracking-wider mb-1">{d.label}</div>}
          <DiffView original={d.original} modified={d.modified} language={d.lang} maxHeight={260} />
        </div>
      ))}

      {open && hasNested && (
        <div className="pl-4 border-l border-border-soft ml-4">
          {nested.map((c) => (
            <ToolCard key={c.id} item={c} />
          ))}
        </div>
      )}

      {open && hasArgs && (
        <div className="px-2 pb-2">
          {item.args && <CodeViewer value={pretty(item.args)} language="json" maxHeight={120} />}
        </div>
      )}

      {open && hasOutput && (
        <div
          className="flex items-center gap-1.5 px-2.5 py-1.5 text-[11px] text-fg-faint cursor-pointer hover:text-fg hover:bg-bg-soft border-t border-border-soft"
          onClick={(e) => { e.stopPropagation(); setShowOutput((v) => !v); }}
        >
          {showOutput ? <EyeOff size={11} /> : <Eye size={11} />}
          <span>
            {showOutput ? "隐藏输出" : `显示输出`}
            {outputLines > 0 && ` (${outputLines} 行)`}
          </span>
        </div>
      )}

      {open && showOutput && hasOutput && (
        <div className="px-2 pb-2">
          <CodeViewer value={item.output!} maxHeight={280} />
          {item.truncated && <div className="mt-1 px-2 py-1 border border-border-soft rounded bg-bg-soft text-fg-dim text-[11px]">{t("tool.truncated")}</div>}
        </div>
      )}

      {open && item.error && <div className="px-2.5 py-1.5 text-err text-[12px] leading-relaxed border-t border-err/20">{item.error}</div>}
    </div>
  );
}
