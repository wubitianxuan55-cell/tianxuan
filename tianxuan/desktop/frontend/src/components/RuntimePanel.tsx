import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChevronDown, Copy, Cpu, RefreshCw, Search } from "lucide-react";
import { app } from "../lib/bridge";
import type { BuiltinToolView } from "../lib/types";
import { useToast } from "./Toast";
import { useGSAPCollapse } from "../lib/useGSAPCollapse";
import {
  filterCatalog,
  groupByCategory,
  highlightParts,
  sortCatalog,
  type CatalogGroup,
  type CatalogTool,
} from "../lib/toolCatalog";

type Counts = Record<string, number>;

/** Highlight renders text with case-insensitive query matches marked. */
function Highlight({ text, query, className }: { text: string; query: string; className?: string }) {
  const parts = highlightParts(text, query);
  return (
    <span className={className}>
      {parts.map((p, i) =>
        p.hit ? (
          <mark key={i} className="bg-accent/25 text-accent rounded-[2px] px-px">
            {p.text}
          </mark>
        ) : (
          <span key={i}>{p.text}</span>
        ),
      )}
    </span>
  );
}

function CatalogCard({
  tool,
  count,
  query,
  onCopy,
}: {
  tool: CatalogTool;
  count: number;
  query: string;
  onCopy: (name: string) => void;
}) {
  const active = count > 0;
  return (
    <button
      type="button"
      title={tool.fullDescription || tool.description || tool.name}
      onClick={() => onCopy(tool.name)}
      className={`group flex items-start gap-1.5 w-full px-2 py-1.5 rounded-md border border-border-soft bg-bg cursor-pointer text-left transition-colors ${
        active ? "border-accent-soft bg-sidebar-active" : "hover:border-accent-soft hover:bg-sidebar-active"
      }`}
    >
      <span className={`w-1.5 h-1.5 mt-[5px] rounded-full shrink-0 ${active ? "bg-accent" : "bg-border-soft"}`} />
      <span className="flex-1 min-w-0 flex flex-col gap-0.5 leading-[1.25]">
        <span className="flex items-center gap-1 min-w-0">
          <Highlight
            text={tool.name}
            query={query}
            className={`font-mono text-[10.5px] truncate ${active ? "text-accent font-semibold" : "text-fg-dim"}`}
          />
          {tool.readOnly && (
            <span
              className="shrink-0 text-[9px] px-1 py-px rounded bg-border-soft/60 text-fg-faint"
              title="只读工具（无副作用）"
            >
              RO
            </span>
          )}
        </span>
        {tool.description && (
          <Highlight
            text={tool.description}
            query={query}
            className="text-[10px] text-fg-faint leading-[1.3] line-clamp-1"
          />
        )}
      </span>
      <span className={`shrink-0 font-mono text-[11px] font-semibold mt-px ${active ? "text-accent" : "text-fg-faint"}`}>
        {count}
      </span>
      <Copy
        size={10}
        className="shrink-0 self-center text-fg-faint opacity-0 group-hover:opacity-100 transition-opacity"
      />
    </button>
  );
}

