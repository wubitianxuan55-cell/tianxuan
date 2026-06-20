import { useEffect, useRef, useState } from "react";
import {
  Activity,
  ArrowRightLeft,
  Ban,
  BookOpen,
  Brain,
  Bug,
  Check,
  CheckCircle,
  ChevronRight,
  Clock,
  Eye,
  EyeOff,
  FilePen,
  FileText,
  FolderOpen,
  GitBranch,
  Globe,
  Hourglass,
  Layers,
  Lightbulb,
  List,
  ListTree,
  Loader2,
  Pencil,
  Plug,
  PlusCircle,
  Search,
  Sparkles,
  SquareTerminal,
  Trash2,
  Wrench,
  X,
  Zap,
  type LucideIcon,
} from "lucide-react";
import { CodeViewer } from "./CodeViewer";
import { DiffView } from "./DiffView";
import { useT } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import { diffsFor, subjectOf, summarize } from "../lib/tools";
import type { Item } from "../lib/store";

type ToolItem = Extract<Item, { kind: "tool" }>;

const ICONS: Record<string, LucideIcon> = {
  // 文件读写
  edit_file: FilePen,
  multi_edit: FilePen,
  write_file: FilePen,
  read_file: FileText,
  delete_range: Trash2,
  delete_symbol: Trash2,
  notebook_edit: FilePen,
  // Shell
  bash: SquareTerminal,
  bash_output: SquareTerminal,
  kill_shell: Ban,
  // 文件浏览
  ls: FolderOpen,
  glob: Search,
  grep: Search,
  // 网络
  web_fetch: Globe,
  web_search: Globe,
  // 子代理
  task: ListTree,
  run_skill: Zap,
  parallel_skills: Layers,
  install_skill: PlusCircle,
  // Git
  git_status: GitBranch,
  git_diff: GitBranch,
  git_log: GitBranch,
  git_commit: GitBranch,
  // LSP
  lsp_diagnostics: Bug,
  lsp_definition: ArrowRightLeft,
  lsp_references: List,
  lsp_hover: Lightbulb,
  lsp_completion: Sparkles,
  lsp_rename: Pencil,
  // 记忆 / 知识
  memory_search: Brain,
  remember: Brain,
  read_skill: BookOpen,
  // 诊断 / 时间
  doctor: Activity,
  time: Clock,
  wait: Hourglass,
  // 计划
  complete_step: CheckCircle,
  // 交互
  ask: List,
};

/** MCP 工具（`mcp__<server>__<tool>`）统一用插头图标 */
function mcpOr(name: string): LucideIcon {
  return name.startsWith("mcp__") ? Plug : Wrench;
}

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

  // 延迟折叠：完成后等 500ms 再自动折叠（紧凑模式 300ms）
  const collapseTimer = useRef<ReturnType<typeof window.setTimeout> | null>(null);
  const wasRunning = useRef(false);
  useEffect(() => {
    if (item.status === "running") {
      wasRunning.current = true;
      if (collapseTimer.current) clearTimeout(collapseTimer.current);
    } else if (wasRunning.current && (item.status === "done" || item.status === "error" || item.status === "stopped")) {
      wasRunning.current = false;
      const timer = setTimeout(() => setOpen(false), compact ? 300 : 500);
      collapseTimer.current = timer;
    }
    return () => { if (collapseTimer.current) clearTimeout(collapseTimer.current); };
  }, [item.status, compact]);

  // 运行中自动展开
  const effectiveOpen = open || item.status === "running";

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
      item.status === "error" ? "border-err/40 bg-[color-mix(in_srgb,var(--err)_6%,transparent)]" :
      item.status === "running" ? "border-accent/30 bg-accent/[0.02] shadow-[0_0_8px_var(--accent-soft)]" :
      item.status === "stopped" ? "border-border-soft opacity-70" :
      "border-border-soft"
    } ${quiet ? "border-transparent bg-transparent" : ""}`}>
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
          className={`shrink-0 ${item.status === "error" ? "text-err" : item.status === "running" ? "text-accent" : "text-fg-faint"}`}
          size={iconSize}
        />
        <span className={`font-mono font-medium ${item.status === "error" ? "text-err" : "text-fg"} ${compact ? "text-[11px]" : "text-xs"}`}>
          {item.name}
        </span>
        {subject && (
          <span className={`text-fg-faint truncate ${summarySize}`}>{subject}</span>
        )}
        {summary && (
          <span className={`text-fg-faint italic ml-1 ${summarySize}`}>{summary}</span>
        )}
        <span className="ml-auto shrink-0 flex items-center gap-1">
          <StatusGlyph status={item.status} />
        </span>
      </div>

      <div className={`grid transition-all duration-200 ease-in-out ${
        effectiveOpen && expandable ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0"
      }`}>
        <div className="overflow-hidden min-h-0">
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

          {item.error && (
            <div className="px-2.5 py-1.5 text-err text-[12px] leading-relaxed border-t border-err/20">
              {item.error}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
