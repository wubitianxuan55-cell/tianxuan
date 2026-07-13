import { memo, useMemo, useRef, useState } from "react";
import { ChevronRight, Compass } from "lucide-react";
import { CodeViewer } from "./CodeViewer";
import { DiffView } from "./DiffView";
import { useT } from "../lib/i18n";
import { useCompact } from "../hooks/useCompact";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";
import { useNow } from "../lib/useNow";
import { diffsFor, subjectOf, summarize } from "../lib/tools";
import type { Item } from "../lib/store";

type ToolItem = Extract<Item, { kind: "tool" }>;

const SUBAGENT_TOOLS = new Set(["task", "run_skill", "explore", "research", "review", "security_review"]);

const SHELL_PREVIEW_LINES = 10;
const ERROR_SUMMARY_MAX_CHARS = 140;

// ── helpers ──────────────────────────────────────────────────────────

function pretty(json: string): string {
  try { return JSON.stringify(JSON.parse(json), null, 2); } catch { return json; }
}

function normalizeErrorText(text: string): string {
  return text.replace(/\r\n/g, "\n").trim();
}

function summarizeToolError(text: string): string {
  const normalized = normalizeErrorText(text).replace(/^error:\s*/i, "");
  if (!normalized) return "";
  const firstLine = normalized.split("\n")[0]?.trim() ?? "";
  if (firstLine.length <= ERROR_SUMMARY_MAX_CHARS) return firstLine;
  return `${firstLine.slice(0, ERROR_SUMMARY_MAX_CHARS - 1)}…`;
}

function errorNeedsDetails(text: string, summary: string): boolean {
  const normalized = normalizeErrorText(text).replace(/^error:\s*/i, "");
  if (!normalized) return false;
  return normalized.includes("\n") || normalized.length > 200 || (summary !== "" && normalized !== summary);
}

function splitPreview(text: string, n: number): { preview: string; total: number; hasMore: boolean } {
  const lines = text.split("\n");
  const total = lines.length;
  if (total <= n) return { preview: text, total, hasMore: false };
  return { preview: lines.slice(0, n).join("\n"), total, hasMore: true };
}

// ── ToolCard ──────────────────────────────────────────────────────────