function CatalogGroup({
  group,
  counts,
  query,
  defaultOpen,
  onCopy,
}: {
  group: CatalogGroup;
  counts: Counts;
  query: string;
  defaultOpen: boolean;
  onCopy: (name: string) => void;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const ref = useRef<HTMLDivElement>(null);
  useGSAPCollapse(ref, open, { duration: 0.18 });

  const activeCount = group.tools.filter((t) => (counts[t.name] ?? 0) > 0).length;

  return (
    <div className="px-1.5 py-0.5">
      <button
        className="flex items-center gap-1 w-full px-1 py-1.5 bg-transparent border-0 text-left cursor-pointer hover:bg-bg-soft rounded transition-colors"
        onClick={() => setOpen((v) => !v)}
      >
        <ChevronDown
          size={10}
          className={`text-fg-faint transition-transform duration-150 ${open ? "rotate-0" : "-rotate-90"}`}
        />
        <span className="text-[10px] font-semibold uppercase tracking-[0.5px] text-fg-faint">{group.label}</span>
        <span className="text-[9px] font-mono text-fg-faint/50">{group.tools.length}</span>
        {activeCount > 0 && <span className="ml-auto text-[9px] font-mono text-accent">{activeCount}</span>}
      </button>
      <div ref={ref} style={{ overflow: "hidden" }}>
        <div className="flex flex-col gap-0.5 pt-0.5 pb-1">
          {group.tools.map((t) => (
            <CatalogCard key={t.name} tool={t} count={counts[t.name] ?? 0} query={query} onCopy={onCopy} />
          ))}
        </div>
      </div>
    </div>
  );
}

/** RuntimePanel — right drawer 工具 tab: data-driven catalog with search
 *  highlighting, used-first ordering, an "only used" filter, read-only badges
 *  and one-click copy. Tool data comes from the kernel (App.Tools), so the
 *  list never drifts from the real tool set. */
export function RuntimePanel({ counts }: { counts: Counts }) {
  const toast = useToast();
  const [tools, setTools] = useState<BuiltinToolView[]>([]);
  const [loadState, setLoadState] = useState<"loading" | "ready" | "error">("loading");
  const [query, setQuery] = useState("");
  const [onlyUsed, setOnlyUsed] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const load = useCallback(() => {
    setLoadState("loading");
    app
      .Tools()
      .then((v) => {
        setTools(v);
        setLoadState("ready");
      })
      .catch(() => setLoadState("error"));
  }, []);
  useEffect(load, [load]);

  const catalog: CatalogTool[] = useMemo(
    () =>
      tools.map((t) => ({
        name: t.name,
        description: t.description,
        fullDescription: t.fullDescription,
        readOnly: t.readOnly,
      })),
    [tools],
  );

  const filtered = useMemo(() => {
    const sorted = sortCatalog(catalog, counts);
    return filterCatalog(sorted, query, onlyUsed, counts);
  }, [catalog, query, onlyUsed, counts]);

  const groups = useMemo(() => groupByCategory(filtered), [filtered]);
  const activeTotal = useMemo(() => catalog.filter((t) => (counts[t.name] ?? 0) > 0).length, [catalog, counts]);

  const copyName = useCallback(
    (name: string) => {
      navigator.clipboard?.writeText(name).then(
        () => toast.show(`已复制 ${name}`, "info"),
        () => {},
      );
    },
    [toast],
  );

  return (
    <div className="flex flex-col overflow-hidden text-xs h-full">
      {/* Header */}
      <div className="flex items-center gap-1.5 px-2.5 py-2 border-b border-border-soft text-fg-dim font-semibold text-[11px] shrink-0">
        <Cpu size={12} />
        <span>工具</span>
        <span className="ml-auto text-[10px] font-mono text-fg-faint/50">
          {activeTotal > 0 ? `${activeTotal}/${catalog.length}` : catalog.length}
        </span>
      </div>

      {/* Search + only-used filter */}
      <div className="flex items-center gap-1.5 mx-2 my-1.5 shrink-0">
        <div className="flex items-center gap-1.5 flex-1 px-2 h-7 border border-border rounded-md bg-bg text-fg-faint">
          <Search size={12} />
          <input
            ref={inputRef}
            className="flex-1 min-w-0 border-0 outline-none bg-transparent text-fg text-[11.5px] placeholder:text-fg-faint"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索工具…"
          />
          {query && (
            <button
              className="border-0 bg-transparent text-fg-faint cursor-pointer p-0 leading-none hover:text-fg"
              onClick={() => {
                setQuery("");
                inputRef.current?.focus();
              }}
            >
              ✕
            </button>
          )}
        </div>
        <button
          className={`h-7 px-2 rounded-md border text-[10.5px] font-medium cursor-pointer transition-colors ${
            onlyUsed
              ? "border-accent bg-accent/10 text-accent"
              : "border-border text-fg-faint hover:text-fg hover:border-border-strong"
          }`}
          onClick={() => setOnlyUsed((v) => !v)}
          title="只显示本次会话用过的工具"
        >
          仅已用
        </button>
      </div>

      {/* Catalog list */}
      <div className="flex-1 min-h-0 overflow-y-auto pb-2">
        {loadState === "loading" ? (
          <div className="empty-state">加载中…</div>
        ) : loadState === "error" ? (
          <div className="flex flex-col items-center gap-2 px-4 py-6 text-fg-faint">
            <span>工具列表加载失败</span>
            <button
              className="flex items-center gap-1 px-2.5 h-7 rounded-md border border-border text-fg-dim cursor-pointer hover:text-fg hover:border-border-strong transition-colors"
              onClick={load}
            >
              <RefreshCw size={11} />
              重试
            </button>
          </div>
        ) : groups.length === 0 ? (
          <div className="empty-state">{query || onlyUsed ? "无匹配工具" : "暂无工具"}</div>
        ) : (
          groups.map((g, i) => (
            <CatalogGroup
              key={g.category}
              group={g}
              counts={counts}
              query={query}
              defaultOpen={i < 2 || groups.length <= 3}
              onCopy={copyName}
            />
          ))
        )}
      </div>
    </div>
  );
}
