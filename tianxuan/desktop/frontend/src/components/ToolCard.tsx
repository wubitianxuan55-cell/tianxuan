import { useRef, useState } from "react";
import {
  Ban,
  Check,
  ChevronRight,
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
  if (status === "running") return <Loader2 className="animate-spin" size={12} />;
  if (status === "error") return <X className={recoverable ? "text-fg-faint/60" : "text-err"} size={12} />;
  if (status === "stopped") return <Ban className="text-fg-faint" size={12} />;
  return <Check className="text-ok" size={12} />;
}

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

  const effectiveOpen = open;

  const bodyRef = useRef<HTMLDivElement>(null);
  useGSAPCollapse(bodyRef, effectiveOpen && expandable);

  const quiet =
    item.readOnly && !hasNested && item.status !== "error" && item.status !== "stopped";

  const outputLines = item.output ? item.output.split("\n").length : 0;

  const rowPy = compact ? "py-0" : "py-0.5";
  const rowPx = compact ? "px-1" : "px-1.5";
  const fontSize = compact ? "text-[10px]" : "text-[11px]";
  const chevronSize = compact ? 10 : 11;
  const summarySize = compact ? "text-[9px]" : "text-[10px]";
  const innerPx = compact ? "px-1" : "px-1.5";
  const innerPb = compact ? "pb-1" : "pb-1.5";

  return (
    <div className={`my-px rounded-md overflow-hidden border transition-colors duration-300 ${
      item.status === "error" && !item.recoverable ? "border-err/40" :
      item.status === "error" && item.recoverable ? "border-fg-faint/30" :
      item.status === "running" ? "border-accent/30 bg-accent/[0.02] shadow-[0_0_6px_var(--accent-soft)]" :
      item.status === "stopped" ? "border-border-soft opacity-70" :
      "border-border-soft"
    } ${quiet ? "border-transparent bg-transparent" : ""}`}
    style={item.status === "error" && !item.recoverable ? {background: "var(--ds-danger-soft)"} : undefined} data-tone={item.status === "error" && !item.recoverable ? "danger" : item.status === "running" ? "info" : item.status === "done" ? "success" : item.status === "stopped" ? "warning" : undefined}>
      <div
        className={`flex items-center gap-1.5 ${rowPx} ${rowPy} text-fg-dim ${fontSize} select-none ${
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
          size={chevronSize + 2}
        />
        <span className={`font-mono font-medium truncate ${item.status === "error" && !item.recoverable ? "text-err" : item.status === "error" && item.recoverable ? "text-fg-dim/60 line-through" : "text-fg"} ${compact ? "text-[9px]" : "text-[10px]"}`}>
          {item.name}
        </span>
        {subject && (
          <span className={`text-fg-faint truncate ${summarySize}`}>{subject}</span>
        )}
        {summary && (
          <span className={`text-fg-faint italic ml-0.5 ${summarySize}`}>{summary}</span>
        )}
        <span className="ml-auto shrink-0">
          <StatusGlyph status={item.status} recoverable={item.recoverable} />
        </span>
      </div>

      <div ref={bodyRef} style={{ overflow: "hidden" }}>
        <div>
          {diffs.map((d, i) => (
            <div className={`${innerPx} ${innerPb}`} key={i}>
              {d.label && <div className="text-[10px] text-fg-faint uppercase tracking-wider mb-0.5">{d.label}</div>}
              <DiffView original={d.original} modified={d.modified} language={d.lang} maxHeight={220} />
            </div>
          ))}

          {hasNested && (
            <div className="pl-3 border-l border-border-soft ml-3">
              {nested.map((c) => (
                <ToolCard key={c.id} item={c} />
              ))}
            </div>
          )}

          {hasArgs && (
            <div className={`${innerPx} ${innerPb}`}>
              {item.args && <CodeViewer value={pretty(item.args)} language="json" maxHeight={60} />}
            </div>
          )}
          {hasOutput && (
            <div className={`${innerPx} ${innerPb}`}>
              <div className="text-[9px] text-fg-faint/60 uppercase tracking-wider mb-0.5 select-none">输出 · {outputLines}L</div>
              <CodeViewer value={item.output!} maxHeight={160} />
              {item.truncated && (
                <div className="mt-1 px-2 py-0.5 border border-border-soft rounded bg-bg-soft text-fg-dim text-[10px]">
                  {t("tool.truncated")}
                </div>
              )}
            </div>
          )}

          {item.error && !item.recoverable && (
            <div className={`${innerPx} py-1 text-err text-[11px] leading-snug border-t border-err/20`}>
              {item.error}
            </div>
          )}
          {item.error && item.recoverable && (
            <div className={`${innerPx} py-1 text-fg-faint/60 text-[11px] leading-snug border-t border-fg-faint/15`}>
              {item.error}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