export const ToolCard = memo(function ToolCard({ item, subcalls }: { item: ToolItem; subcalls?: ToolItem[] }) {
  const t = useT();
  const compact = useCompact();
  const diffs = useMemo(() => diffsFor(item.name, item.args), [item.name, item.args]);
  const subject = useMemo(() => subjectOf(item.name, item.args), [item.name, item.args]);
  const nested = subcalls ?? [];
  const hasNested = nested.length > 0;
  const isSubagent = SUBAGENT_TOOLS.has(item.name);
  const now = useNow();

  // Duration tracking: record start time when tool becomes running,
  // show live timer while running via useNow tick, final value on completion.
  const startedAtRef = useRef(0);
  if (item.status === "running" && startedAtRef.current === 0) {
    startedAtRef.current = Date.now();
  }
  const durationMs = item.status === "running"
    ? (startedAtRef.current > 0 ? Math.max(0, now - startedAtRef.current) : 0)
    : (startedAtRef.current > 0 ? Math.max(0, Date.now() - startedAtRef.current) : 0);
  const duration = item.status !== "running" && durationMs > 0
    ? `${Math.round(durationMs)} ms`
    : "";

  // All tools default to collapsed; running/error tools open so user sees progress.
  const defaultOpen = !compact && (hasNested ? item.status === "running" : item.status === "running" || item.status === "error");
  const [userOpen, setUserOpen] = useState<boolean | null>(null);
  const open = userOpen ?? defaultOpen;
  const openRef = useRef(open);
  openRef.current = open;

  const [showAll, setShowAll] = useState(false);
  const [showErrorDetails, setShowErrorDetails] = useState(false);

  // Shell output: split into preview + "show all" toggle.
  const shellOutput = item.output ?? null;
  const shellPreview = shellOutput ? splitPreview(shellOutput, SHELL_PREVIEW_LINES) : null;
  const hasArgs = diffs.length > 0 || !!item.args;
  const hasOutput = !!item.output;
  const hasBody = Boolean(diffs.length > 0 || hasNested || shellPreview || hasArgs || hasOutput || item.error);
  const errorText = item.error ? normalizeErrorText(item.error) : "";
  const errorSummary = errorText ? summarizeToolError(errorText) : "";
  const hasErrorDetails = errorText ? errorNeedsDetails(errorText, errorSummary) : false;

  const quiet = item.readOnly && !hasNested && item.status !== "error" && item.status !== "stopped";

  const summary =
    item.status === "running"
      ? ""
      : hasNested
        ? t(nested.length === 1 ? "tool.stepOne" : "tool.stepOther", { n: nested.length })
        : item.error
          ? errorSummary
          : summarize(item.name, item.args, item.output, item.error);

  // GSAP-driven collapse/expand
  const toolBodyRef = useRef<HTMLDivElement>(null);
  useGSAPCollapse(toolBodyRef, open && hasBody);

  return (
    <div className={`tool${quiet ? " tool--quiet" : ""}${isSubagent ? " tool--subagent" : ""}${open && hasBody ? " tool--open" : ""}`} data-entrance={item.id}>
      <button
        type="button"
        className="tool__head"
        data-running={item.status === "running" ? "" : undefined}
        onClick={() => hasBody && setUserOpen(!open)}
        aria-expanded={hasBody ? open : undefined}
      >
        <span className="tool__label-group">
          {hasNested && (
            <span className="tool__nested-count" aria-label={`${nested.length} nested tool calls`}>
              <Compass className="tool__nested-icon" size={14} strokeWidth={2} aria-hidden="true" />
              <span>{nested.length}</span>
            </span>
          )}
          {item.status === "error" && <span className="tool__status-icon tool__status-icon--err">✗</span>}
          {item.status === "done" && <span className="tool__status-icon tool__status-icon--ok">✓</span>}
          {item.status === "stopped" && <span className="tool__status-icon tool__status-icon--stopped">—</span>}
          <span className="tool__name">{item.name}</span>
          {subject && <span className="tool__subject">{subject}</span>}
        </span>
        {summary && <span className="tool__summary">{summary}</span>}
        {duration && <span className="tool__duration">{duration}</span>}
        {hasBody && (
          <span className={`tool__chevron${open ? " tool__chevron--open" : ""}`}>
            <ChevronRight size={12} />
          </span>
        )}
      </button>

      <div ref={toolBodyRef} className="tool__body">
        {diffs.map((d, i) => (
          <div key={i}>
            {d.label && <div className="tool__difflabel">{d.label}</div>}
            <DiffView original={d.original} modified={d.modified} language={d.lang} maxHeight={220} />
          </div>
        ))}

        {hasNested && (
          <div className="tool__nested">
            {nested.map((c) => (
              <ToolCard key={c.id} item={c} />
            ))}
          </div>
        )}

        {shellPreview && (
          <>
            <CodeViewer value={showAll ? shellOutput! : shellPreview.preview} maxHeight={showAll ? 480 : 260} />
            {shellPreview.hasMore && !showAll && (
              <button className="tool__showall" onClick={() => setShowAll(true)}>
          显示全部 {shellPreview.total} 行
              </button>
            )}
            {item.truncated && <div className="tool__note">{t("tool.truncated")}</div>}
          </>
        )}

        {!shellPreview && hasArgs && (
          <div>
            {item.args && <CodeViewer value={pretty(item.args)} language="json" maxHeight={180} />}
          </div>
        )}
        {!shellPreview && hasOutput && !item.args && (
          <div>
            <CodeViewer value={item.output!} maxHeight={260} />
            {item.truncated && <div className="tool__note">{t("tool.truncated")}</div>}
          </div>
        )}

        {errorText && (
          <div className={`tool__err${hasErrorDetails ? " tool__err--compact" : ""}`}>
            {hasErrorDetails ? (
              <>
                <div className="tool__err-summary">{errorSummary}</div>
                <button
                  type="button"
                  className="tool__err-toggle"
                  onClick={() => setShowErrorDetails((v) => !v)}
                  aria-expanded={showErrorDetails}
                >
                  <ChevronRight className={`tool__err-toggle-icon${showErrorDetails ? " tool__err-toggle-icon--open" : ""}`} size={12} aria-hidden="true" />
                  <span>{showErrorDetails ? "隐藏详情" : "显示详情"}</span>
                </button>
                {showErrorDetails && <div className="tool__err-details">{errorText}</div>}
              </>
            ) : (
              errorText
            )}
          </div>
        )}
      </div>
    </div>
  );
});
